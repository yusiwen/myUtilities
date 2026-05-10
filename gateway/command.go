package gateway

import (
	"fmt"
	"log"
	"net/http"

	"github.com/yusiwen/myUtilities/core/store"
	"github.com/yusiwen/myUtilities/es"
	"github.com/yusiwen/myUtilities/wol"
)

const landingPage = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>mu Gateway</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #1a1a2e; color: #e0e0e0; min-height: 100vh; display: flex; align-items: center; justify-content: center; }
  .container { text-align: center; padding: 2rem; }
  h1 { font-size: 2rem; margin-bottom: 0.5rem; color: #ffffff; }
  .subtitle { color: #888; margin-bottom: 2rem; }
  .apps { display: flex; gap: 1.5rem; justify-content: center; flex-wrap: wrap; }
  .app-card { background: #16213e; border: 1px solid #0f3460; border-radius: 12px; padding: 2rem; width: 220px; text-decoration: none; color: #e0e0e0; transition: transform 0.2s, border-color 0.2s; }
  .app-card:hover { transform: translateY(-4px); border-color: #4a9eff; }
  .app-icon { font-size: 2.5rem; margin-bottom: 0.75rem; }
  .app-name { font-size: 1.25rem; font-weight: 600; margin-bottom: 0.5rem; color: #ffffff; }
  .app-desc { font-size: 0.85rem; color: #888; }
  .footer { margin-top: 2.5rem; font-size: 0.75rem; color: #555; }
</style>
</head>
<body>
<div class="container">
  <h1>mu Gateway</h1>
  <p class="subtitle">Unified access to all mu services</p>
  <div class="apps">
    <a href="/wol/" class="app-card">
      <div class="app-icon">&#9200;</div>
      <div class="app-name">Wake-on-LAN</div>
      <div class="app-desc">Manage devices, send WOL magic packets, track boot events</div>
    </a>
    <a href="/es/" class="app-card">
      <div class="app-icon">&#128269;</div>
      <div class="app-name">Elasticsearch</div>
      <div class="app-desc">Query indices, browse documents, manage ES connections</div>
    </a>
  </div>
  <p class="footer">mu &copy; 2025</p>
</div>
</body>
</html>`

func (o *Options) Run() error {
	wolCfg, err := wol.LoadConfig(o.WolConfig)
	if err != nil {
		return fmt.Errorf("gateway: failed to load WOL config: %v", err)
	}
	log.Printf("Gateway: WOL config loaded from %s", o.WolConfig)

	wolStore, err := store.OpenStore(wolCfg.DBPath)
	if err != nil {
		return fmt.Errorf("gateway: failed to open WOL store: %v", err)
	}
	defer wolStore.Close()
	log.Printf("Gateway: WOL store at %s", wolCfg.DBPath)

	wolOpts := &wol.ServeOptions{
		Interface: wolCfg.Interface,
		DBPath:    wolCfg.DBPath,
		Port:      wolCfg.Port,
		Token:     wolCfg.Token,
	}

	esState := es.NewServerState(o.EsConfig)
	if err := esState.LoadConfig(); err != nil {
		log.Printf("Gateway: warning: could not load ES config: %v", err)
	}

	mux := http.NewServeMux()

	mux.Handle("/wol/", http.StripPrefix("/wol", wol.FrontendHandler()))
	mux.Handle("/es/", http.StripPrefix("/es", es.FrontendHandler()))

	wol.RegisterHandlers(mux, wolStore, wolOpts)
	es.RegisterHandlers(mux, esState)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, landingPage)
	})

	addr := fmt.Sprintf(":%d", o.Port)
	log.Printf("Gateway: starting on http://localhost%s", addr)
	log.Printf("Gateway:   /      -> landing page")
	log.Printf("Gateway:   /wol/* -> WOL frontend")
	log.Printf("Gateway:   /es/*  -> ES frontend")
	return http.ListenAndServe(addr, mux)
}
