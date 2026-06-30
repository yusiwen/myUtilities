package mock

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type DynamicServerConfig struct {
	Port      int               `json:"port"`
	Endpoints []*EndpointConfig `json:"endpoints"`
}

type EndpointConfig struct {
	Method    string                 `json:"method"`
	Path      string                 `json:"path"`
	Response  *ResponseConfig        `json:"response"`
	Responses []*ConditionalResponse `json:"responses,omitempty"`
}

type ResponseConfig struct {
	Status  int                 `json:"status"`
	Delay   string              `json:"delay,omitempty"`
	Headers map[string]string   `json:"headers,omitempty"`
	Body    interface{}         `json:"body"`
}

type ConditionalResponse struct {
	When map[string]string `json:"when"`
	Then *ResponseConfig   `json:"then"`
}

func (o *DynamicServerOptions) Run() error {
	data, err := os.ReadFile(o.Config)
	if err != nil {
		return fmt.Errorf("read config file %s failed: %v", o.Config, err)
	}

	var config DynamicServerConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parse config file failed: %v", err)
	}

	if config.Port == 0 {
		config.Port = 8084
	}

	configDir := filepath.Dir(o.Config)

	mux := http.NewServeMux()
	for _, ep := range config.Endpoints {
		if ep.Method == "" {
			ep.Method = http.MethodGet
		}
		ep.Method = strings.ToUpper(ep.Method)
		pattern := toGoPattern(ep.Path)
		mux.HandleFunc(ep.Method+" "+pattern, makeHandler(ep, configDir))
	}

	fmt.Printf("Dynamic mock server listening on :%d\n", config.Port)
	for _, ep := range config.Endpoints {
		fmt.Printf("  %s %s\n", ep.Method, ep.Path)
		if len(ep.Responses) > 0 {
			fmt.Printf("    with %d conditional(s)\n", len(ep.Responses))
		}
	}

	return http.ListenAndServe(fmt.Sprintf(":%d", config.Port), mux)
}

var pathParamRe = regexp.MustCompile(`:([a-zA-Z_][a-zA-Z0-9_]*)`)
var templateRe = regexp.MustCompile(`\{\{([^}]+)\}\}`)

func toGoPattern(path string) string {
	return pathParamRe.ReplaceAllString(path, "{$1}")
}

func extractPathParams(pattern string) []string {
	matches := pathParamRe.FindAllStringSubmatch(pattern, -1)
	params := make([]string, 0, len(matches))
	for _, m := range matches {
		params = append(params, m[1])
	}
	return params
}

type requestContext struct {
	query  map[string]string
	path   map[string]string
	header map[string]string
	body   map[string]interface{}
}

func buildRequestContext(r *http.Request, pathParams []string) *requestContext {
	ctx := &requestContext{
		query:  make(map[string]string),
		path:   make(map[string]string),
		header: make(map[string]string),
		body:   make(map[string]interface{}),
	}
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			ctx.query[k] = v[0]
		}
	}
	for _, p := range pathParams {
		ctx.path[p] = r.PathValue(p)
	}
	for k := range r.Header {
		ctx.header[strings.ToLower(k)] = r.Header.Get(k)
	}
	if r.Body != nil {
		bodyBytes, err := io.ReadAll(r.Body)
		if err == nil && len(bodyBytes) > 0 {
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
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

func matchConditions(when map[string]string, ctx *requestContext) bool {
	for key, expected := range when {
		actual := resolveValue(key, ctx)
		if actual != expected {
			return false
		}
	}
	return true
}

func selectResponse(ep *EndpointConfig, ctx *requestContext) *ResponseConfig {
	for _, cr := range ep.Responses {
		if matchConditions(cr.When, ctx) {
			return cr.Then
		}
	}
	return ep.Response
}

func resolveBody(body interface{}, ctx *requestContext, baseDir string) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	var content string
	switch v := body.(type) {
	case string:
		bodyPath := v
		if !filepath.IsAbs(bodyPath) {
			bodyPath = filepath.Join(baseDir, bodyPath)
		}
		data, err := os.ReadFile(bodyPath)
		if err != nil {
			return nil, fmt.Errorf("read response body file %s failed: %v", v, err)
		}
		content = string(data)
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshal inline response body failed: %v", err)
		}
		content = string(data)
	}
	resolved := resolveTemplate(content, ctx)
	return []byte(resolved), nil
}

func makeHandler(ep *EndpointConfig, baseDir string) http.HandlerFunc {
	pathParams := extractPathParams(ep.Path)

	return func(w http.ResponseWriter, r *http.Request) {
		ctx := buildRequestContext(r, pathParams)

		resp := selectResponse(ep, ctx)
		if resp == nil {
			http.Error(w, `{"error":"no response defined"}`, http.StatusInternalServerError)
			return
		}

		if resp.Delay != "" {
			d, err := time.ParseDuration(resp.Delay)
			if err == nil {
				time.Sleep(d)
			}
		}

		body, err := resolveBody(resp.Body, ctx, baseDir)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
			return
		}

		for k, v := range resp.Headers {
			w.Header().Set(k, resolveTemplate(v, ctx))
		}
		if w.Header().Get("Content-Type") == "" && len(body) > 0 {
			w.Header().Set("Content-Type", "application/json")
		}

		status := resp.Status
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		if len(body) > 0 {
			w.Write(body)
		}
	}
}
