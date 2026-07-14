package k8s

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type GetOptions struct {
	Resource   string `arg:"" name:"resource" help:"Resource type: pods|nodes|deployments|services"`
	Namespace  string `short:"n" name:"namespace" help:"Kubernetes namespace (default: all namespaces)."`
	Context    string `name:"context" help:"Kubeconfig context name (default: current-context)."`
	Kubeconfig string `name:"kubeconfig" help:"Path to kubeconfig file."`
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

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("create kubernetes client: %w", err)
	}

	ctx := context.Background()
	ns := o.Namespace

	switch strings.ToLower(o.Resource) {
	case "pods":
		return listPods(ctx, clientset, ns)
	case "nodes":
		return listNodes(ctx, clientset)
	case "deployments":
		return listDeployments(ctx, clientset, ns)
	case "services":
		return listServices(ctx, clientset, ns)
	default:
		return fmt.Errorf("unsupported resource type: %s (supported: pods, nodes, deployments, services)", o.Resource)
	}
}

func listPods(ctx context.Context, cs *kubernetes.Clientset, namespace string) error {
	pods, err := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list pods: %w", err)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAMESPACE\tNAME\tREADY\tSTATUS\tRESTARTS\tAGE")
	for _, pod := range pods.Items {
		ready := countReady(pod.Status.ContainerStatuses)
		restarts := countRestarts(pod.Status.ContainerStatuses)
		fmt.Fprintf(w, "%s\t%s\t%d/%d\t%s\t%d\t%s\n",
			pod.Namespace, pod.Name,
			ready, len(pod.Status.ContainerStatuses),
			pod.Status.Phase, restarts,
			humanAge(pod.CreationTimestamp.Time),
		)
	}
	return w.Flush()
}

func listNodes(ctx context.Context, cs *kubernetes.Clientset) error {
	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATUS\tROLES\tVERSION\tAGE")
	for _, node := range nodes.Items {
		status := "Ready"
		for _, c := range node.Status.Conditions {
			if c.Type == corev1.NodeReady {
				if c.Status != corev1.ConditionTrue {
					status = string(c.Status)
				}
				break
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			node.Name, status, extractRoles(node.Labels),
			node.Status.NodeInfo.KubeletVersion,
			humanAge(node.CreationTimestamp.Time),
		)
	}
	return w.Flush()
}

func listDeployments(ctx context.Context, cs *kubernetes.Clientset, namespace string) error {
	deployments, err := cs.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list deployments: %w", err)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAMESPACE\tNAME\tREADY\tUP-TO-DATE\tAVAILABLE\tAGE")
	for _, dep := range deployments.Items {
		fmt.Fprintf(w, "%s\t%s\t%d/%d\t%d\t%d\t%s\n",
			dep.Namespace, dep.Name,
			dep.Status.ReadyReplicas, dep.Status.Replicas,
			dep.Status.UpdatedReplicas, dep.Status.AvailableReplicas,
			humanAge(dep.CreationTimestamp.Time),
		)
	}
	return w.Flush()
}

func listServices(ctx context.Context, cs *kubernetes.Clientset, namespace string) error {
	svcs, err := cs.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list services: %w", err)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "NAMESPACE\tNAME\tTYPE\tCLUSTER-IP\tPORT(S)\tAGE")
	for _, svc := range svcs.Items {
		ports := formatServicePorts(svc.Spec.Ports)
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			svc.Namespace, svc.Name, svc.Spec.Type, svc.Spec.ClusterIP,
			ports, humanAge(svc.CreationTimestamp.Time),
		)
	}
	return w.Flush()
}

func countReady(statuses []corev1.ContainerStatus) int {
	ready := 0
	for _, s := range statuses {
		if s.Ready {
			ready++
		}
	}
	return ready
}

func countRestarts(statuses []corev1.ContainerStatus) int {
	total := int32(0)
	for _, s := range statuses {
		total += s.RestartCount
	}
	return int(total)
}

func extractRoles(labels map[string]string) string {
	var roles []string
	for k, v := range labels {
		if strings.HasPrefix(k, "node-role.kubernetes.io/") && v != "false" {
			role := strings.TrimPrefix(k, "node-role.kubernetes.io/")
			roles = append(roles, role)
		}
	}
	if len(roles) == 0 {
		return "<none>"
	}
	return strings.Join(roles, ",")
}

func formatServicePorts(ports []corev1.ServicePort) string {
	var parts []string
	for _, p := range ports {
		if p.NodePort > 0 {
			parts = append(parts, fmt.Sprintf("%d:%d/%s", p.Port, p.NodePort, p.Protocol))
		} else {
			parts = append(parts, fmt.Sprintf("%d/%s", p.Port, p.Protocol))
		}
	}
	return strings.Join(parts, ",")
}

func humanAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/24/365))
	}
}
