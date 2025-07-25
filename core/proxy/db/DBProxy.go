package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	go_ora "github.com/sijms/go-ora/v2"
	"github.com/yusiwen/myUtilities/core/proxy"
	"io"
	"log"
	"net"
	"sync"
	"time"
)

// TCP 代理服务器
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

// 启动代理服务器
func (p *OracleProxy) Start() error {
	// 启动健康检查
	p.StartHealthChecks()

	// 启动代理服务器
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

		go p.handleClient(clientConn)
	}
}

func (p *OracleProxy) Close() {
	// 停止健康检查
	p.StopHealthChecks()

	// 关闭所有后端连接
	p.Mutex.Lock()
	defer p.Mutex.Unlock()
	for _, backend := range p.Backends {
		backend.Mutex.Lock()
		backend.IsAvailable = false
		backend.Mutex.Unlock()
	}
	log.Println("Oracle proxy closed")
}

// 处理客户端连接
func (p *OracleProxy) handleClient(clientConn net.Conn) {
	defer clientConn.Close()

	// 获取活动后端
	backend, err := p.getActiveBackend()
	if err != nil {
		log.Printf("Failed to route: %v", err)
		return
	}

	log.Printf("Routing connection to %s (%s)", backend.Config.Name, backend.Config.Host)

	// 连接到后端数据库
	backendConn, err := net.DialTimeout("tcp",
		fmt.Sprintf("%s:%d", backend.Config.Host, backend.Config.Port), 3*time.Second)
	if err != nil {
		log.Printf("Failed to connect to backend %s: %v", backend.Config.Name, err)
		return
	}
	defer backendConn.Close()

	// 启动双向数据转发
	var wg sync.WaitGroup
	wg.Add(2)

	// 客户端 -> 后端
	go func() {
		defer wg.Done()
		_, err := io.Copy(backendConn, clientConn)
		if err != nil && !errors.Is(err, io.EOF) {
			log.Printf("Client->Backend copy error: %v", err)
		}
	}()

	// 后端 -> 客户端
	go func() {
		defer wg.Done()
		_, err := io.Copy(clientConn, backendConn)
		if err != nil && !errors.Is(err, io.EOF) {
			log.Printf("Backend->Client copy error: %v", err)
		}
	}()

	wg.Wait()
}

// 获取活动后端
func (p *OracleProxy) getActiveBackend() (*OracleBackendStatus, error) {
	p.Mutex.RLock()
	defer p.Mutex.RUnlock()

	// 查找第一个可用的后端（按优先级）
	for i, backend := range p.Backends {
		backend.Mutex.RLock()
		if backend.IsAvailable {
			backend.Mutex.RUnlock()

			// 更新当前选中的后端
			p.Mutex.Lock()
			p.CurrentIdx = i
			p.Mutex.Unlock()

			log.Printf("Using new route by priority: %s", backend.Config.Name)
			return backend, nil
		}
		backend.Mutex.RUnlock()
	}

	return nil, errors.New("no available route found")
}

// 启动健康检查
func (p *OracleProxy) StartHealthChecks() {
	ctx, cancel := context.WithCancel(context.Background())
	p.HealthCheck.CancelFunc = cancel

	// 对所有后端启动独立健康检查
	for _, backend := range p.Backends {
		go p.runHealthCheck(ctx, backend)
	}
}

// 停止健康检查
func (p *OracleProxy) StopHealthChecks() {
	if p.HealthCheck.CancelFunc != nil {
		p.HealthCheck.CancelFunc()
	}
}

// 运行健康检查循环
func (p *OracleProxy) runHealthCheck(ctx context.Context, backend *OracleBackendStatus) {
	ticker := time.NewTicker(p.HealthCheck.Interval)
	defer ticker.Stop()

	// 立即执行首次检查
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

// 执行健康检查
func (p *OracleProxy) performHealthCheck(backend *OracleBackendStatus) {
	// 1. TCP 连接检查
	if err := p.checkTCPConnection(backend); err != nil {
		backend.Mutex.Lock()
		backend.IsAvailable = false
		backend.LastError = fmt.Errorf("TCP check failed: %w", err)
		backend.LastCheck = time.Now()
		backend.Mutex.Unlock()
		log.Printf("Backend '%s' TCP check failed: %v", backend.Config.Name, err)
		return
	}

	// 2. SQL 健康检查
	if err := p.checkSQLHealth(backend); err != nil {
		backend.Mutex.Lock()
		backend.IsAvailable = false
		backend.LastError = fmt.Errorf("SQL check failed: %w", err)
		backend.LastCheck = time.Now()
		backend.Mutex.Unlock()
		log.Printf("Backend '%s' SQL check failed: %v", backend.Config.Name, err)
		return
	}

	// 标记为健康
	backend.Mutex.Lock()
	backend.IsAvailable = true
	backend.LastError = nil
	backend.LastCheck = time.Now()
	backend.Mutex.Unlock()

	log.Printf("Backend %s is healthy", backend.Config.Name)
}

// 检查 TCP 连接
func (p *OracleProxy) checkTCPConnection(backend *OracleBackendStatus) error {
	conn, err := net.DialTimeout("tcp",
		fmt.Sprintf("%s:%d", backend.Config.Host, backend.Config.Port), 3*time.Second)
	if err != nil {
		return fmt.Errorf("TCP connection failed: %w", err)
	}
	conn.Close()
	return nil
}

// 检查 SQL 健康
func (p *OracleProxy) checkSQLHealth(backend *OracleBackendStatus) error {
	// 创建带超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), p.HealthCheck.Timeout)
	defer cancel()

	// 连接到数据库
	connStr := go_ora.BuildUrl(backend.Config.Host, backend.Config.Port, backend.Config.ServiceName,
		backend.Config.Username, backend.Config.Password, nil)
	db, err := sql.Open("oracle", connStr)
	if err != nil {
		return fmt.Errorf("failed to open connection: %w", err)
	}
	defer db.Close()

	// 执行健康检查查询
	var result string
	err = db.QueryRowContext(ctx, p.HealthCheck.Query).Scan(&result)
	if err != nil {
		return fmt.Errorf("query execution failed: %w", err)
	}

	// 验证结果
	if result != p.HealthCheck.Expected {
		return fmt.Errorf("unexpected result: %s", result)
	}

	return nil
}

// 获取后端状态报告
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
