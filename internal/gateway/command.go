package gateway

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/yusiwen/myUtilities/internal/budget"
	corees "github.com/yusiwen/myUtilities/internal/core/es"
	"github.com/yusiwen/myUtilities/internal/core/httpserver"
	coremetrics "github.com/yusiwen/myUtilities/internal/core/metrics"
	"github.com/yusiwen/myUtilities/internal/core/store"
	coresv "github.com/yusiwen/myUtilities/internal/core/svcreg"
	corewol "github.com/yusiwen/myUtilities/internal/core/wol"
	"github.com/yusiwen/myUtilities/internal/crypto"
	"github.com/yusiwen/myUtilities/internal/diff"
	"github.com/yusiwen/myUtilities/internal/es"
	"github.com/yusiwen/myUtilities/internal/jarinfo"
	"github.com/yusiwen/myUtilities/internal/k8s"
	"github.com/yusiwen/myUtilities/internal/metrics"
	"github.com/yusiwen/myUtilities/internal/misc"
	"github.com/yusiwen/myUtilities/internal/mock"
	"github.com/yusiwen/myUtilities/internal/network"
	"github.com/yusiwen/myUtilities/internal/qrcode"
	"github.com/yusiwen/myUtilities/internal/svcreg"
	"github.com/yusiwen/myUtilities/internal/wol"
)

var versionStr string

func SetVersion(v string) {
	versionStr = v
}

// gatewayService is one route listed on the gateway landing page.
type gatewayService struct {
	Path string // route shown in the monospace column
	Href string // link target
	Name string
	Desc string
}

// landingPage renders the gateway's own page: a route table for every
// web-enabled module, in the order the modules are registered.
func landingPage(hasMock bool) string {
	services := []gatewayService{
		{Path: "/wol", Href: "/wol/", Name: "Wake-on-LAN", Desc: "Devices, magic packets, boot history"},
		{Path: "/es", Href: "/es/", Name: "Elasticsearch", Desc: "Indices, documents, connections"},
		{Path: "/qrcode", Href: "/qrcode/", Name: "QR Code", Desc: "Text to QR code"},
		{Path: "/jarinfo", Href: "/jarinfo/", Name: "JAR Analyzer", Desc: "JAR structure and metadata"},
		{Path: "/crypto", Href: "/crypto/", Name: "Crypto", Desc: "Encrypt, decrypt, passwords"},
		{Path: "/diff", Href: "/diff/", Name: "Diff", Desc: "Side-by-side text and file diff"},
		{Path: "/misc", Href: "/misc/", Name: "Misc", Desc: "JSON, UUID, timestamp, hash"},
		{Path: "/network", Href: "/network/", Name: "Network", Desc: "DNS lookup and dig queries"},
		{Path: "/k8s", Href: "/k8s/", Name: "K8s", Desc: "Secret YAML encode and decode"},
		{Path: "/svcreg", Href: "/svcreg/", Name: "Service Registry", Desc: "Register and discover services"},
		{Path: "/budget", Href: "/budget/", Name: "API Budget", Desc: "LLM API balance and usage"},
		{Path: "/metrics", Href: "/metrics/", Name: "Metrics", Desc: "System time-series charts"},
	}
	if hasMock {
		services = append(services, gatewayService{
			Path: "/mock/__admin/", Href: "/mock/__admin/", Name: "Mock Server", Desc: "Mock endpoints and responses",
		})
	}

	var rows strings.Builder
	for _, s := range services {
		fmt.Fprintf(&rows, `    <a class="route" href="%s">
      <span class="path">%s</span>
      <span class="name">%s</span>
      <span class="desc">%s</span>
      <svg class="chev" viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m9 18 6-6-6-6"/></svg>
    </a>
`, s.Href, s.Path, s.Name, s.Desc)
	}
	count := fmt.Sprint(len(services))

	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0, viewport-fit=cover">
<title>mu gateway</title>
<link rel="icon" type="image/svg+xml" href="data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'><rect width='64' height='64' rx='14' fill='%2315151f'/><text x='32' y='40' font-family='ui-monospace,Menlo,monospace' font-size='25' font-weight='600' fill='%23e9ebf2' text-anchor='middle'>mu</text><rect x='21' y='47' width='22' height='4' rx='2' fill='%23e94560'/></svg>" />
<style>
  :root {
    --bg: #15151f;
    --surface: #1a1b26;
    --raised: #23242f;
    --text: #e9ebf2;
    --text2: #a4aab8;
    --text3: #8b91a1;
    --line: rgba(233, 235, 242, 0.09);
    --line-strong: rgba(233, 235, 242, 0.18);
    --accent: #e94560;
    --primary: var(--accent);
    --sans: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif;
    --mono: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace;
    --ease: cubic-bezier(0.22, 1, 0.36, 1);
  }
  :root.light {
    --bg: #f5f6f8;
    --surface: #ffffff;
    --raised: #f0f2f5;
    --text: #14151c;
    --text2: #4c5261;
    --text3: #6b7180;
    --line: rgba(20, 21, 28, 0.10);
    --line-strong: rgba(20, 21, 28, 0.20);
    --accent: #c22846;
    --primary: var(--accent);
  }
  html { color-scheme: dark; }
  :root.light { color-scheme: light; }
  * { margin: 0; padding: 0; box-sizing: border-box; -webkit-tap-highlight-color: transparent; }
  body { min-height: 100vh; min-height: 100svh; display: flex; flex-direction: column; background: var(--bg); color: var(--text); font-family: var(--sans); font-size: 14px; line-height: 1.5; -webkit-font-smoothing: antialiased; }
  ::selection { background: rgba(233, 69, 96, 0.28); }
  a { color: inherit; text-decoration: none; }

  .top { flex: none; display: flex; align-items: center; justify-content: space-between; width: 100%; max-width: 1240px; margin: 0 auto; height: 56px; padding: 0 32px; }
  .brand { display: flex; align-items: baseline; gap: 8px; font-family: var(--mono); line-height: 1; }
  .brand .mu { padding-bottom: 2px; border-bottom: 2px solid var(--accent); font-size: 15px; font-weight: 600; letter-spacing: -0.01em; }
  .brand .sep { font-size: 13px; color: var(--line-strong); }
  .brand .app { font-size: 13px; color: var(--text3); }
  .theme { display: grid; place-items: center; width: 32px; height: 32px; border: 1px solid var(--line); border-radius: 8px; background: transparent; color: var(--text3); cursor: pointer; transition: color 140ms var(--ease), background-color 140ms var(--ease), border-color 140ms var(--ease); }
  .theme:hover { color: var(--text); background: var(--raised); border-color: var(--line-strong); }
  :focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
  .theme svg { width: 16px; height: 16px; }
  .icon-moon { display: none; }
  :root.light .icon-sun { display: none; }
  :root.light .icon-moon { display: block; }

  main { flex: 1; width: 100%; max-width: 1240px; margin: 0 auto; padding: clamp(24px, 10vh, 88px) 32px 40px; display: grid; align-content: start; grid-template-columns: 340px minmax(0, 1fr); gap: 56px; }

  .intro { display: flex; flex-direction: column; justify-content: center; }
  h1 { font-size: 40px; font-weight: 500; line-height: 1.08; letter-spacing: -0.025em; text-wrap: balance; }
  .lede { max-width: 32ch; margin-top: 16px; font-size: 15px; line-height: 1.55; color: var(--text2); }
  .lede code { padding: 1px 5px; border: 1px solid var(--line); border-radius: 4px; background: var(--surface); font-family: var(--mono); font-size: 13px; color: var(--text); white-space: nowrap; }
  .colophon { display: flex; flex-direction: column; align-items: flex-start; gap: 6px; margin-top: 32px; font-family: var(--mono); font-size: 12px; color: var(--text3); }
  .colophon a { transition: color 140ms var(--ease); }
  .colophon a:hover { color: var(--text); }
  .colophon .sep { color: var(--line-strong); }

  .routes { border: 1px solid var(--line); border-radius: 14px; background: var(--surface); overflow: hidden; }
  .routes-head { display: flex; align-items: center; justify-content: space-between; height: 40px; padding: 0 20px; border-bottom: 1px solid var(--line); font-family: var(--mono); font-size: 12px; font-weight: 400; color: var(--text3); }
  .route { display: grid; grid-template-columns: 128px 168px minmax(0, 1fr) 16px; gap: 12px; align-items: center; height: 48px; padding: 0 20px; transition: background-color 140ms var(--ease), box-shadow 140ms var(--ease); }
  .route + .route { border-top: 1px solid var(--line); }
  .route:hover { background: var(--raised); box-shadow: inset 2px 0 0 var(--accent); }
  .route:focus-visible { outline-offset: -2px; background: var(--raised); }
  .route:hover .path, .route:focus-visible .path { color: var(--text); }
  .path { font-family: var(--mono); font-size: 13px; color: var(--text2); transition: color 140ms var(--ease); }
  .name { font-size: 15px; font-weight: 500; }
  .desc { min-width: 0; font-size: 13px; color: var(--text2); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .chev { justify-self: end; color: var(--text3); transition: color 140ms var(--ease), transform 140ms var(--ease); }
  .route:hover .chev, .route:focus-visible .chev { color: var(--accent); transform: translateX(2px); }

  @media (max-width: 1079px) {
    .top { max-width: 900px; }
    main { max-width: 900px; grid-template-columns: minmax(0, 1fr); gap: 40px; padding: 40px 32px 48px; }
    .intro { justify-content: flex-start; }
    h1 { font-size: 34px; }
    .lede { max-width: 46ch; }
  }

  @media (max-width: 700px) {
    .top { padding: 0 20px; }
    main { gap: 32px; padding: 24px 20px 40px; }
    h1 { font-size: 30px; }
    .routes-head { padding: 0 16px; }
    .route { grid-template-columns: 108px minmax(0, 1fr) 16px; grid-template-areas: "path name chev" ". desc ."; row-gap: 2px; height: auto; min-height: 56px; padding: 10px 16px; }
    .path { grid-area: path; font-size: 12px; }
    .name { grid-area: name; }
    .desc { grid-area: desc; white-space: normal; line-height: 1.4; }
    .chev { grid-area: chev; }
  }

  @media (prefers-reduced-motion: reduce) {
    * { transition: none !important; animation: none !important; }
  }
</style>
<script>
(function () {
  if (localStorage.getItem('mu-theme') === 'light') document.documentElement.className = 'light';
})();
</script>
</head>
<body>
<header class="top">
  <div class="brand"><span class="mu">mu</span><span class="sep">/</span><span class="app">gateway</span></div>
  <button class="theme" id="theme-btn" type="button" aria-label="Switch to light theme">
    <svg class="icon-sun" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="4"/><path d="M12 2v2"/><path d="M12 20v2"/><path d="m4.93 4.93 1.41 1.41"/><path d="m17.66 17.66 1.41 1.41"/><path d="M2 12h2"/><path d="M20 12h2"/><path d="m6.34 17.66-1.41 1.41"/><path d="m19.07 4.93-1.41 1.41"/></svg>
    <svg class="icon-moon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"/></svg>
  </button>
</header>
<main>
  <section class="intro">
    <h1>One port for the whole toolbox</h1>
    <p class="lede">Each tool below has a web UI, served by the same <code>mu gateway</code> process.</p>
    <div class="colophon">
      <span>` + versionStr + `</span>
      <span><a href="https://github.com/yusiwen/myUtilities">GitHub</a><span class="sep"> / </span>&copy; <span id="copyright-year">2026</span> Siwen Yu</span>
    </div>
  </section>
  <section class="routes" aria-labelledby="routes-title">
    <h2 class="routes-head" id="routes-title"><span>Routes</span><span>` + count + `</span></h2>
` + rows.String() + `  </section>
</main>
<script>
(function () {
  var key = 'mu-theme';
  var root = document.documentElement;
  var btn = document.getElementById('theme-btn');
  function label() {
    btn.setAttribute('aria-label', root.className === 'light' ? 'Switch to dark theme' : 'Switch to light theme');
  }
  function apply(theme) {
    root.className = theme === 'light' ? 'light' : '';
    localStorage.setItem(key, theme);
    label();
  }
  btn.addEventListener('click', function () {
    apply(root.className === 'light' ? 'dark' : 'light');
  });
  label();
})();
</script>
<script>document.getElementById('copyright-year').textContent = new Date().getFullYear();</script>
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

func (o *Options) configDir() string {
	dir := o.ConfigDir
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			dir = filepath.Join(home, dir[2:])
		}
	}
	return dir
}

func (o *Options) configPath(name string) string {
	return filepath.Join(o.configDir(), name)
}

func (o *Options) Run() error {
	wolCfg, err := corewol.LoadConfig(o.configPath("wol-config.json"))
	if err != nil {
		return fmt.Errorf("gateway: failed to load WOL config: %v", err)
	}
	log.Printf("Gateway: WOL config loaded from %s", o.configPath("wol-config.json"))

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

	esState := corees.NewServerState(o.configPath("es-config.json"))
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
	mockConfigPath := o.configPath("mock-config.json")
	if _, err := os.Stat(mockConfigPath); os.IsNotExist(err) {
		defaultCfg := `{
  "port": 8084,
  "endpoints": [
    {
      "id": "default",
      "method": "GET",
      "path": "/api/hello",
      "status": 200,
      "headers": {"X-Server": "mock-dynamic"},
      "body": "{\"message\": \"Hello from Mock Dynamic Server!\", \"docs\": \"Edit or add endpoints at /__admin/\"}"
    },
    {
      "id": "hello-id",
      "method": "GET",
      "path": "/api/hello/:id",
      "status": 200,
      "delay": "100ms",
      "headers": {"X-Server": "mock-dynamic"},
      "body": "{\"message\": \"Hello guest!\", \"id\": \"{{path.id}}\", \"hint\": \"Try /api/hello/1 or /api/hello/2\"}",
      "responses": [
        {
          "condition": "{{path.id}} == 1",
          "delay": "500ms",
          "headers": {"X-Role": "admin"},
          "body": "{\"message\": \"Hello Admin!\", \"id\": \"{{path.id}}\", \"role\": \"admin\"}"
        },
        {
          "condition": "{{path.id}} == 2",
          "headers": {"X-Role": "user"},
          "body": "{\"message\": \"Hello User!\", \"id\": \"{{path.id}}\", \"role\": \"user\"}"
        }
      ]
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
	k8s.RegisterHandlers(mux, o.ConfigDir)
	log.Printf("Gateway:   /k8s/* -> Kubernetes Tools frontend")

	mux.Handle("/misc/", http.StripPrefix("/misc", withGateway(misc.FrontendHandler())))
	misc.RegisterHandlers(mux)
	log.Printf("Gateway:   /misc/* -> Misc Tools frontend")

	mux.Handle("/network/", http.StripPrefix("/network", withGateway(network.FrontendHandler())))
	network.RegisterHandlers(mux)
	log.Printf("Gateway:   /network/* -> Network Tools frontend")

	svcregClient := &coresv.Client{Server: o.SvcregServer}
	coresv.RestoreState(o.ConfigDir)
	coresv.RegisterProxyAPI(mux, svcregClient)
	mux.Handle("/svcreg/", http.StripPrefix("/svcreg", withGateway(svcreg.FrontendHandler())))
	log.Printf("Gateway:   /svcreg/* -> Service Registry frontend (backend: %s)", o.SvcregServer)

	budget.RegisterHandlers(mux, o.configPath("budget-config.json"))
	mux.Handle("/budget/", http.StripPrefix("/budget", withGateway(budget.FrontendHandler())))
	log.Printf("Gateway:   /budget/* -> API Budget frontend")

	var metricsBackend string
	var metricsMgr *coremetrics.ManagedServer
	metricsProxyTarget := o.MetricsServer

	if o.MetricsManage {
		metricsMgr = coremetrics.NewManagedServer(o.MetricsPort)
		metricsMgr.ConfigDir = o.configDir()
		metricsProxyTarget = fmt.Sprintf("http://localhost:%d", o.MetricsPort)
		if o.MetricsAutoStart {
			if err := metricsMgr.Start(); err != nil {
				log.Printf("Gateway: metrics manage: %v", err)
			}
		}
		coremetrics.RegisterManagedAPI(mux, metricsMgr)
		metricsBackend = fmt.Sprintf("managed localhost:%d", o.MetricsPort)
		st := metricsMgr.Status()
		log.Printf("Gateway: metrics manage: state=%s", st.State)
		if st.Error != "" {
			log.Printf("Gateway: metrics manage: %s", st.Error)
		}
	} else {
		metricsBackend = o.MetricsServer
	}

	coremetrics.RegisterProxyAPI(mux, metricsProxyTarget)
	mux.Handle("/metrics/", http.StripPrefix("/metrics", withGateway(metrics.FrontendHandler())))
	log.Printf("Gateway:   /metrics/* -> Metrics dashboard (backend: %s)", metricsBackend)

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

	srv := httpserver.New(addr, mux)
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		log.Printf("Gateway: shutting down")
		srv.Close()
	}()

	err = srv.ListenAndServe()
	if metricsMgr != nil {
		if err := metricsMgr.Stop(); err != nil {
			log.Printf("Gateway: stop managed metrics: %v", err)
		}
	}
	if err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}
