package es

import (
	"fmt"
	"log"
	"net/http"

	corees "github.com/yusiwen/myUtilities/core/es"
)

func (o *SetHostOptions) Run() error {
	cfg, err := corees.LoadConfig(o.Config)
	if err != nil {
		return err
	}
	cfg.Host = o.Host
	if err := corees.SaveConfig(o.Config, cfg); err != nil {
		return err
	}
	fmt.Printf("ES host set to: %s\n", o.Host)
	return nil
}

func (o *SetUserOptions) Run() error {
	cfg, err := corees.LoadConfig(o.Config)
	if err != nil {
		return err
	}
	cfg.Username = o.User
	if err := corees.SaveConfig(o.Config, cfg); err != nil {
		return err
	}
	fmt.Printf("ES username set to: %s\n", o.User)
	return nil
}

func (o *SetPasswordOptions) Run() error {
	cfg, err := corees.LoadConfig(o.Config)
	if err != nil {
		return err
	}
	cfg.Password = o.Password
	if err := corees.SaveConfig(o.Config, cfg); err != nil {
		return err
	}
	fmt.Println("ES password set")
	return nil
}

func (o *ServeOptions) Run() error {
	state := corees.NewServerState(o.Config)
	if err := state.LoadConfig(); err != nil {
		log.Printf("Warning: could not load ES config: %v", err)
	}

	mux := http.NewServeMux()
	RegisterHandlers(mux, state)
	mux.Handle("/", FrontendHandler())

	host := o.Host
	if host == "" {
		host = "127.0.0.1"
	}
	addr := fmt.Sprintf("%s:%d", host, o.Port)
	log.Printf("Starting ES search UI on http://%s", addr)
	return http.ListenAndServe(addr, mux)
}

// RegisterHandlers registers the ES API routes on the given mux.
func RegisterHandlers(mux *http.ServeMux, state *corees.ServerState) {
	corees.RegisterHandlers(mux, state)
}
