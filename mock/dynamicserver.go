package mock

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var pathParamRe = regexp.MustCompile(`:([a-zA-Z_][a-zA-Z0-9_]*)`)
var templateRe = regexp.MustCompile(`\{\{([^}]+)\}\}`)

type requestContext struct {
	query  map[string]string
	path   map[string]string
	header map[string]string
	body   map[string]interface{}
}

func buildRequestContext(r *http.Request, pathParams map[string]string) *requestContext {
	ctx := &requestContext{
		query:  make(map[string]string),
		path:   pathParams,
		header: make(map[string]string),
		body:   make(map[string]interface{}),
	}
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			ctx.query[k] = v[0]
		}
	}
	for k := range r.Header {
		ctx.header[strings.ToLower(k)] = r.Header.Get(k)
	}
	if r.Body != nil {
		bodyBytes, err := io.ReadAll(r.Body)
		if err == nil && len(bodyBytes) > 0 {
			r.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
			json.Unmarshal(bodyBytes, &ctx.body)
		}
	}
	return ctx
}

func resolveValue(path string, ctx *requestContext) string {
	parts := strings.SplitN(path, ".", 2)
	if len(parts) != 2 {
		return ""
	}
	prefix, key := parts[0], parts[1]
	switch prefix {
	case "query":
		return ctx.query[key]
	case "path":
		return ctx.path[key]
	case "header":
		return ctx.header[strings.ToLower(key)]
	case "body":
		return resolveNestedBody(key, ctx.body)
	}
	return ""
}

func resolveNestedBody(key string, body map[string]interface{}) string {
	parts := strings.Split(key, ".")
	current := interface{}(body)
	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return ""
		}
		current, ok = m[part]
		if !ok {
			return ""
		}
	}
	if current == nil {
		return ""
	}
	return fmt.Sprintf("%v", current)
}

func resolveTemplate(content string, ctx *requestContext) string {
	return templateRe.ReplaceAllStringFunc(content, func(match string) string {
		inner := strings.TrimSpace(match[2 : len(match)-2])
		return resolveValue(inner, ctx)
	})
}

func (o *DynamicServerOptions) Run() error {
	endpoints, port, err := loadConfig(o.Config)
	if err != nil {
		return err
	}

	if port == 0 {
		port = 8084
	}

	router := NewDynamicRouter(endpoints, nil, o.Verbose)
	admin := newAdminHandler(router, o.Config, o.Verbose)
	router.admin = admin

	fmt.Printf("Dynamic mock server listening on :%d\n", port)
	fmt.Printf("  Admin UI: http://localhost:%d/__admin/\n", port)
	for _, ep := range router.List() {
		fmt.Printf("  %s %s\n", ep.Method, ep.Path)
	}

	return http.ListenAndServe(fmt.Sprintf(":%d", port), router)
}

type configFile struct {
	Port      int                `json:"port"`
	Endpoints []*ManagedEndpoint `json:"endpoints"`
}

func loadConfig(path string) ([]*ManagedEndpoint, int, error) {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, fmt.Errorf("read config file %s failed: %w", path, err)
	}

	var cfg configFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, 0, fmt.Errorf("parse config file %s failed: %v", path, err)
	}

	if cfg.Endpoints == nil {
		cfg.Endpoints = []*ManagedEndpoint{}
	}

	// Resolve relative body file paths
	baseDir := filepath.Dir(path)
	for _, ep := range cfg.Endpoints {
		ep.Method = strings.ToUpper(ep.Method)
		if ep.Status == 0 {
			ep.Status = http.StatusOK
		}
		if ep.ID == "" {
			ep.ID = generateID()
		}
		// Try to resolve body as file path (backward compat)
		if ep.Body != "" && !strings.HasPrefix(ep.Body, "{") && !strings.HasPrefix(ep.Body, "[") {
			bodyPath := ep.Body
			if !filepath.IsAbs(bodyPath) {
				bodyPath = filepath.Join(baseDir, bodyPath)
			}
			if b, err := os.ReadFile(bodyPath); err == nil {
				ep.Body = string(b)
			}
		}
	}

	return cfg.Endpoints, cfg.Port, nil
}
