package misc

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Options struct {
	UUID    UUIDOptions    `cmd:"" name:"uuid" help:"Generate UUIDs."`
	JSON    JSONOptions    `cmd:"" name:"json" help:"Format, validate, or minify JSON."`
	TS      TimestampOptions `cmd:"" name:"timestamp" aliases:"ts" help:"Convert timestamps."`
	Hash    HashOptions    `cmd:"" name:"hash" help:"Compute hash of text or file."`
	Serve   ServeOptions   `cmd:"" name:"serve" help:"Start Misc tools HTTP server."`
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
		u, err := genUUID()
		if err != nil {
			return err
		}
		fmt.Println(u)
	}
	return nil
}

func genUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate uuid: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func (o *JSONFormatCmd) Run() error {
	input, err := readInput(o.Input)
	if err != nil {
		return err
	}
	var v interface{}
	if err := json.Unmarshal([]byte(input), &v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	out, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(out))
	return nil
}

func (o *JSONValidateCmd) Run() error {
	input, err := readInput(o.Input)
	if err != nil {
		return err
	}
	if json.Valid([]byte(input)) {
		fmt.Println("✅ Valid JSON")
	} else {
		fmt.Println("❌ Invalid JSON")
	}
	return nil
}

func (o *JSONMinifyCmd) Run() error {
	input, err := readInput(o.Input)
	if err != nil {
		return err
	}
	var v interface{}
	if err := json.Unmarshal([]byte(input), &v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	var buf bytes.Buffer
	json.Compact(&buf, []byte(input))
	fmt.Println(buf.String())
	return nil
}

func (o *TimestampOptions) Run() error {
	if o.Input == "" {
		fmt.Println(time.Now().Unix())
		return nil
	}
	// Try common date formats first
	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
		"2006/01/02",
		time.RFC1123,
	}
	t, err := time.Parse(formats[0], o.Input)
	if err != nil {
		for _, f := range formats {
			t, err = time.Parse(f, o.Input)
			if err == nil {
				break
			}
		}
	}
	if err != nil {
		// Try Unix timestamp (only if the input looks like a pure number)
		var sec int64
		if _, e := fmt.Sscanf(o.Input, "%d", &sec); e == nil && fmt.Sprint(sec) == o.Input {
			t = time.Unix(sec, 0)
		} else {
			return fmt.Errorf("unable to parse time: %s", o.Input)
		}
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

	var h hash.Hash
	switch o.Alg {
	case "md5":
		h = md5.New()
	case "sha256":
		h = sha256.New()
	case "sha512":
		h = sha512.New()
	default:
		return fmt.Errorf("unsupported algorithm: %s", o.Alg)
	}
	h.Write(data)
	fmt.Printf("%x\n", h.Sum(nil))
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

func RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/api/misc/json/format", handleJSONOp(formatJSON))
	mux.HandleFunc("/api/misc/json/validate", handleJSONOp(validateJSON))
	mux.HandleFunc("/api/misc/json/minify", handleJSONOp(minifyJSON))
	mux.HandleFunc("/api/misc/uuid", handleUUID)
	mux.HandleFunc("/api/misc/timestamp", handleTimestamp)
	mux.HandleFunc("/api/misc/hash/", handleHash)
}

type jsonFunc func(string) (string, error)

func formatJSON(input string) (string, error) {
	var v interface{}
	if err := json.Unmarshal([]byte(input), &v); err != nil {
		return "", err
	}
	out, _ := json.MarshalIndent(v, "", "  ")
	return string(out), nil
}

func validateJSON(input string) (string, error) {
	if json.Valid([]byte(input)) {
		return "✅ Valid JSON", nil
	}
	return "❌ Invalid JSON", nil
}

func minifyJSON(input string) (string, error) {
	var v interface{}
	if err := json.Unmarshal([]byte(input), &v); err != nil {
		return "", err
	}
	var buf bytes.Buffer
	json.Compact(&buf, []byte(input))
	return buf.String(), nil
}

func handleJSONOp(fn jsonFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
			return
		}
		var req struct{ Input string `json:"input"` }
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
	var req struct{ Count int `json:"count"` }
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
		u, err := genUUID()
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
	var req struct{ Input string `json:"input"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	if req.Input == "" {
		result := fmt.Sprintf("%d", time.Now().Unix())
		json.NewEncoder(w).Encode(map[string]string{"result": result})
		return
	}

	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
		"2006/01/02",
	}
	t, err := time.Parse(formats[0], req.Input)
	if err != nil {
		for _, f := range formats {
			t, err = time.Parse(f, req.Input)
			if err == nil {
				break
			}
		}
	}
	if err != nil {
		var sec int64
		if _, e := fmt.Sscanf(req.Input, "%d", &sec); e == nil && fmt.Sprint(sec) == req.Input {
			t = time.Unix(sec, 0)
		} else {
			http.Error(w, fmt.Sprintf(`{"error":"unable to parse time"}`), http.StatusBadRequest)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": fmt.Sprintf("%d (%s)", t.Unix(), t.Format(time.RFC3339))})
}

func handleHash(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}
	alg := strings.TrimPrefix(r.URL.Path, "/api/misc/hash/")
	var req struct{ Input string `json:"input"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	var h hash.Hash
	switch alg {
	case "md5":
		h = md5.New()
	case "sha256":
		h = sha256.New()
	case "sha512":
		h = sha512.New()
	default:
		http.Error(w, `{"error":"unsupported algorithm"}`, http.StatusBadRequest)
		return
	}
	h.Write([]byte(req.Input))
	result := fmt.Sprintf("%x", h.Sum(nil))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"result": result})
}
