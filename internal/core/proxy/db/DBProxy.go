package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	go_ora "github.com/sijms/go-ora/v2"
	"github.com/yusiwen/myUtilities/internal/core/proxy"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

// OracleBackendConfig holds Oracle database backend configuration.
type OracleBackendConfig struct {
	proxy.BackendConfig
	Username    string
	Password    string
	ServiceName string
}

type OracleBackendStatus struct {
	proxy.BackendStatus
	Config OracleBackendConfig
}

type OracleProxy struct {
	proxy.DefaultProxy
	Backends []*OracleBackendStatus
}

// Start starts the proxy server and health checks.
func (p *OracleProxy) Start() error {
	// Start health checks
	p.StartHealthChecks()

	// Start proxy listener
	log.Printf("Starting Oracle proxy on %s", p.ListenAddr)
	listener, err := net.Listen("tcp", p.ListenAddr)
	if err != nil {
		return fmt.Errorf("failed to start listener: %w", err)
	}
	defer listener.Close()

	for {
		clientConn, err := listener.Accept()
		if err != nil {
			log.Printf("Accept error: %v", err)
			continue
		}
		log.Printf("New client connection from %s", clientConn.RemoteAddr())

		go p.handleClient(clientConn)
	}
}

func (p *OracleProxy) Close() {
	// Stop health checks
	p.StopHealthChecks()

	// Close all backend connections
	p.Mutex.Lock()
	defer p.Mutex.Unlock()
	for _, backend := range p.Backends {
		backend.Mutex.Lock()
		backend.IsAvailable = false
		backend.Mutex.Unlock()
	}
	log.Println("Oracle proxy closed")
}

// handleClient handles a single client connection, routing it to an available backend.
func (p *OracleProxy) handleClient(clientConn net.Conn) {
	defer clientConn.Close()

	for {
		var rst = func() bool {
			log.Printf("Routing connection for %s", clientConn.RemoteAddr())
			// Get an active backend
			backend, err := p.getActiveBackend()
			if err != nil {
				log.Printf("Failed to route: %v", err)
				return false
			}

			log.Printf("Routing connection to %s (%s)", backend.Config.Name, backend.Config.Host)

			// Connect to the backend database
			backendConn, err := net.DialTimeout("tcp",
				fmt.Sprintf("%s:%d", backend.Config.Host, backend.Config.Port), 3*time.Second)
			if err != nil {
				log.Printf("Failed to connect to backend %s: %v", backend.Config.Name, err)
				return false
			}
			var once sync.Once
			defer once.Do(func() { backendConn.Close() })

			// Start bidirectional data forwarding
			var wg sync.WaitGroup
			wg.Add(2)

			// Client -> Backend
			go func() {
				defer wg.Done()
				_, err := io.Copy(backendConn, clientConn)
				if err != nil && !errors.Is(err, io.EOF) {
					log.Printf("Client->Backend copy error: %v, %s", err, clientConn.RemoteAddr())
				}
				log.Printf("Exit Client->Backend forwarding for %s", clientConn.RemoteAddr())
			}()

			// Backend -> Client
			go func() {
				defer wg.Done()
				_, err := io.Copy(clientConn, backendConn)
				if err != nil && !errors.Is(err, io.EOF) {
					log.Printf("Backend->Client copy error: %v, %s", err, clientConn.RemoteAddr())
				}
				log.Printf("Exit Backend->Client forwarding for %s", clientConn.RemoteAddr())
			}()

			go func() {
				<-backend.Context.Done()
				once.Do(func() { backendConn.Close() })
				log.Printf("Helper goroutine for %s exited", clientConn.RemoteAddr())
			}()

			wg.Wait()

			backend.Mutex.RLock()
			if backend.LastError == nil {
				backend.Cancel()
				backend.Mutex.RUnlock()
				return true
			} else {
				backend.Mutex.RUnlock()
				return false
			}
		}()
		if rst {
			break
		}
		log.Printf("Backend is not available, retrying...")
	}
	log.Printf("Goroutine for %s exited", clientConn.RemoteAddr())
}

// getActiveBackend returns the first available backend by priority.
func (p *OracleProxy) getActiveBackend() (*OracleBackendStatus, error) {
	p.Mutex.Lock()
	defer p.Mutex.Unlock()

	// Find the first available backend (by priority)
	for i, backend := range p.Backends {
		if backend.IsAvailable {
			if backend.Context == nil || backend.Context.Err() != nil {
				backend.Context, backend.Cancel = context.WithCancel(context.Background())
			}

			// Update the currently selected backend
			p.CurrentIdx = i

			log.Printf("Using new route by priority: %s", backend.Config.Name)
			return backend, nil
		}
	}

	return nil, errors.New("no available route found")
}

// StartHealthChecks starts background health check goroutines for all backends.
func (p *OracleProxy) StartHealthChecks() {
	ctx, cancel := context.WithCancel(context.Background())
	p.HealthCheck.CancelFunc = cancel

	// Start independent health checks for all backends
	for _, backend := range p.Backends {
		go p.runHealthCheck(ctx, backend)
	}
}

// StopHealthChecks stops all background health check goroutines.
func (p *OracleProxy) StopHealthChecks() {
	if p.HealthCheck.CancelFunc != nil {
		p.HealthCheck.CancelFunc()
	}
}

// runHealthCheck runs the periodic health check loop for a single backend.
func (p *OracleProxy) runHealthCheck(ctx context.Context, backend *OracleBackendStatus) {
	ticker := time.NewTicker(p.HealthCheck.Interval)
	defer ticker.Stop()

	// Run initial check immediately
	p.performHealthCheck(backend)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Stopping health checks for %s", backend.Config.Name)
			return
		case <-ticker.C:
			p.performHealthCheck(backend)
		}
	}
}

// performHealthCheck runs TCP and SQL health checks on a backend.
func (p *OracleProxy) performHealthCheck(backend *OracleBackendStatus) {
	// 1. TCP connection check
	if err := p.checkTCPConnection(backend); err != nil {
		backend.Mutex.Lock()
		backend.IsAvailable = false
		backend.LastError = fmt.Errorf("TCP check failed: %w", err)
		backend.LastCheck = time.Now()
		backend.Mutex.Unlock()
		if backend.Cancel != nil {
			backend.Cancel()
		}
		log.Printf("Backend '%s' TCP check failed: %v", backend.Config.Name, err)
		return
	}

	// 2. SQL health check
	if err := p.checkSQLHealth(backend); err != nil {
		backend.Mutex.Lock()
		backend.IsAvailable = false
		backend.LastError = fmt.Errorf("SQL check failed: %w", err)
		backend.LastCheck = time.Now()
		backend.Mutex.Unlock()
		if backend.Cancel != nil {
			backend.Cancel()
		}
		log.Printf("Backend '%s' SQL check failed: %v", backend.Config.Name, err)
		return
	}

	// Mark as healthy
	backend.Mutex.Lock()
	backend.IsAvailable = true
	backend.LastError = nil
	backend.LastCheck = time.Now()
	if backend.Context == nil || backend.Context.Err() != nil {
		backend.Context, backend.Cancel = context.WithCancel(context.Background())
	}
	backend.Mutex.Unlock()

	log.Printf("Backend %s is healthy", backend.Config.Name)
}

// checkTCPConnection verifies TCP reachability of the backend.
func (p *OracleProxy) checkTCPConnection(backend *OracleBackendStatus) error {
	conn, err := net.DialTimeout("tcp",
		fmt.Sprintf("%s:%d", backend.Config.Host, backend.Config.Port), 3*time.Second)
	if err != nil {
		return fmt.Errorf("TCP connection failed: %w", err)
	}
	conn.Close()
	return nil
}

// checkSQLHealth verifies the backend responds to SQL queries.
func (p *OracleProxy) checkSQLHealth(backend *OracleBackendStatus) error {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), p.HealthCheck.Timeout)
	defer cancel()

	// Connect to the database
	connStr := go_ora.BuildUrl(backend.Config.Host, backend.Config.Port, backend.Config.ServiceName,
		backend.Config.Username, backend.Config.Password, nil)
	db, err := sql.Open("oracle", connStr)
	if err != nil {
		return fmt.Errorf("failed to open connection: %w", err)
	}
	defer db.Close()

	// Execute the health check query
	var result string
	err = db.QueryRowContext(ctx, p.HealthCheck.Query).Scan(&result)
	if err != nil {
		return fmt.Errorf("query execution failed: %w", err)
	}

	// Verify the result matches expected value
	if result != p.HealthCheck.Expected {
		return fmt.Errorf("unexpected result: %s", result)
	}

	return nil
}

// GetStatusReport returns a formatted status report for all backends.
func (p *OracleProxy) GetStatusReport() string {
	p.Mutex.RLock()
	defer p.Mutex.RUnlock()

	report := "Database Backend Status:\n"
	for i, backend := range p.Backends {
		backend.Mutex.RLock()
		status := "DOWN"
		if backend.IsAvailable {
			status = "UP"
		}

		lastError := ""
		if backend.LastError != nil {
			lastError = backend.LastError.Error()
		}

		report += fmt.Sprintf("[%d] %s (%s): %s\n", i+1, backend.Config.Name, backend.Config.Host, status)
		report += fmt.Sprintf("  Last check: %s\n", backend.LastCheck.Format(time.RFC3339))
		report += fmt.Sprintf("  Last error: %s\n", lastError)

		if i == p.CurrentIdx {
			report += "  CURRENTLY ACTIVE\n"
		}

		backend.Mutex.RUnlock()
	}
	return report
}
