package k8s

import (
	"context"
	"fmt"
	"os"
	"strings"

	corek8s "github.com/yusiwen/myUtilities/internal/core/k8s"
	"k8s.io/client-go/tools/clientcmd"
)

type GetOptions struct {
	Resource   string `arg:"" name:"resource" help:"Resource type: pods|nodes|deployments|services"`
	Namespace  string `short:"n" name:"namespace" help:"Kubernetes namespace (default: all namespaces)."`
	Context    string `name:"context" help:"Kubeconfig context name (default: current-context)."`
	Kubeconfig string `name:"kubeconfig" help:"Path to kubeconfig file."`
	Metrics    bool   `name:"metrics" help:"Show CPU/memory usage (requires metrics-server)."`
}

func (o *GetOptions) Run() error {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if o.Kubeconfig != "" {
		loadingRules.ExplicitPath = o.Kubeconfig
	}

	configOverrides := &clientcmd.ConfigOverrides{}
	if o.Context != "" {
		configOverrides.CurrentContext = o.Context
	}

	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	restConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return fmt.Errorf("load kubeconfig: %w", err)
	}

	clientset, err := corek8s.ClientFromConfig(restConfig)
	if err != nil {
		return fmt.Errorf("create kubernetes client: %w", err)
	}

	ctx := context.Background()
	ns := o.Namespace

	switch strings.ToLower(o.Resource) {
	case "pods":
		return corek8s.ListPods(os.Stdout, ctx, clientset, ns, o.Metrics)
	case "nodes":
		return corek8s.ListNodes(os.Stdout, ctx, clientset, o.Metrics)
	case "deployments":
		return corek8s.ListDeployments(os.Stdout, ctx, clientset, ns)
	case "services":
		return corek8s.ListServices(os.Stdout, ctx, clientset, ns)
	case "configmaps", "cm":
		return corek8s.ListConfigMaps(os.Stdout, ctx, clientset, ns)
	case "namespaces", "ns":
		return corek8s.ListNamespaces(os.Stdout, ctx, clientset)
	case "statefulsets", "sts":
		return corek8s.ListStatefulSets(os.Stdout, ctx, clientset, ns)
	case "daemonsets", "ds":
		return corek8s.ListDaemonSets(os.Stdout, ctx, clientset, ns)
	case "ingresses", "ing":
		return corek8s.ListIngresses(os.Stdout, ctx, clientset, ns)
	case "secrets":
		return corek8s.ListSecrets(os.Stdout, ctx, clientset, ns)
	default:
		return fmt.Errorf("unsupported resource type: %s (supported: pods, nodes, deployments, services, configmaps, namespaces, statefulsets, daemonsets, ingresses, secrets)", o.Resource)
	}
}
