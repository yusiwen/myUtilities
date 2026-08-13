package misc

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	coremisc "github.com/yusiwen/myUtilities/internal/core/misc"
)

type Options struct {
	UUID  UUIDOptions      `cmd:"" name:"uuid" help:"Generate UUIDs."`
	JSON  JSONOptions      `cmd:"" name:"json" help:"Format, validate, or minify JSON."`
	TS    TimestampOptions `cmd:"" name:"timestamp" aliases:"ts" help:"Convert timestamps."`
	Hash  HashOptions      `cmd:"" name:"hash" help:"Compute hash of text or file."`
	Serve ServeOptions     `cmd:"" name:"serve" help:"Start Misc tools HTTP server."`
}

type UUIDOptions struct {
	Count int `arg:"" optional:"" name:"count" help:"Number of UUIDs to generate." default:"1"`
}

type JSONOptions struct {
	Format   JSONFormatCmd   `cmd:"" name:"format" help:"Format JSON."`
	Validate JSONValidateCmd `cmd:"" name:"validate" help:"Validate JSON."`
	Minify   JSONMinifyCmd   `cmd:"" name:"minify" help:"Minify JSON."`
}

type JSONFormatCmd struct {
	Input string `arg:"" optional:"" name:"input" help:"JSON string (or pipe from stdin)."`
}

type JSONValidateCmd struct {
	Input string `arg:"" optional:"" name:"input" help:"JSON string (or pipe from stdin)."`
}

type JSONMinifyCmd struct {
	Input string `arg:"" optional:"" name:"input" help:"JSON string (or pipe from stdin)."`
}

type TimestampOptions struct {
	Input string `arg:"" optional:"" name:"input" help:"Unix timestamp, ISO date, or empty for now."`
}

type HashOptions struct {
	Alg   string `arg:"" name:"alg" help:"Hash algorithm: md5|sha256|sha512"`
	Input string `arg:"" optional:"" name:"input" help:"Text to hash (or pipe from stdin, or --file)."`
	File  string `short:"f" name:"file" help:"Path to file to hash."`
}

type ServeOptions struct {
	Port int `help:"Port to listen on." default:"8090"`
}

func (o *UUIDOptions) Run() error {
	for i := 0; i < o.Count; i++ {
		u, err := coremisc.GenUUID()
		if err != nil {
			return err
		}
		fmt.Println(u)
	}
	return nil
}

func (o *JSONFormatCmd) Run() error {
	input, err := readInput(o.Input)
	if err != nil {
		return err
	}
	out, err := coremisc.FormatJSON(input)
	if err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	fmt.Println(out)
	return nil
}

func (o *JSONValidateCmd) Run() error {
	input, err := readInput(o.Input)
	if err != nil {
		return err
	}
	out, err := coremisc.ValidateJSON(input)
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}

func (o *JSONMinifyCmd) Run() error {
	input, err := readInput(o.Input)
	if err != nil {
		return err
	}
	out, err := coremisc.MinifyJSON(input)
	if err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	fmt.Println(out)
	return nil
}

func (o *TimestampOptions) Run() error {
	t, err := coremisc.ConvertTimestamp(o.Input)
	if err != nil {
		return err
	}
	fmt.Println(t.Unix())
	return nil
}

func (o *HashOptions) Run() error {
	var data []byte
	if o.File != "" {
		d, err := os.ReadFile(o.File)
		if err != nil {
			return fmt.Errorf("read file: %w", err)
		}
		data = d
	} else {
		input, err := readInput(o.Input)
		if err != nil {
			return err
		}
		data = []byte(input)
	}
	out, err := coremisc.Hash(o.Alg, string(data))
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}

func readInput(text string) (string, error) {
	if text != "" {
		return text, nil
	}
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		return strings.TrimRight(string(data), "\n\r"), err
	}
	return "", fmt.Errorf("input required; pipe input or provide as argument")
}

func (o *ServeOptions) Run() error {
	mux := http.NewServeMux()
	mux.Handle("/", FrontendHandler())
	RegisterHandlers(mux)
	fmt.Printf("Misc tools server listening on :%d\n", o.Port)
	return http.ListenAndServe(fmt.Sprintf(":%d", o.Port), mux)
}

var trackersCache = coremisc.DefaultTrackers()

func RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/api/misc/json/format", handleJSONOp(coremisc.FormatJSON))
	mux.HandleFunc("/api/misc/json/validate", handleJSONOp(coremisc.ValidateJSON))
	mux.HandleFunc("/api/misc/json/minify", handleJSONOp(coremisc.MinifyJSON))
	mux.HandleFunc("/api/misc/uuid", handleUUID)
	mux.HandleFunc("/api/misc/timestamp", handleTimestamp)
	mux.HandleFunc("/api/misc/hash/", handleHash)
	mux.HandleFunc("/api/misc/trackers", handleTrackers)
}

type jsonFunc func(string) (string, error)

func handleJSONOp(fn jsonFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}
		result, err := fn(req.Input)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"result": result})
	}
}

func handleUUID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}
	if req.Count < 1 {
		req.Count = 1
	}
	if req.Count > 100 {
		req.Count = 100
	}
	uuids := make([]string, req.Count)
	for i := 0; i < req.Count; i++ {
		u, err := coremisc.GenUUID()
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
			return
		}
		uuids[i] = u
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"uuids": uuids})
}

func handleTimestamp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	t, err := coremisc.ConvertTimestamp(req.Input)
	if err != nil {
		http.Error(w, `{"error":"unable to parse time"}`, http.StatusBadRequest)
		return
	}
	var result string
	if req.Input == "" {
		result = fmt.Sprintf("%d", t.Unix())
	} else {
		result = fmt.Sprintf("%d (%s)", t.Unix(), t.Format(time.RFC3339))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": result})
}

func handleHash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}
	alg := strings.TrimPrefix(r.URL.Path, "/api/misc/hash/")
	var req struct {
		Input string `json:"input"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	result, err := coremisc.Hash(alg, req.Input)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": result})
}

func handleTrackers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"GET required"}`, http.StatusMethodNotAllowed)
		return
	}
	refresh := r.URL.Query().Get("refresh") == "1"
	content, err := trackersCache.Get(refresh)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"content": content})
}
