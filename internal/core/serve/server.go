package serve

import (
	"net/http"

	"github.com/yusiwen/myUtilities/internal/core/httpserver"
)

// NewDownloadServer builds an HTTP server suited to long file transfers.
//
// httpserver's shared defaults cap the whole response write at five minutes,
// which is right for API calls but truncates a multi-gigabyte download over a
// slow LAN. Only the header phase (the slowloris attack surface) and the idle
// phase stay bounded here, so an active transfer has no wall-clock limit.
func NewDownloadServer(addr string, handler http.Handler) *http.Server {
	return httpserver.NewWith(addr, handler, httpserver.Options{
		ReadHeaderTimeout: httpserver.DefaultReadHeaderTimeout,
		IdleTimeout:       httpserver.DefaultIdleTimeout,
	})
}
