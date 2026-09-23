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
		// Rewrite replaces Director, which is deprecated since Go 1.26. The
		// outbound request gets the backend scheme/host while keeping the
		// incoming path, which is what the Director hook used to do.
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = base.Scheme
			pr.Out.URL.Host = base.Host
			pr.Out.Host = base.Host
		},
	}

	mux.HandleFunc("GET /api/metrics", rp.ServeHTTP)
	mux.HandleFunc("GET /api/metrics/hosts", rp.ServeHTTP)
	mux.HandleFunc("GET /api/metrics/info", rp.ServeHTTP)
	mux.HandleFunc("GET /api/metrics/{name}", rp.ServeHTTP)
}
