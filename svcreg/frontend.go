package svcreg

import (
	"fmt"
	"log"
	"net/http"
)

type FrontendOptions struct {
	Server string `help:"Service registry server address." default:"http://localhost:30100" env:"MU_SVCREG_SERVER"`
	Port   int    `help:"Port to listen on." default:"30101"`
}

func (o *FrontendOptions) Run() error {
	RestoreState("")
	addr := fmt.Sprintf(":%d", o.Port)
	client := &Client{Server: o.Server}
	mux := http.NewServeMux()
	RegisterProxyAPI(mux, client)
	mux.Handle("/", FrontendHandler())
	log.Printf("Service Registry frontend on %s, backend %s", addr, o.Server)
	return http.ListenAndServe(addr, mux)
}
