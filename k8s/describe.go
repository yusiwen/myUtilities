package k8s

import (
	"context"
	"fmt"
	"os"
	"strings"

	corek8s "github.com/yusiwen/myUtilities/core/k8s"
	"k8s.io/client-go/tools/clientcmd"
)

type DescribeOptions struct {
	Resource   string `arg:"" name:"resource" help:"Resource type: pods|nodes|deployments|services|configmaps|namespaces|statefulsets|daemonsets|ingresses|secrets"`
	Name       string `arg:"" name:"name" help:"Resource name."`
	Namespace  string `short:"n" name:"namespace" help:"Kubernetes namespace."`
	Context    string `name:"context" help:"Kubeconfig context name (default: current-context)."`
	Kubeconfig string `name:"kubeconfig" help:"Path to kubeconfig file."`
}

func (o *DescribeOptions) Run() error {
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

	var text string
	switch strings.ToLower(o.Resource) {
	case "pod", "pods":
		text, err = corek8s.DescribePod(ctx, clientset, o.Namespace, o.Name)
	case "node", "nodes":
		text, err = corek8s.DescribeNode(ctx, clientset, o.Name)
	case "deployment", "deployments":
		text, err = corek8s.DescribeDeployment(ctx, clientset, o.Namespace, o.Name)
	case "service", "services":
		text, err = corek8s.DescribeService(ctx, clientset, o.Namespace, o.Name)
	case "configmap", "configmaps", "cm":
		text, err = corek8s.DescribeConfigMap(ctx, clientset, o.Namespace, o.Name)
	case "namespace", "namespaces", "ns":
		text, err = corek8s.DescribeNamespace(ctx, clientset, o.Name)
	case "statefulset", "statefulsets", "sts":
		text, err = corek8s.DescribeStatefulSet(ctx, clientset, o.Namespace, o.Name)
	case "daemonset", "daemonsets", "ds":
		text, err = corek8s.DescribeDaemonSet(ctx, clientset, o.Namespace, o.Name)
	case "ingress", "ingresses", "ing":
		text, err = corek8s.DescribeIngress(ctx, clientset, o.Namespace, o.Name)
	case "secret", "secrets":
		text, err = corek8s.DescribeSecret(ctx, clientset, o.Namespace, o.Name)
	default:
		return fmt.Errorf("unsupported resource type: %s (supported: pods, nodes, deployments, services, configmaps, namespaces, statefulsets, daemonsets, ingresses, secrets)", o.Resource)
	}
	if err != nil {
		return err
	}
	fmt.Fprint(os.Stdout, text)
	return nil
}
