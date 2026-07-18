package gateway

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/yusiwen/myUtilities/core/store"
	"github.com/yusiwen/myUtilities/crypto"
	"github.com/yusiwen/myUtilities/diff"
	"github.com/yusiwen/myUtilities/es"
	"github.com/yusiwen/myUtilities/jarinfo"
	"github.com/yusiwen/myUtilities/k8s"
	"github.com/yusiwen/myUtilities/misc"
	"github.com/yusiwen/myUtilities/mock"
	"github.com/yusiwen/myUtilities/network"
	"github.com/yusiwen/myUtilities/qrcode"
	"github.com/yusiwen/myUtilities/wol"
)

var versionStr string

func SetVersion(v string) {
	versionStr = v
}

func landingPage(hasMock bool) string {
	mockCard := ""
	if hasMock {
		mockCard = `
    <a href="/mock/__admin/" class="app-card">
      <div class="app-icon">&#128521;</div>
      <div class="app-name">Mock Server</div>
      <div class="app-desc">Dynamic mock endpoints management</div>
    </a>`
	}
	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>mu Gateway</title>
<link rel="icon" type="image/svg+xml" href="data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 100 100'><text y='.9em' font-size='90'>🌐</text></svg>" />
<style>
  :root {
    --bg: #1a1a2e;
    --surface: #16213e;
    --surface2: #0f3460;
    --text: #e0e0e0;
    --text2: #a0a0b0;
    --text-title: #ffffff;
    --border: #0f3460;
    --border-hover: #4a9eff;
  }
  :root.light {
    --bg: #f5f5f5;
    --surface: #ffffff;
    --surface2: #e0e0e0;
    --text: #333333;
    --text2: #888888;
    --text-title: #222222;
    --border: #dddddd;
    --border-hover: #4a9eff;
  }
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: var(--bg); color: var(--text); min-height: 100vh; display: flex; align-items: center; justify-content: center; }
  .container { text-align: center; padding: 2rem; position: relative; }
  h1 { font-size: 2rem; margin-bottom: 0.5rem; color: var(--text-title); }
  .subtitle { color: var(--text2); margin-bottom: 2rem; }
  .subtitle .version { font-size: 0.75em; color: var(--text2); }
  .apps { display: grid; grid-template-columns: repeat(3, 1fr); gap: 1.5rem; max-width: 780px; margin: 0 auto; }
  .app-card { background: var(--surface); border: 1px solid var(--border); border-radius: 12px; padding: 2rem; width: 220px; text-decoration: none; color: var(--text); transition: transform 0.2s, border-color 0.2s; }
  .app-card:hover { transform: translateY(-4px); border-color: var(--border-hover); }
  .app-icon { font-size: 2.5rem; margin-bottom: 0.75rem; }
  .app-name { font-size: 1.25rem; font-weight: 600; margin-bottom: 0.5rem; color: var(--text-title); }
  .app-desc { font-size: 0.85rem; color: var(--text2); }
  .footer { margin-top: 2.5rem; font-size: 0.75rem; color: var(--text2); }
  .footer a { color: var(--primary); text-decoration: none; }
  .footer a:hover { text-decoration: underline; }
  .toggle-btn { position: fixed; top: 16px; right: 16px; padding: 6px 12px; border: 1px solid var(--border); border-radius: 20px; background: var(--surface); color: var(--text2); cursor: pointer; font-size: 14px; z-index: 100; }
  .toggle-btn:hover { border-color: var(--border-hover); color: var(--text); }
</style>
<script>
(function(){
  var k = 'mu-theme', btn = document.getElementById('theme-btn');
  function setTheme(t) {
    document.documentElement.className = t === 'light' ? 'light' : '';
    localStorage.setItem(k, t);
    if (btn) btn.textContent = t === 'light' ? '\u263E' : '\u2600';
  }
  function toggleTheme() { setTheme(document.documentElement.className === 'light' ? 'dark' : 'light'); }
  var saved = localStorage.getItem(k);
  if (saved) setTheme(saved);
  window.toggleTheme = toggleTheme;
})()
</script>
</head>
<body>
<button class="toggle-btn" id="theme-btn" onclick="toggleTheme()">☀</button>
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
    <a href="/qrcode/" class="app-card">
      <div class="app-icon">&#128208;</div>
      <div class="app-name">QR Code</div>
      <div class="app-desc">Generate QR codes from text</div>
    </a>
    <a href="/jarinfo/" class="app-card">
      <div class="app-icon">&#128230;</div>
      <div class="app-name">JAR Analyzer</div>
      <div class="app-desc">Analyze JAR file structure and metadata</div>
    </a>
    <a href="/crypto/" class="app-card">
      <div class="app-icon">&#128274;</div>
      <div class="app-name">Crypto</div>
      <div class="app-desc">Encrypt, decrypt, and generate passwords</div>
    </a>
    <a href="/diff/" class="app-card">
      <div class="app-icon">&#128196;</div>
      <div class="app-name">Diff</div>
      <div class="app-desc">Compare text and files side by side</div>
    </a>
    <a href="/misc/" class="app-card">
      <div class="app-icon">&#128377;</div>
      <div class="app-name">Misc</div>
      <div class="app-desc">JSON, UUID, timestamp, hash tools</div>
    </a>
    <a href="/network/" class="app-card">
      <div class="app-icon">&#127760;</div>
      <div class="app-name">Network</div>
      <div class="app-desc">DNS lookup and dig query tools</div>
    </a>
    <a href="/k8s/" class="app-card">
      <div class="app-icon">&#128736;</div>
      <div class="app-name">K8s</div>
      <div class="app-desc">Kubernetes Secret YAML generator and decoder</div>
    </a>` + mockCard + `
  </div>
  <p class="footer"><span class="version">` + versionStr + `</span> &mdash; mu &copy; <span id="copyright-year"></span> <a href="https://github.com/yusiwen/myUtilities">Siwen Yu</a></p>
</div>
<script>document.getElementById('copyright-year').textContent=new Date().getFullYear()</script>
</body>
</html>`
}

// injectGatewayFlag returns a middleware that injects <script>window.__MU_GATEWAY__=true</script>
// before </head> in HTML responses, so Svelte frontends can detect they are behind the gateway.
func injectGatewayFlag() func(http.Handler) http.Handler {
	script := []byte("<script>window.__MU_GATEWAY__=true</script>")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			iw := &injectWriter{ResponseWriter: w}
			next.ServeHTTP(iw, r)
			if iw.buf.Len() == 0 {
				return
			}
			body := iw.buf.Bytes()
			if isHTML(body) {
				body = bytes.Replace(body, []byte("</head>"), append(script, []byte("</head>")...), 1)
			}
			w.Write(body)
		})
	}
}

func isHTML(data []byte) bool {
	return bytes.HasPrefix(data, []byte("<!DOCTYPE html>")) ||
		bytes.HasPrefix(data, []byte("<html"))
}

type injectWriter struct {
	http.ResponseWriter
	buf bytes.Buffer
}

func (w *injectWriter) Write(data []byte) (int, error) {
	return w.buf.Write(data)
}

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

	withGateway := injectGatewayFlag()

	mux.Handle("/wol/", http.StripPrefix("/wol", withGateway(wol.FrontendHandler())))
	mux.Handle("/es/", http.StripPrefix("/es", withGateway(es.FrontendHandler())))

	wol.RegisterHandlers(mux, wolStore, wolOpts)
	es.RegisterHandlers(mux, esState)
	log.Printf("Gateway:   /wol/* -> WOL frontend")
	log.Printf("Gateway:   /es/* -> ES frontend")

	hasMock := false
	mockConfigPath := o.MockConfig
	if strings.HasPrefix(mockConfigPath, "~/") {
		home, ere := os.UserHomeDir()
		if ere == nil {
			mockConfigPath = filepath.Join(home, mockConfigPath[2:])
		}
	}
	if _, err := os.Stat(mockConfigPath); os.IsNotExist(err) {
		defaultCfg := `{
  "port": 8084,
  "endpoints": [
    {
      "id": "default",
      "method": "GET",
      "path": "/api/hello",
      "status": 200,
      "body": "{\"message\": \"Hello from Mock Dynamic Server!\", \"docs\": \"Edit or add endpoints at /__admin/\"}"
    }
  ]
}`
		if we := os.WriteFile(mockConfigPath, []byte(defaultCfg), 0644); we == nil {
			log.Printf("Gateway: created default mock config at %s", mockConfigPath)
		}
	}
	mockAdmin, adminErr := mock.NewMockAdminHandler(mockConfigPath)
	if adminErr != nil {
		log.Printf("Gateway: warning: could not load mock config: %v", adminErr)
	} else {
		mux.Handle("/mock/", http.StripPrefix("/mock", withGateway(mockAdmin)))
		log.Printf("Gateway:   /mock/* -> Mock Dynamic admin")
		hasMock = true
	}

	mux.Handle("/qrcode/", http.StripPrefix("/qrcode", withGateway(qrcode.FrontendHandler())))
	qrcode.RegisterHandlers(mux)
	log.Printf("Gateway:   /qrcode/* -> QR Code frontend")

	mux.Handle("/jarinfo/", http.StripPrefix("/jarinfo", withGateway(jarinfo.FrontendHandler())))
	jarinfo.RegisterHandlers(mux)
	log.Printf("Gateway:   /jarinfo/* -> JAR Analyzer frontend")

	mux.Handle("/crypto/", http.StripPrefix("/crypto", withGateway(crypto.FrontendHandler())))
	crypto.RegisterHandlers(mux)
	log.Printf("Gateway:   /crypto/* -> Crypto Toolkit frontend")

	mux.Handle("/diff/", http.StripPrefix("/diff", withGateway(diff.FrontendHandler())))
	diff.RegisterHandlers(mux)
	log.Printf("Gateway:   /diff/* -> Diff Tool frontend")

	mux.Handle("/k8s/", http.StripPrefix("/k8s", withGateway(k8s.FrontendHandler())))
	k8s.RegisterHandlers(mux)
	log.Printf("Gateway:   /k8s/* -> Kubernetes Tools frontend")

	mux.Handle("/misc/", http.StripPrefix("/misc", withGateway(misc.FrontendHandler())))
	misc.RegisterHandlers(mux)
	log.Printf("Gateway:   /misc/* -> Misc Tools frontend")

	mux.Handle("/network/", http.StripPrefix("/network", withGateway(network.FrontendHandler())))
	network.RegisterHandlers(mux)
	log.Printf("Gateway:   /network/* -> Network Tools frontend")

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, landingPage(hasMock))
	})
	log.Printf("Gateway:   / -> landing page")

	addr := fmt.Sprintf(":%d", o.Port)
	log.Printf("Gateway: starting on http://localhost%s", addr)
	return http.ListenAndServe(addr, mux)
}
