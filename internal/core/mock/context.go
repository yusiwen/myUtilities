package mock

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
