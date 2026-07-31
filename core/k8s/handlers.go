package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegisterHandlers registers all k8s API routes on the given mux.
func RegisterHandlers(mux *http.ServeMux, configDir string) {
	if configDir != "" {
		SetConfigDir(configDir)
	}
	mux.HandleFunc("/api/k8s/secret", handleSecret)
	mux.HandleFunc("/api/k8s/secret/decode", handleSecretDecode)
	mux.HandleFunc("/api/k8s/resources", handleResources)
	mux.HandleFunc("/api/k8s/namespaces", handleNamespaces)
	mux.HandleFunc("/api/k8s/describe", handleDescribe)
	mux.HandleFunc("/api/k8s/config", handleConfig)
	mux.HandleFunc("/api/k8s/configs/", handleConfigs)
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		idx, _ := LoadIndex()
		if idx.Active == "" || idx.Configs[idx.Active] == "" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"active":  false,
				"configs": idx.Configs,
			})
			return
		}
		contexts, currentCtx := ParseKubeconfigMeta(idx.Configs[idx.Active])
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

		idx, _ := LoadIndex()

		if req.Deactivate {
			idx.Active = ""
			SaveIndex(idx)
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
			SaveIndex(idx)
			contexts, currentCtx := ParseKubeconfigMeta(idx.Configs[idx.Active])
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
			_, currentCtx := ParseKubeconfigMeta(req.Kubeconfig)
			name = currentCtx
			if name == "" {
				name = "default"
			}
		}
		idx.Active = name
		idx.Configs[name] = req.Kubeconfig
		SaveIndex(idx)

		contexts, currentCtx := ParseKubeconfigMeta(req.Kubeconfig)
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

	if r.Method == http.MethodDelete {
		name := strings.TrimPrefix(r.URL.Path, "/api/k8s/configs/")
		if name == "" {
			http.Error(w, `{"error":"name required"}`, http.StatusBadRequest)
			return
		}
		idx, _ := LoadIndex()
		if idx.Active == name {
			idx.Active = ""
		}
		delete(idx.Configs, name)
		SaveIndex(idx)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"active":  idx.Active != "" && idx.Configs[idx.Active] != "",
			"configs": idx.Configs,
		})
		return
	}

	if r.Method == http.MethodPost {
		name := strings.TrimPrefix(r.URL.Path, "/api/k8s/configs/")
		if name == "" {
			http.Error(w, `{"error":"name required"}`, http.StatusBadRequest)
			return
		}
		idx, _ := LoadIndex()
		if _, ok := idx.Configs[name]; !ok {
			http.Error(w, `{"error":"config not found"}`, http.StatusNotFound)
			return
		}
		idx.Active = name
		SaveIndex(idx)

		contexts, currentCtx := ParseKubeconfigMeta(idx.Configs[name])
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
		Metrics    bool   `json:"metrics"`
	}
	var err error
	if err = json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	kc := req.Kubeconfig
	if kc == "" {
		kc, err = ActiveKubeconfig()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	cs, err := LoadClient(kc)
	if err != nil {
		http.Error(w, fmt.Sprintf("kubeconfig error: %v", err), http.StatusBadRequest)
		return
	}

	ctx := context.Background()

	switch strings.ToLower(req.Type) {
	case "pods":
		ListPodsJSON(ctx, cs, req.Namespace, req.Metrics, w)
	case "nodes":
		ListNodesJSON(ctx, cs, req.Metrics, w)
	case "deployments":
		ListDeploymentsJSON(ctx, cs, req.Namespace, w)
	case "services":
		ListServicesJSON(ctx, cs, req.Namespace, w)
	case "configmaps", "cm":
		ListConfigMapsJSON(ctx, cs, req.Namespace, w)
	case "namespaces", "ns":
		ListNamespacesJSON(ctx, cs, w)
	case "statefulsets", "sts":
		ListStatefulSetsJSON(ctx, cs, req.Namespace, w)
	case "daemonsets", "ds":
		ListDaemonSetsJSON(ctx, cs, req.Namespace, w)
	case "ingresses", "ing":
		ListIngressesJSON(ctx, cs, req.Namespace, w)
	case "secrets":
		ListSecretsJSON(ctx, cs, req.Namespace, w)
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

	kc, err := ActiveKubeconfig()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadRequest)
		return
	}

	cs, err := LoadClient(kc)
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

func handleDescribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Type      string `json:"type"`
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	kc, err := ActiveKubeconfig()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadRequest)
		return
	}
	cs, err := LoadClient(kc)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadRequest)
		return
	}

	ctx := context.Background()
	var text string
	switch strings.ToLower(req.Type) {
	case "pod", "pods":
		text, err = DescribePod(ctx, cs, req.Namespace, req.Name)
	case "node", "nodes":
		text, err = DescribeNode(ctx, cs, req.Name)
	case "deployment", "deployments":
		text, err = DescribeDeployment(ctx, cs, req.Namespace, req.Name)
	case "service", "services":
		text, err = DescribeService(ctx, cs, req.Namespace, req.Name)
	case "configmap", "configmaps", "cm":
		text, err = DescribeConfigMap(ctx, cs, req.Namespace, req.Name)
	case "namespace", "namespaces", "ns":
		text, err = DescribeNamespace(ctx, cs, req.Name)
	case "statefulset", "statefulsets", "sts":
		text, err = DescribeStatefulSet(ctx, cs, req.Namespace, req.Name)
	case "daemonset", "daemonsets", "ds":
		text, err = DescribeDaemonSet(ctx, cs, req.Namespace, req.Name)
	case "ingress", "ingresses", "ing":
		text, err = DescribeIngress(ctx, cs, req.Namespace, req.Name)
	case "secret", "secrets":
		text, err = DescribeSecret(ctx, cs, req.Namespace, req.Name)
	default:
		http.Error(w, `{"error":"unsupported resource type"}`, http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"describe": text})
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

	yamlOut, err := EncodeSecretYAMLMap(req.Name, req.Data)
	if err != nil {
		http.Error(w, fmt.Sprintf("render: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"yaml": yamlOut})
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

	result, err := DecodeSecretYAML([]byte(req.YAML))
	if err != nil {
		http.Error(w, fmt.Sprintf("parse YAML: %v", err), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": result})
}
