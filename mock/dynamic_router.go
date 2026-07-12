package mock

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

type ManagedEndpoint struct {
	ID      string            `json:"id"`
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Status  int               `json:"status"`
	Delay   string            `json:"delay,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body"`
}

type DynamicRouter struct {
	mu        sync.RWMutex
	endpoints []*ManagedEndpoint
	admin     http.Handler
	verbose   bool
}

func NewDynamicRouter(endpoints []*ManagedEndpoint, admin http.Handler, verbose bool) *DynamicRouter {
	return &DynamicRouter{
		endpoints: endpoints,
		admin:     admin,
		verbose:   verbose,
	}
}

func (r *DynamicRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if strings.HasPrefix(req.URL.Path, "/__admin") {
		r.admin.ServeHTTP(w, req)
		return
	}

	r.mu.RLock()
	for _, ep := range r.endpoints {
		if params, ok := matchEndpoint(ep, req.Method, req.URL.Path); ok {
			r.mu.RUnlock()
			r.handleMock(w, req, ep, params)
			return
		}
	}
	r.mu.RUnlock()

	http.NotFound(w, req)
}

func (r *DynamicRouter) List() []*ManagedEndpoint {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*ManagedEndpoint, len(r.endpoints))
	copy(result, r.endpoints)
	return result
}

func (r *DynamicRouter) Add(ep *ManagedEndpoint) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.endpoints = append(r.endpoints, ep)
}

func (r *DynamicRouter) Update(id string, ep *ManagedEndpoint) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, e := range r.endpoints {
		if e.ID == id {
			ep.ID = id
			r.endpoints[i] = ep
			return true
		}
	}
	return false
}

func (r *DynamicRouter) Delete(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, e := range r.endpoints {
		if e.ID == id {
			r.endpoints = append(r.endpoints[:i], r.endpoints[i+1:]...)
			return true
		}
	}
	return false
}

func matchEndpoint(ep *ManagedEndpoint, method, path string) (map[string]string, bool) {
	if ep.Method != method {
		return nil, false
	}
	pattern := pathParamRe.ReplaceAllString(ep.Path, `([^/]+)`)
	re, err := regexp.Compile("^" + pattern + "$")
	if err != nil {
		return nil, false
	}
	matches := re.FindStringSubmatch(path)
	if matches == nil {
		return nil, false
	}
	paramNames := extractPathParams(ep.Path)
	params := make(map[string]string, len(paramNames))
	for i, name := range paramNames {
		params[name] = matches[i+1]
	}
	return params, true
}

func extractPathParams(pattern string) []string {
	matches := pathParamRe.FindAllStringSubmatch(pattern, -1)
	params := make([]string, 0, len(matches))
	for _, m := range matches {
		params = append(params, m[1])
	}
	return params
}

func (r *DynamicRouter) handleMock(w http.ResponseWriter, req *http.Request, ep *ManagedEndpoint, pathParams map[string]string) {
	rawBody, _ := io.ReadAll(req.Body)
	if len(rawBody) > 0 {
		req.Body = io.NopCloser(bytes.NewBuffer(rawBody))
	}

	ctx := buildRequestContext(req, pathParams)

	if r.verbose {
		fmt.Printf("---\n→ %s %s\n", req.Method, req.URL.RequestURI())
		for k, v := range req.Header {
			fmt.Printf("  %s: %s\n", k, v[0])
		}
		if len(rawBody) > 0 {
			var pretty bytes.Buffer
			if json.Indent(&pretty, rawBody, "", "  ") == nil {
				fmt.Printf("  Body:\n%s\n", pretty.Bytes())
			} else {
				fmt.Printf("  Body: %s\n", rawBody)
			}
		}
	}

	if ep.Delay != "" {
		d, err := time.ParseDuration(ep.Delay)
		if err == nil {
			time.Sleep(d)
		}
	}

	bodyContent := resolveTemplate(ep.Body, ctx)
	body := []byte(bodyContent)

	for k, v := range ep.Headers {
		w.Header().Set(k, resolveTemplate(v, ctx))
	}
	if w.Header().Get("Content-Type") == "" && len(body) > 0 {
		w.Header().Set("Content-Type", "application/json")
	}

	status := ep.Status
	if status == 0 {
		status = http.StatusOK
	}

	if r.verbose {
		delayStr := ""
		if ep.Delay != "" {
			delayStr = fmt.Sprintf(" (%s)", ep.Delay)
		}
		fmt.Printf("← %d%s\n", status, delayStr)
		if len(body) > 0 {
			var pretty bytes.Buffer
			if json.Indent(&pretty, body, "", "  ") == nil {
				fmt.Printf("%s\n", pretty.Bytes())
			} else {
				fmt.Printf("  %s\n", body)
			}
		}
	}

	w.WriteHeader(status)
	if len(body) > 0 {
		w.Write(body)
	}
}
