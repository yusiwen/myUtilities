package k8s

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type DescribeOptions struct {
	Resource   string `arg:"" name:"resource" help:"Resource type: pods|nodes|deployments|services"`
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
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("create kubernetes client: %w", err)
	}
	ctx := context.Background()

	var text string
	switch strings.ToLower(o.Resource) {
	case "pod", "pods":
		text, err = describePod(ctx, clientset, o.Namespace, o.Name)
	case "node", "nodes":
		text, err = describeNode(ctx, clientset, o.Name)
	case "deployment", "deployments":
		text, err = describeDeployment(ctx, clientset, o.Namespace, o.Name)
	case "service", "services":
		text, err = describeService(ctx, clientset, o.Namespace, o.Name)
	default:
		return fmt.Errorf("unsupported resource type: %s (supported: pods, nodes, deployments, services)", o.Resource)
	}
	if err != nil {
		return err
	}
	fmt.Print(text)
	return nil
}

func describePod(ctx context.Context, cs *kubernetes.Clientset, namespace, name string) (string, error) {
	if namespace == "" {
		namespace = "default"
	}
	pod, err := cs.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get pod: %w", err)
	}
	var b bytes.Buffer
	w := func(format string, args ...interface{}) { fmt.Fprintf(&b, format, args...) }

	w("Name:         %s\n", pod.Name)
	w("Namespace:    %s\n", pod.Namespace)
	if pod.Spec.NodeName != "" {
		w("Node:         %s\n", pod.Spec.NodeName)
	}
	w("Labels:       %s\n", formatLabels(pod.Labels))
	w("Status:       %s\n", pod.Status.Phase)
	w("IP:           %s\n", pod.Status.PodIP)
	if len(pod.Status.ContainerStatuses) > 0 {
		w("Containers:\n")
		for _, c := range pod.Spec.Containers {
			cs := findContainerStatus(pod.Status.ContainerStatuses, c.Name)
			ready := "not running"
			if cs != nil && cs.Ready {
				ready = "running"
			}
			w("  Name:    %s (%s)\n", c.Name, ready)
			w("  Image:   %s\n", c.Image)
			if len(c.Ports) > 0 {
				var ports []string
				for _, p := range c.Ports {
					proto := string(p.Protocol)
					if proto == "" {
						proto = "TCP"
					}
					ports = append(ports, fmt.Sprintf("%d/%s", p.ContainerPort, proto))
				}
				w("  Ports:   %s\n", strings.Join(ports, ", "))
			}
			if c.Resources.Requests != nil || c.Resources.Limits != nil {
				w("  Resources:\n")
				formatResourceList(&b, "    Requests:", c.Resources.Requests)
				formatResourceList(&b, "    Limits:  ", c.Resources.Limits)
			}
		}
	}
	if len(pod.Status.Conditions) > 0 {
		w("Conditions:\n")
		for _, c := range pod.Status.Conditions {
			w("  %-20s %s   %s\n", c.Type, c.Status, c.Reason)
		}
	}
	events, _ := getEvents(cs, pod.Namespace, pod.Name, "Pod")
	if events != "" {
		w("\nEvents:\n%s", events)
	}
	return b.String(), nil
}

func describeNode(ctx context.Context, cs *kubernetes.Clientset, name string) (string, error) {
	node, err := cs.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get node: %w", err)
	}
	var b bytes.Buffer
	w := func(format string, args ...interface{}) { fmt.Fprintf(&b, format, args...) }

	w("Name:     %s\n", node.Name)
	w("Status:   ")
	for _, c := range node.Status.Conditions {
		if c.Type == corev1.NodeReady {
			if c.Status == corev1.ConditionTrue {
				w("Ready")
			} else {
				w(string(c.Status))
			}
			break
		}
	}
	w("\n")
	w("Roles:    %s\n", extractRoles(node.Labels))
	w("Version:  %s\n", node.Status.NodeInfo.KubeletVersion)
	w("OS/Arch:  %s/%s\n", node.Status.NodeInfo.OperatingSystem, node.Status.NodeInfo.Architecture)
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			w("IP:       %s\n", addr.Address)
			break
		}
	}
	if node.Spec.PodCIDR != "" {
		w("Pod CIDR: %s\n", node.Spec.PodCIDR)
	}
	w("\nCapacity:\n")
	for k, v := range node.Status.Capacity {
		w("  %-12s %s\n", k, v.String())
	}
	w("\nAllocatable:\n")
	for k, v := range node.Status.Allocatable {
		w("  %-12s %s\n", k, v.String())
	}
	return b.String(), nil
}

func describeDeployment(ctx context.Context, cs *kubernetes.Clientset, namespace, name string) (string, error) {
	if namespace == "" {
		namespace = "default"
	}
	dep, err := cs.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get deployment: %w", err)
	}
	var b bytes.Buffer
	w := func(format string, args ...interface{}) { fmt.Fprintf(&b, format, args...) }

	w("Name:         %s\n", dep.Name)
	w("Namespace:    %s\n", dep.Namespace)
	w("Labels:       %s\n", formatLabels(dep.Labels))
	w("Replicas:     %d desired | %d updated | %d ready | %d available\n",
		dep.Status.Replicas, dep.Status.UpdatedReplicas,
		dep.Status.ReadyReplicas, dep.Status.AvailableReplicas)
	if dep.Spec.Strategy.Type != "" {
		w("Strategy:     %s\n", dep.Spec.Strategy.Type)
	}
	if len(dep.Spec.Template.Spec.Containers) > 0 {
		w("Containers:\n")
		for _, c := range dep.Spec.Template.Spec.Containers {
			w("  Name:    %s\n", c.Name)
			w("  Image:   %s\n", c.Image)
			if len(c.Ports) > 0 {
				var ports []string
				for _, p := range c.Ports {
					proto := string(p.Protocol)
					if proto == "" {
						proto = "TCP"
					}
					ports = append(ports, fmt.Sprintf("%d/%s", p.ContainerPort, proto))
				}
				w("  Ports:   %s\n", strings.Join(ports, ", "))
			}
		}
	}
	return b.String(), nil
}

func describeService(ctx context.Context, cs *kubernetes.Clientset, namespace, name string) (string, error) {
	if namespace == "" {
		namespace = "default"
	}
	svc, err := cs.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get service: %w", err)
	}
	var b bytes.Buffer
	w := func(format string, args ...interface{}) { fmt.Fprintf(&b, format, args...) }

	w("Name:        %s\n", svc.Name)
	w("Namespace:   %s\n", svc.Namespace)
	w("Labels:      %s\n", formatLabels(svc.Labels))
	w("Type:        %s\n", svc.Spec.Type)
	w("Cluster IP:  %s\n", svc.Spec.ClusterIP)
	if svc.Spec.LoadBalancerIP != "" {
		w("LoadBalancer IP: %s\n", svc.Spec.LoadBalancerIP)
	}
	if len(svc.Spec.Ports) > 0 {
		w("Ports:\n")
		for _, p := range svc.Spec.Ports {
			proto := string(p.Protocol)
			if proto == "" {
				proto = "TCP"
			}
			portStr := fmt.Sprintf("%d/%s", p.Port, proto)
			if p.NodePort > 0 {
				portStr = fmt.Sprintf("%d:%d/%s", p.Port, p.NodePort, proto)
			}
			w("  %-10s %s\n", p.Name, portStr)
		}
	}
	w("Selector:    %s\n", formatLabels(svc.Spec.Selector))
	return b.String(), nil
}

func getEvents(cs *kubernetes.Clientset, namespace, name, kind string) (string, error) {
	events, err := cs.CoreV1().Events(namespace).List(context.Background(), metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s,involvedObject.kind=%s", name, kind),
	})
	if err != nil || len(events.Items) == 0 {
		return "", err
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "  %-24s %-10s %-10s %s\n", "Type", "Reason", "Age", "From")
	for _, e := range events.Items {
		fmt.Fprintf(&b, "  %-24s %-10s %-10s %s\n", e.Type, e.Reason, e.LastTimestamp, e.Source.Component)
	}
	return b.String(), nil
}

func findContainerStatus(statuses []corev1.ContainerStatus, name string) *corev1.ContainerStatus {
	for i := range statuses {
		if statuses[i].Name == name {
			return &statuses[i]
		}
	}
	return nil
}

func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return "<none>"
	}
	var parts []string
	for k, v := range labels {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, ", ")
}

func formatResourceList(b *bytes.Buffer, prefix string, list corev1.ResourceList) {
	if len(list) == 0 {
		fmt.Fprintf(b, "  %s -\n", prefix)
		return
	}
	var parts []string
	for k, v := range list {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v.String()))
	}
	fmt.Fprintf(b, "  %s %s\n", prefix, strings.Join(parts, ", "))
}
