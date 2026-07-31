package k8s

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func DescribePod(ctx context.Context, cs *kubernetes.Clientset, namespace, name string) (string, error) {
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
	w("Labels:       %s\n", FormatLabels(pod.Labels))
	w("Status:       %s\n", pod.Status.Phase)
	w("IP:           %s\n", pod.Status.PodIP)
	if len(pod.Status.ContainerStatuses) > 0 {
		w("Containers:\n")
		for _, c := range pod.Spec.Containers {
			cs := FindContainerStatus(pod.Status.ContainerStatuses, c.Name)
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
				FormatResourceList(&b, "    Requests:", c.Resources.Requests)
				FormatResourceList(&b, "    Limits:  ", c.Resources.Limits)
			}
		}
	}
	if len(pod.Status.Conditions) > 0 {
		w("Conditions:\n")
		for _, c := range pod.Status.Conditions {
			w("  %-20s %s   %s\n", c.Type, c.Status, c.Reason)
		}
	}
	events, _ := GetEvents(cs, pod.Namespace, pod.Name, "Pod")
	if events != "" {
		w("\nEvents:\n%s", events)
	}
	return b.String(), nil
}

func DescribeNode(ctx context.Context, cs *kubernetes.Clientset, name string) (string, error) {
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
				w("%s", "Ready")
			} else {
				w("%s", string(c.Status))
			}
			break
		}
	}
	w("\n")
	w("Roles:    %s\n", ExtractRoles(node.Labels))
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

func DescribeDeployment(ctx context.Context, cs *kubernetes.Clientset, namespace, name string) (string, error) {
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
	w("Labels:       %s\n", FormatLabels(dep.Labels))
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

func DescribeService(ctx context.Context, cs *kubernetes.Clientset, namespace, name string) (string, error) {
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
	w("Labels:      %s\n", FormatLabels(svc.Labels))
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
	w("Selector:    %s\n", FormatLabels(svc.Spec.Selector))
	return b.String(), nil
}

func DescribeConfigMap(ctx context.Context, cs *kubernetes.Clientset, namespace, name string) (string, error) {
	if namespace == "" {
		namespace = "default"
	}
	cm, err := cs.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get configmap: %w", err)
	}
	var b bytes.Buffer
	w := func(format string, args ...interface{}) { fmt.Fprintf(&b, format, args...) }
	w("Name:      %s\n", cm.Name)
	w("Namespace: %s\n", cm.Namespace)
	w("Labels:    %s\n", FormatLabels(cm.Labels))
	w("Data:\n")
	for k, v := range cm.Data {
		val := v
		if len(val) > 200 {
			val = val[:200] + "..."
		}
		w("  %s: %s\n", k, strings.ReplaceAll(val, "\n", "\\n"))
	}
	return b.String(), nil
}

func DescribeNamespace(ctx context.Context, cs *kubernetes.Clientset, name string) (string, error) {
	ns, err := cs.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get namespace: %w", err)
	}
	var b bytes.Buffer
	w := func(format string, args ...interface{}) { fmt.Fprintf(&b, format, args...) }
	w("Name:   %s\n", ns.Name)
	w("Status: %s\n", ns.Status.Phase)
	w("Labels: %s\n", FormatLabels(ns.Labels))
	return b.String(), nil
}

func DescribeStatefulSet(ctx context.Context, cs *kubernetes.Clientset, namespace, name string) (string, error) {
	if namespace == "" {
		namespace = "default"
	}
	ss, err := cs.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get statefulset: %w", err)
	}
	var b bytes.Buffer
	w := func(format string, args ...interface{}) { fmt.Fprintf(&b, format, args...) }
	w("Name:      %s\n", ss.Name)
	w("Namespace: %s\n", ss.Namespace)
	w("Labels:    %s\n", FormatLabels(ss.Labels))
	w("Replicas:  %d ready | %d current | %d desired\n", ss.Status.ReadyReplicas, ss.Status.CurrentReplicas, ss.Status.Replicas)
	if ss.Spec.ServiceName != "" {
		w("Service:   %s\n", ss.Spec.ServiceName)
	}
	if len(ss.Spec.Template.Spec.Containers) > 0 {
		w("Containers:\n")
		for _, c := range ss.Spec.Template.Spec.Containers {
			w("  Name:  %s\n", c.Name)
			w("  Image: %s\n", c.Image)
		}
	}
	return b.String(), nil
}

func DescribeDaemonSet(ctx context.Context, cs *kubernetes.Clientset, namespace, name string) (string, error) {
	if namespace == "" {
		namespace = "default"
	}
	ds, err := cs.AppsV1().DaemonSets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get daemonset: %w", err)
	}
	var b bytes.Buffer
	w := func(format string, args ...interface{}) { fmt.Fprintf(&b, format, args...) }
	w("Name:      %s\n", ds.Name)
	w("Namespace: %s\n", ds.Namespace)
	w("Labels:    %s\n", FormatLabels(ds.Labels))
	w("Selector:  %s\n", FormatLabels(ds.Spec.Selector.MatchLabels))
	w("Desired:   %d nodes\n", ds.Status.DesiredNumberScheduled)
	w("Ready:     %d nodes\n", ds.Status.NumberReady)
	if len(ds.Spec.Template.Spec.Containers) > 0 {
		w("Containers:\n")
		for _, c := range ds.Spec.Template.Spec.Containers {
			w("  Name:  %s\n", c.Name)
			w("  Image: %s\n", c.Image)
		}
	}
	return b.String(), nil
}

func DescribeIngress(ctx context.Context, cs *kubernetes.Clientset, namespace, name string) (string, error) {
	if namespace == "" {
		namespace = "default"
	}
	ing, err := cs.NetworkingV1().Ingresses(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get ingress: %w", err)
	}
	var b bytes.Buffer
	w := func(format string, args ...interface{}) { fmt.Fprintf(&b, format, args...) }
	w("Name:      %s\n", ing.Name)
	w("Namespace: %s\n", ing.Namespace)
	w("Labels:    %s\n", FormatLabels(ing.Labels))
	if ing.Spec.IngressClassName != nil {
		w("Class:     %s\n", *ing.Spec.IngressClassName)
	}
	if len(ing.Spec.Rules) > 0 {
		w("Rules:\n")
		for _, r := range ing.Spec.Rules {
			host := r.Host
			if host == "" {
				host = "*"
			}
			for _, path := range r.HTTP.Paths {
				serviceName := path.Backend.Service.Name
				servicePort := path.Backend.Service.Port.Number
				if servicePort == 0 && path.Backend.Service.Port.Name != "" {
					w("  Host: %s Path: %s → %s:%s\n", host, path.Path, serviceName, path.Backend.Service.Port.Name)
				} else {
					w("  Host: %s Path: %s → %s:%d\n", host, path.Path, serviceName, servicePort)
				}
			}
		}
	}
	if len(ing.Spec.TLS) > 0 {
		w("TLS:\n")
		for _, t := range ing.Spec.TLS {
			w("  Hosts: %s\n", strings.Join(t.Hosts, ", "))
			w("  Secret: %s\n", t.SecretName)
		}
	}
	return b.String(), nil
}

func DescribeSecret(ctx context.Context, cs *kubernetes.Clientset, namespace, name string) (string, error) {
	if namespace == "" {
		namespace = "default"
	}
	secret, err := cs.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get secret: %w", err)
	}
	var b bytes.Buffer
	w := func(format string, args ...interface{}) { fmt.Fprintf(&b, format, args...) }
	w("Name:      %s\n", secret.Name)
	w("Namespace: %s\n", secret.Namespace)
	w("Type:      %s\n", secret.Type)
	w("Labels:    %s\n", FormatLabels(secret.Labels))
	w("Data:\n")
	for k, v := range secret.Data {
		size := len(v)
		sizeStr := fmt.Sprintf("%d bytes", size)
		if size >= 1024 {
			sizeStr = fmt.Sprintf("%.1f KiB", float64(size)/1024)
		}
		w("  %s: %s\n", k, sizeStr)
	}
	return b.String(), nil
}

func FormatResourceList(b *bytes.Buffer, prefix string, list corev1.ResourceList) {
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
