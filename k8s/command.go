package k8s

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"gopkg.in/yaml.v3"
)

type Options struct {
	Secret SecretOptions `cmd:"" name:"secret" aliases:"s" help:"Generate or decode a Kubernetes Secret YAML."`
	Get    GetOptions    `cmd:"" name:"get" help:"List Kubernetes resources (pods, nodes, deployments, services)."`
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
	Port       int    `help:"Port to listen on." default:"8089"`
	Kubeconfig string `name:"kubeconfig" help:"Path to kubeconfig file."`
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

func kubeconfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "mu", "kubeconfig.yaml")
}

func (o *ServeOptions) Run() error {
	if o.Kubeconfig != "" {
		data, err := os.ReadFile(o.Kubeconfig)
		if err != nil {
			return fmt.Errorf("read kubeconfig: %w", err)
		}
		os.WriteFile(kubeconfigPath(), data, 0644)
	}

	mux := http.NewServeMux()
	mux.Handle("/", FrontendHandler())
	RegisterHandlers(mux)
	fmt.Printf("Kubernetes tools server listening on :%d\n", o.Port)
	if _, err := os.Stat(kubeconfigPath()); err == nil {
		fmt.Printf("  Kubeconfig: %s (loaded)\n", kubeconfigPath())
	} else {
		fmt.Printf("  Kubeconfig: not configured\n")
	}
	return http.ListenAndServe(fmt.Sprintf(":%d", o.Port), mux)
}

func RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/api/k8s/secret", handleSecret)
	mux.HandleFunc("/api/k8s/secret/decode", handleSecretDecode)
	mux.HandleFunc("/api/k8s/resources", handleResources)
	mux.HandleFunc("/api/k8s/config", handleConfig)
}

func kubeconfigFromStore() (string, error) {
	data, err := os.ReadFile(kubeconfigPath())
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func kubeconfigToStore(data string) error {
	return os.WriteFile(kubeconfigPath(), []byte(data), 0644)
}

func kubeconfigDelete() {
	os.Remove(kubeconfigPath())
}

func loadK8sClient(kubeconfigContent string) (*kubernetes.Clientset, *clientcmd.ConfigOverrides, error) {
	config, err := clientcmd.NewClientConfigFromBytes([]byte(kubeconfigContent))
	if err != nil {
		return nil, nil, fmt.Errorf("parse kubeconfig: %w", err)
	}
	restConfig, err := config.ClientConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("create rest config: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("create kubernetes client: %w", err)
	}
	return clientset, nil, nil
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		data, err := kubeconfigFromStore()
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"active": false})
			return
		}
		// Parse to get contexts
		rawCfg, err := clientcmd.NewClientConfigFromBytes([]byte(data))
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"active": false})
			return
		}
		cfg, err := rawCfg.RawConfig()
		activeContext := cfg.CurrentContext
		if err != nil {
			activeContext = ""
		}
		var contexts []string
		for name := range cfg.Contexts {
			contexts = append(contexts, name)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"active":         true,
			"kubeconfig":     data,
			"contexts":       contexts,
			"currentContext": activeContext,
		})

	case http.MethodPost:
		var req struct {
			Kubeconfig    string `json:"kubeconfig"`
			Clear         bool   `json:"clear"`
			SwitchContext string `json:"switchContext"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
			return
		}
		if req.Clear {
			kubeconfigDelete()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"active": false})
			return
		}
		if req.SwitchContext != "" {
			data, err := kubeconfigFromStore()
			if err != nil {
				http.Error(w, "no kubeconfig configured", http.StatusBadRequest)
				return
			}
			var cfg map[string]interface{}
			if err := yaml.Unmarshal([]byte(data), &cfg); err != nil {
				http.Error(w, "parse kubeconfig: "+err.Error(), http.StatusBadRequest)
				return
			}
			// Navigate to contexts and set current-context
			if contexts, ok := cfg["contexts"].([]interface{}); ok {
				found := false
				for _, c := range contexts {
					if ctx, ok := c.(map[string]interface{}); ok {
						if name, _ := ctx["name"].(string); name == req.SwitchContext {
							found = true
							break
						}
					}
				}
				if !found {
					http.Error(w, "context not found", http.StatusBadRequest)
					return
				}
			}
			cfg["current-context"] = req.SwitchContext
			updated, _ := yaml.Marshal(cfg)
			if err := kubeconfigToStore(string(updated)); err != nil {
				http.Error(w, "save config: "+err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"active":         true,
				"currentContext": req.SwitchContext,
			})
			return
		}
		if err := kubeconfigToStore(req.Kubeconfig); err != nil {
			http.Error(w, fmt.Sprintf("save config: %v", err), http.StatusInternalServerError)
			return
		}
		// Parse and return context info
		rawCfg, err := clientcmd.NewClientConfigFromBytes([]byte(req.Kubeconfig))
		if err != nil {
			http.Error(w, fmt.Sprintf("parse kubeconfig: %v", err), http.StatusBadRequest)
			return
		}
		cfg, err := rawCfg.RawConfig()
		var contexts []string
		activeContext := cfg.CurrentContext
		if err == nil {
			for name := range cfg.Contexts {
				contexts = append(contexts, name)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"active":         true,
			"contexts":       contexts,
			"currentContext": activeContext,
		})

	default:
		http.Error(w, "GET or POST", http.StatusMethodNotAllowed)
	}
}

func handleResources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Type       string `json:"type"`
		Namespace  string `json:"namespace"`
		Kubeconfig string `json:"kubeconfig"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	kc := req.Kubeconfig
	if kc == "" {
		stored, err := kubeconfigFromStore()
		if err != nil {
			http.Error(w, "kubeconfig not configured; upload or paste your config first", http.StatusBadRequest)
			return
		}
		kc = stored
	}

	cs, _, err := loadK8sClient(kc)
	if err != nil {
		http.Error(w, fmt.Sprintf("kubeconfig error: %v", err), http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	switch strings.ToLower(req.Type) {
	case "pods":
		listPodsJSON(ctx, cs, req.Namespace, w)
	case "nodes":
		listNodesJSON(ctx, cs, w)
	case "deployments":
		listDeploymentsJSON(ctx, cs, req.Namespace, w)
	case "services":
		listServicesJSON(ctx, cs, req.Namespace, w)
	default:
		http.Error(w, "unsupported resource type", http.StatusBadRequest)
	}
}

func listPodsJSON(ctx context.Context, cs *kubernetes.Clientset, namespace string, w http.ResponseWriter) {
	pods, err := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		http.Error(w, fmt.Sprintf("list pods: %v", err), http.StatusInternalServerError)
		return
	}
	columns := []string{"NAMESPACE", "NAME", "READY", "STATUS", "RESTARTS", "AGE"}
	var rows [][]string
	for _, pod := range pods.Items {
		ready := 0
		for _, s := range pod.Status.ContainerStatuses {
			if s.Ready {
				ready++
			}
		}
		restarts := int32(0)
		for _, s := range pod.Status.ContainerStatuses {
			restarts += s.RestartCount
		}
		rows = append(rows, []string{
			pod.Namespace, pod.Name,
			fmt.Sprintf("%d/%d", ready, len(pod.Status.ContainerStatuses)),
			string(pod.Status.Phase),
			fmt.Sprintf("%d", restarts),
			humanAge(pod.CreationTimestamp.Time),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"columns": columns, "rows": rows})
}

func listNodesJSON(ctx context.Context, cs *kubernetes.Clientset, w http.ResponseWriter) {
	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		http.Error(w, fmt.Sprintf("list nodes: %v", err), http.StatusInternalServerError)
		return
	}
	columns := []string{"NAME", "STATUS", "ROLES", "VERSION", "AGE"}
	var rows [][]string
	for _, node := range nodes.Items {
		status := "Ready"
		for _, c := range node.Status.Conditions {
			if c.Type == "Ready" {
				if c.Status != "True" {
					status = string(c.Status)
				}
				break
			}
		}
		var roles []string
		for k, v := range node.Labels {
			if strings.HasPrefix(k, "node-role.kubernetes.io/") && v != "false" {
				roles = append(roles, strings.TrimPrefix(k, "node-role.kubernetes.io/"))
			}
		}
		roleStr := strings.Join(roles, ",")
		if roleStr == "" {
			roleStr = "<none>"
		}
		rows = append(rows, []string{
			node.Name, status, roleStr,
			node.Status.NodeInfo.KubeletVersion,
			humanAge(node.CreationTimestamp.Time),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"columns": columns, "rows": rows})
}

func listDeploymentsJSON(ctx context.Context, cs *kubernetes.Clientset, namespace string, w http.ResponseWriter) {
	deployments, err := cs.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		http.Error(w, fmt.Sprintf("list deployments: %v", err), http.StatusInternalServerError)
		return
	}
	columns := []string{"NAMESPACE", "NAME", "READY", "UP-TO-DATE", "AVAILABLE", "AGE"}
	var rows [][]string
	for _, dep := range deployments.Items {
		rows = append(rows, []string{
			dep.Namespace, dep.Name,
			fmt.Sprintf("%d/%d", dep.Status.ReadyReplicas, dep.Status.Replicas),
			fmt.Sprintf("%d", dep.Status.UpdatedReplicas),
			fmt.Sprintf("%d", dep.Status.AvailableReplicas),
			humanAge(dep.CreationTimestamp.Time),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"columns": columns, "rows": rows})
}

func listServicesJSON(ctx context.Context, cs *kubernetes.Clientset, namespace string, w http.ResponseWriter) {
	svcs, err := cs.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		http.Error(w, fmt.Sprintf("list services: %v", err), http.StatusInternalServerError)
		return
	}
	columns := []string{"NAMESPACE", "NAME", "TYPE", "CLUSTER-IP", "PORT(S)", "AGE"}
	var rows [][]string
	for _, svc := range svcs.Items {
		var ports []string
		for _, p := range svc.Spec.Ports {
			if p.NodePort > 0 {
				ports = append(ports, fmt.Sprintf("%d:%d/%s", p.Port, p.NodePort, p.Protocol))
			} else {
				ports = append(ports, fmt.Sprintf("%d/%s", p.Port, p.Protocol))
			}
		}
		rows = append(rows, []string{
			svc.Namespace, svc.Name, string(svc.Spec.Type),
			svc.Spec.ClusterIP, strings.Join(ports, ","),
			humanAge(svc.CreationTimestamp.Time),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"columns": columns, "rows": rows})
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
