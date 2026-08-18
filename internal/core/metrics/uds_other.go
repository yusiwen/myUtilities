//go:build !unix

package metrics

// UDSServer is a no-op on non-unix platforms; HTTP remains the status path.
type UDSServer struct{}

// ServeUDS is a no-op on platforms without unix domain sockets.
func ServeUDS(path string, payload func() []byte) *UDSServer {
	return nil
}

// Close is a no-op.
func (u *UDSServer) Close() {}
