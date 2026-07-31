package mock

import (
	"fmt"
	"log"
	"net/http"
	"os"

	coremock "github.com/yusiwen/myUtilities/core/mock"
	"github.com/yusiwen/myUtilities/mock/oauth"
)

func (o FileServerOptions) Run() error {
	if err := os.MkdirAll(o.LocalDir, os.ModePerm); err != nil {
		return fmt.Errorf("create local directory failed: %v", err)
	}

	fs := coremock.NewFileServer(o.LocalDir, o.FormKey, o.MaxFileSize)
	fmt.Printf("Server listening at :%d\n", o.Port)
	return http.ListenAndServe(fmt.Sprintf(":%d", o.Port), fs.Handler())
}

func (o MockServerOptions) Run() error {
	if o.Size > 10000 {
		return fmt.Errorf("size to large, max 10000")
	}

	server, err := coremock.NewMockServer(o.Size, o.CsvFiles)
	if err != nil {
		return err
	}

	fmt.Printf("Server listening at :%d\n", o.Port)
	return http.ListenAndServe(fmt.Sprintf(":%d", o.Port), server.Handler())
}

func (o OAuthServerOptions) Run() error {
	authServer := oauth.NewAuthServer()
	mux := http.NewServeMux()
	authServer.SetupRoutes(mux)
	fmt.Printf("OAuth server started on http://localhost:%d\n", o.Port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", o.Port), mux))
	return nil
}

func (o DynamicServerOptions) Run() error {
	endpoints, port, err := coremock.LoadConfig(o.Config)
	if err != nil {
		return err
	}

	if port == 0 {
		port = 8084
	}

	router := coremock.NewDynamicRouter(endpoints, nil, o.Verbose)
	admin := coremock.NewAdminHandler(router, o.Config, o.Verbose, MockFrontendHandler())
	router.SetAdmin(admin)

	fmt.Printf("Dynamic mock server listening on :%d\n", port)
	fmt.Printf("  Admin UI: http://localhost:%d/__admin/\n", port)
	for _, ep := range router.List() {
		fmt.Printf("  %s %s\n", ep.Method, ep.Path)
	}

	return http.ListenAndServe(fmt.Sprintf(":%d", port), router)
}

// NewMockAdminHandler creates an http.Handler that serves the mock admin frontend
// and CRUD API, backed by the given config file.
func NewMockAdminHandler(configPath string) (http.Handler, error) {
	endpoints, _, err := coremock.LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	router := coremock.NewDynamicRouter(endpoints, nil, false)
	admin := coremock.NewAdminHandler(router, configPath, false, MockFrontendHandler())
	router.SetAdmin(admin)
	return admin, nil
}
