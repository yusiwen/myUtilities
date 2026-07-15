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

func indexPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "mu", "kubeconfigs.yaml")
}

type configIndex struct {
	Active  string            `yaml:"active"`
	Configs map[string]string `yaml:"configs"`
}

func loadIndex() (*configIndex, error) {
	idx := &configIndex{Configs: make(map[string]string)}
	data, err := os.ReadFile(indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			// Migrate old single-file format
			oldPath := filepath.Join(filepath.Dir(indexPath()), "kubeconfig.yaml")
			oldData, oldErr := os.ReadFile(oldPath)
			if oldErr == nil && len(oldData) > 0 {
				rawCfg, parseErr := clientcmd.NewClientConfigFromBytes(oldData)
				if parseErr == nil {
					cfg, _ := rawCfg.RawConfig()
					name := cfg.CurrentContext
					if name == "" {
						name = "default"
					}
					idx.Active = name
					idx.Configs[name] = string(oldData)
					saveIndex(idx)
					os.Remove(oldPath)
				}
			}
			return idx, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(data, idx); err != nil {
		return nil, err
	}
	if idx.Configs == nil {
		idx.Configs = make(map[string]string)
	}
	return idx, nil
}

func saveIndex(idx *configIndex) error {
	data, err := yaml.Marshal(idx)
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath(), data, 0644)
}

func (o *ServeOptions) Run() error {
	if o.Kubeconfig != "" {
		data, err := os.ReadFile(o.Kubeconfig)
		if err != nil {
			return fmt.Errorf("read kubeconfig: %w", err)
		}
		rawCfg, parseErr := clientcmd.NewClientConfigFromBytes(data)
		if parseErr == nil {
			cfg, _ := rawCfg.RawConfig()
			name := cfg.CurrentContext
			if name == "" {
				name = "default"
			}
			idx, _ := loadIndex()
			idx.Active = name
			idx.Configs[name] = string(data)
			saveIndex(idx)
		}
	}

	mux := http.NewServeMux()
	mux.Handle("/", FrontendHandler())
	mux.Handle("/api/k8s/secret", http.HandlerFunc(handleSecret))
	mux.Handle("/api/k8s/secret/decode", http.HandlerFunc(handleSecretDecode))
	mux.Handle("/api/k8s/resources", http.HandlerFunc(handleResources))
	mux.Handle("/api/k8s/namespaces", http.HandlerFunc(handleNamespaces))
	mux.Handle("/api/k8s/config", http.HandlerFunc(handleConfig))
	mux.Handle("/api/k8s/configs/", http.HandlerFunc(handleConfigs))
	fmt.Printf("Kubernetes tools server listening on :%d\n", o.Port)
	idx, _ := loadIndex()
	if idx.Active != "" {
		fmt.Printf("  Active config: %s\n", idx.Active)
		fmt.Printf("  Saved configs: %d\n", len(idx.Configs))
	} else {
		fmt.Printf("  Kubeconfig: not configured\n")
	}
	return http.ListenAndServe(fmt.Sprintf(":%d", o.Port), mux)
}

func RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/api/k8s/secret", handleSecret)
	mux.HandleFunc("/api/k8s/secret/decode", handleSecretDecode)
	mux.HandleFunc("/api/k8s/resources", handleResources)
	mux.HandleFunc("/api/k8s/namespaces", handleNamespaces)
	mux.HandleFunc("/api/k8s/config", handleConfig)
	mux.HandleFunc("/api/k8s/configs/", handleConfigs)
}

func loadK8sClient(kubeconfigContent string) (*kubernetes.Clientset, error) {
	config, err := clientcmd.NewClientConfigFromBytes([]byte(kubeconfigContent))
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}
	restConfig, err := config.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("create rest config: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}
	return clientset, nil
}

func parseKubeconfigMeta(data string) ([]string, string) {
	rawCfg, err := clientcmd.NewClientConfigFromBytes([]byte(data))
	if err != nil {
		return nil, ""
	}
	cfg, err := rawCfg.RawConfig()
	if err != nil {
		return nil, ""
	}
	var contexts []string
	for name := range cfg.Contexts {
		contexts = append(contexts, name)
	}
	return contexts, cfg.CurrentContext
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		idx, _ := loadIndex()
		if idx.Active == "" || idx.Configs[idx.Active] == "" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"active": false,
				"configs": idx.Configs,
			})
			return
		}
		contexts, currentCtx := parseKubeconfigMeta(idx.Configs[idx.Active])
		json.NewEncoder(w).Encode(map[string]interface{}{
			"active":         true,
			"activeName":     idx.Active,
			"configs":        idx.Configs,
			"contexts":       contexts,
			"currentContext": currentCtx,
		})

	case http.MethodPost:
		var req struct {
			Name          string `json:"name"`
			Kubeconfig    string `json:"kubeconfig"`
			Deactivate    bool   `json:"deactivate"`
			SwitchContext string `json:"switchContext"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}

		idx, _ := loadIndex()

		if req.Deactivate {
			idx.Active = ""
			saveIndex(idx)
			json.NewEncoder(w).Encode(map[string]interface{}{"active": false})
			return
		}

		if req.SwitchContext != "" {
			kc, ok := idx.Configs[idx.Active]
			if !ok {
				http.Error(w, `{"error":"no active config"}`, http.StatusBadRequest)
				return
			}
			var cfg map[string]interface{}
			if err := yaml.Unmarshal([]byte(kc), &cfg); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadRequest)
				return
			}
			cfg["current-context"] = req.SwitchContext
			updated, _ := yaml.Marshal(cfg)
			idx.Configs[idx.Active] = string(updated)
			saveIndex(idx)
			contexts, currentCtx := parseKubeconfigMeta(idx.Configs[idx.Active])
			json.NewEncoder(w).Encode(map[string]interface{}{
				"active":         true,
				"activeName":     idx.Active,
				"configs":        idx.Configs,
				"contexts":       contexts,
				"currentContext": currentCtx,
			})
			return
		}

		name := req.Name
		if name == "" {
			_, currentCtx := parseKubeconfigMeta(req.Kubeconfig)
			name = currentCtx
			if name == "" {
				name = "default"
			}
		}
		idx.Active = name
		idx.Configs[name] = req.Kubeconfig
		saveIndex(idx)

		contexts, currentCtx := parseKubeconfigMeta(req.Kubeconfig)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"active":         true,
			"activeName":     name,
			"configs":        idx.Configs,
			"contexts":       contexts,
			"currentContext": currentCtx,
		})

	default:
		http.Error(w, `{"error":"GET or POST"}`, http.StatusMethodNotAllowed)
	}
}

func handleConfigs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// DELETE /api/k8s/configs/{name}
	if r.Method == http.MethodDelete {
		name := strings.TrimPrefix(r.URL.Path, "/api/k8s/configs/")
		if name == "" {
			http.Error(w, `{"error":"name required"}`, http.StatusBadRequest)
			return
		}
		idx, _ := loadIndex()
		if idx.Active == name {
			idx.Active = ""
		}
		delete(idx.Configs, name)
		saveIndex(idx)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"active": idx.Active != "" && idx.Configs[idx.Active] != "",
			"configs": idx.Configs,
		})
		return
	}

	// POST /api/k8s/configs/{name} — activate a saved config
	if r.Method == http.MethodPost {
		name := strings.TrimPrefix(r.URL.Path, "/api/k8s/configs/")
		if name == "" {
			http.Error(w, `{"error":"name required"}`, http.StatusBadRequest)
			return
		}
		idx, _ := loadIndex()
		if _, ok := idx.Configs[name]; !ok {
			http.Error(w, `{"error":"config not found"}`, http.StatusNotFound)
			return
		}
		idx.Active = name
		saveIndex(idx)

		contexts, currentCtx := parseKubeconfigMeta(idx.Configs[name])
		json.NewEncoder(w).Encode(map[string]interface{}{
			"active":         true,
			"activeName":     name,
			"configs":        idx.Configs,
			"contexts":       contexts,
			"currentContext": currentCtx,
		})
		return
	}

	http.Error(w, `{"error":"POST or DELETE"}`, http.StatusMethodNotAllowed)
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
		idx, _ := loadIndex()
		if idx.Active == "" || idx.Configs[idx.Active] == "" {
			http.Error(w, "kubeconfig not configured; upload or paste your config first", http.StatusBadRequest)
			return
		}
		kc = idx.Configs[idx.Active]
	}

	cs, err := loadK8sClient(kc)
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

func handleNamespaces(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"GET required"}`, http.StatusMethodNotAllowed)
		return
	}

	idx, _ := loadIndex()
	if idx.Active == "" || idx.Configs[idx.Active] == "" {
		http.Error(w, `{"error":"kubeconfig not configured"}`, http.StatusBadRequest)
		return
	}

	cs, err := loadK8sClient(idx.Configs[idx.Active])
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadRequest)
		return
	}

	nsList, err := cs.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"list namespaces: %v"}`, err), http.StatusInternalServerError)
		return
	}

	var namespaces []string
	for _, ns := range nsList.Items {
		namespaces = append(namespaces, ns.Name)
	}
	json.NewEncoder(w).Encode(namespaces)
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
