//go:build unix

package metrics

import (
	"log"
	"net"
	"os"
	"path/filepath"
	"time"
)

// UDSServer wraps a unix domain socket listener that answers one status
// request per connection with a JSON payload.
type UDSServer struct {
	ln      net.Listener
	path    string
	done    chan struct{}
	payload func() []byte
}

// ServeUDS listens on the given socket path. On connection it writes the JSON
// produced by payload() and closes. It is a no-op if the path is empty. If the
// socket is already served by a live process, it logs a warning and returns a
// nil server (the existing one stays authoritative). Stale socket files are
// removed before binding.
func ServeUDS(path string, payload func() []byte) *UDSServer {
	if path == "" {
		return nil
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0711); err != nil {
			log.Printf("metrics: uds mkdir %s: %v", dir, err)
			return nil
		}
		// Keep an existing directory (e.g. previously created with 0700)
		// traverseable by other local users so they can reach the socket.
		os.Chmod(dir, 0711)
	}

	// If another live process owns the socket, leave it alone.
	if conn, err := net.DialTimeout("unix", path, 300*time.Millisecond); err == nil {
		conn.Close()
		log.Printf("metrics: uds %s already served, skipping", path)
		return nil
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Printf("metrics: uds remove stale %s: %v", path, err)
	}

	ln, err := net.Listen("unix", path)
	if err != nil {
		log.Printf("metrics: uds listen %s: %v", path, err)
		return nil
	}
	// World-readable so any local user can query the status socket; the
	// payload is the same public info already served over HTTP.
	os.Chmod(path, 0666)

	u := &UDSServer{ln: ln, path: path, done: make(chan struct{}), payload: payload}
	go u.acceptLoop()
	return u
}

func (u *UDSServer) acceptLoop() {
	for {
		conn, err := u.ln.Accept()
		if err != nil {
			select {
			case <-u.done:
				return
			default:
				log.Printf("metrics: uds accept: %v", err)
				time.Sleep(50 * time.Millisecond)
				continue
			}
		}
		go u.handle(conn)
	}
}

func (u *UDSServer) handle(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	if u.payload != nil {
		if _, err := conn.Write(u.payload()); err != nil {
			log.Printf("metrics: uds write: %v", err)
		}
	}
}

// Close stops the UDS listener and removes the socket file.
func (u *UDSServer) Close() {
	if u == nil {
		return
	}
	close(u.done)
	u.ln.Close()
	os.Remove(u.path)
}
