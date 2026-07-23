package svcreg

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	coresv "github.com/yusiwen/myUtilities/core/svcreg"
)

type ServeOptions struct {
	Config string `help:"Path to config JSON file." default:"~/.config/mu/svcreg-config.json"`
	Host   string `help:"Listen address."`
	Port   int    `help:"Override HTTP server port from config."`
	DBPath string `help:"Override BoltDB file path from config."`
	Web    bool   `name:"web" help:"Also serve the web frontend on the same port."`
}

func (o *ServeOptions) resolveConfig() {
	cfg, err := LoadConfig(o.Config)
	if err != nil {
		log.Printf("Warning: could not load config: %v", err)
		return
	}
	if o.Host == "" {
		o.Host = cfg.Host
	}
	if o.Port == 0 {
		o.Port = cfg.Port
	}
	if o.DBPath == "" {
		o.DBPath = cfg.DBPath
	}
}

func (o *ServeOptions) Run() error {
	o.resolveConfig()
	store, err := coresv.NewBoltStore(o.DBPath)
	if err != nil {
		return fmt.Errorf("failed to open store: %w", err)
	}
	addr := fmt.Sprintf("%s:%d", o.Host, o.Port)
	handler := coresv.NewHandler(store, addr)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	if o.Web {
		RegisterProxyAPI(mux, &Client{Server: "http://" + addr})
		mux.Handle("/", FrontendHandler())
	}

	srv := &http.Server{Addr: addr, Handler: coresv.LoggingMiddleware(mux)}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("Service Registry server listening on %s", addr)
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-sigCh:
		fmt.Println("\nshutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
		return store.Close()
	}
}
