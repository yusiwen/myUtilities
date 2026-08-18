package metrics

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// RegisterProxyAPI registers read-only proxy endpoints that forward to a
// running metrics server backend. Only the three GET read endpoints are exposed
// (list, hosts, query); the write/compact endpoints are intentionally not forwarded.
func RegisterProxyAPI(mux *http.ServeMux, serverURL string) {
	base, err := url.Parse(serverURL)
	if err != nil {
		log.Printf("metrics: invalid proxy target %q: %v", serverURL, err)
		return
	}

	rp := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = base.Scheme
			req.URL.Host = base.Host
			req.Host = base.Host
		},
	}

	mux.HandleFunc("GET /api/metrics", rp.ServeHTTP)
	mux.HandleFunc("GET /api/metrics/hosts", rp.ServeHTTP)
	mux.HandleFunc("GET /api/metrics/info", rp.ServeHTTP)
	mux.HandleFunc("GET /api/metrics/{name}", rp.ServeHTTP)
}
