package mock

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

func (o *DynamicServerOptions) Run() error {
	o.Method = strings.ToUpper(o.Method)
	if o.Method != http.MethodGet && o.Method != http.MethodPost {
		return fmt.Errorf("unsupported method %s, only GET and POST are allowed", o.Method)
	}

	mux := http.NewServeMux()
	mux.HandleFunc(o.Path, o.handler)

	fmt.Printf("Dynamic mock server listening at :%d\n", o.Port)
	fmt.Printf("  Matching: %s %s\n", o.Method, o.Path)
	fmt.Printf("  Response: %s\n", o.Resp)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", o.Port), mux); err != nil {
		return fmt.Errorf("server listen failed: %v", err)
	}
	return nil
}

func (o *DynamicServerOptions) handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != o.Method {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	respBody, err := os.ReadFile(o.Resp)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error": "read response file failed: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(respBody)
}
