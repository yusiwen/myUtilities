package k8s

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

type Options struct {
	Secret SecretOptions `cmd:"" name:"secret" aliases:"s" help:"Generate or decode a Kubernetes Secret YAML."`
	Serve  ServeOptions  `cmd:"" name:"serve" help:"Start Kubernetes tools HTTP server."`
}

type SecretOptions struct {
	Name    string   `arg:"" name:"name" help:"Secret name (or path to YAML file when --decode)."`
	Data    []string `arg:"" optional:"" name:"data" help:"key=value pairs."`
	FromEnv string   `short:"f" name:"from-env" help:"Read key=value pairs from .env file."`
	Output  string   `short:"o" name:"output" help:"Output file path."`
	Decode  bool     `short:"d" name:"decode" help:"Decode an existing Secret YAML back to plaintext key=value."`
}

type ServeOptions struct {
	Port int `help:"Port to listen on." default:"8089"`
}

const secretTmpl = `apiVersion: v1
kind: Secret
metadata:
  name: {{.Name}}
type: Opaque
data:
{{- range .Entries}}
  {{.Key}}: {{.Value}}
{{- end}}
`

type entry struct {
	Key   string
	Value string
}

type templateData struct {
	Name    string
	Entries []entry
}

func (o *SecretOptions) Run() error {
	if o.Decode {
		return o.decode()
	}
	return o.encode()
}

func (o *ServeOptions) Run() error {
	mux := http.NewServeMux()
	mux.Handle("/", FrontendHandler())
	RegisterHandlers(mux)
	fmt.Printf("Kubernetes tools server listening on :%d\n", o.Port)
	return http.ListenAndServe(fmt.Sprintf(":%d", o.Port), mux)
}

func RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/api/k8s/secret", handleSecret)
	mux.HandleFunc("/api/k8s/secret/decode", handleSecretDecode)
}

func handleSecret(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Name string            `json:"name"`
		Data map[string]string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if len(req.Data) == 0 {
		http.Error(w, "data is required", http.StatusBadRequest)
		return
	}

	var entries []entry
	// Sort keys for deterministic output
	keys := make([]string, 0, len(req.Data))
	for k := range req.Data {
		keys = append(keys, k)
	}
	for _, k := range keys {
		entries = append(entries, entry{
			Key:   k,
			Value: base64.StdEncoding.EncodeToString([]byte(req.Data[k])),
		})
	}

	tmpl, err := template.New("secret").Parse(secretTmpl)
	if err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, templateData{Name: req.Name, Entries: entries}); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"yaml": buf.String()})
}

func handleSecretDecode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		YAML string `json:"yaml"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	var secret struct {
		Data map[string]string `yaml:"data"`
	}
	if err := yaml.Unmarshal([]byte(req.YAML), &secret); err != nil {
		http.Error(w, fmt.Sprintf("parse YAML: %v", err), http.StatusBadRequest)
		return
	}
	if secret.Data == nil {
		http.Error(w, "no data field found in YAML", http.StatusBadRequest)
		return
	}

	result := make(map[string]string)
	for k, v := range secret.Data {
		decoded, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			result[k] = v // not base64, return as-is
		} else {
			result[k] = string(decoded)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": result})
}

func (o *SecretOptions) decode() error {
	data, err := os.ReadFile(o.Name)
	if err != nil {
		return fmt.Errorf("read YAML: %w", err)
	}

	var secret struct {
		Data map[string]string `yaml:"data"`
	}
	if err := yaml.Unmarshal(data, &secret); err != nil {
		return fmt.Errorf("parse YAML: %w", err)
	}

	out := os.Stdout
	if o.Output != "" {
		f, err := os.Create(o.Output)
		if err != nil {
			return fmt.Errorf("create output: %w", err)
		}
		defer f.Close()
		out = f
	}

	for k, v := range secret.Data {
		decoded, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: invalid base64: %v\n", k, err)
			fmt.Fprintf(out, "%s=%s\n", k, v)
			continue
		}
		fmt.Fprintf(out, "%s=%s\n", k, string(decoded))
	}
	return nil
}

func (o *SecretOptions) encode() error {
	pairs, err := o.resolvePairs()
	if err != nil {
		return err
	}
	if len(pairs) == 0 {
		return fmt.Errorf("no key=value pairs provided")
	}

	var entries []entry
	for _, p := range pairs {
		parts := strings.SplitN(p, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			return fmt.Errorf("invalid pair: %s (expected key=value)", p)
		}
		encoded := base64.StdEncoding.EncodeToString([]byte(parts[1]))
		entries = append(entries, entry{Key: parts[0], Value: encoded})
	}

	tmpl, err := template.New("secret").Parse(secretTmpl)
	if err != nil {
		return fmt.Errorf("template: %w", err)
	}

	out := os.Stdout
	if o.Output != "" {
		f, err := os.Create(o.Output)
		if err != nil {
			return fmt.Errorf("create output: %w", err)
		}
		defer f.Close()
		out = f
	}

	return tmpl.Execute(out, templateData{Name: o.Name, Entries: entries})
}

func (o *SecretOptions) resolvePairs() ([]string, error) {
	if len(o.Data) > 0 {
		return o.Data, nil
	}
	if o.FromEnv != "" {
		data, err := os.ReadFile(o.FromEnv)
		if err != nil {
			return nil, fmt.Errorf("read env file: %w", err)
		}
		return parseEnvFile(string(data)), nil
	}
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		return parseEnvFile(string(data)), nil
	}
	return nil, fmt.Errorf("provide key=value pairs, --from-env, or pipe input")
}

func parseEnvFile(content string) []string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}
