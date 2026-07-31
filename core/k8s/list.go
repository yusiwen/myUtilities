package k8s

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func ListPods(w io.Writer, ctx context.Context, cs *kubernetes.Clientset, namespace string, showMetrics bool) error {
	pods, err := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list pods: %w", err)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	if showMetrics {
		fmt.Fprintln(tw, "NAMESPACE\tNAME\tREADY\tSTATUS\tRESTARTS\tCPU\tMEMORY\tAGE")
	} else {
		fmt.Fprintln(tw, "NAMESPACE\tNAME\tREADY\tSTATUS\tRESTARTS\tAGE")
	}
	podMetrics := FetchPodMetrics(ctx, cs, namespace)
	for _, pod := range pods.Items {
		ready := CountReady(pod.Status.ContainerStatuses)
		restarts := CountRestarts(pod.Status.ContainerStatuses)
		cpu, mem := "-", "-"
		if v, ok := podMetrics[pod.Namespace+"/"+pod.Name]; ok {
			parts := strings.SplitN(v, "/", 2)
			if len(parts) == 2 {
				cpu, mem = parts[0], parts[1]
			}
		}
		if showMetrics {
			fmt.Fprintf(tw, "%s\t%s\t%d/%d\t%s\t%d\t%s\t%s\t%s\n",
				pod.Namespace, pod.Name,
				ready, len(pod.Status.ContainerStatuses),
				pod.Status.Phase, restarts,
				cpu, mem,
				HumanAge(pod.CreationTimestamp.Time),
			)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t%d/%d\t%s\t%d\t%s\n",
				pod.Namespace, pod.Name,
				ready, len(pod.Status.ContainerStatuses),
				pod.Status.Phase, restarts,
				HumanAge(pod.CreationTimestamp.Time),
			)
		}
	}
	return tw.Flush()
}

func ListNodes(w io.Writer, ctx context.Context, cs *kubernetes.Clientset, showMetrics bool) error {
	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list nodes: %w", err)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	if showMetrics {
		fmt.Fprintln(tw, "NAME\tSTATUS\tROLES\tCPU\tCPU%\tMEMORY\tMEM%\tAGE")
	} else {
		fmt.Fprintln(tw, "NAME\tSTATUS\tROLES\tVERSION\tAGE")
	}
	nodeMetrics := FetchNodeMetrics(ctx, cs)
	nodeCap := FetchNodeCapacity(ctx, cs)
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
		if showMetrics {
			cpu := "-"
			mem := "-"
			cpuPct := "-"
			memPct := "-"
			if m, ok := nodeMetrics[node.Name]; ok {
				cpu = m[0]
				mem = m[1]
				if cap, ok := nodeCap[node.Name]; ok {
					cpuPct = CalcPercent(cpu, cap[0])
					memPct = CalcPercent(mem, cap[1])
				}
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				node.Name, status, ExtractRoles(node.Labels),
				cpu, cpuPct, mem, memPct,
				HumanAge(node.CreationTimestamp.Time),
			)
		} else {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				node.Name, status, ExtractRoles(node.Labels),
				node.Status.NodeInfo.KubeletVersion,
				HumanAge(node.CreationTimestamp.Time),
			)
		}
	}
	return tw.Flush()
}

func ListDeployments(w io.Writer, ctx context.Context, cs *kubernetes.Clientset, namespace string) error {
	deployments, err := cs.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list deployments: %w", err)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "NAMESPACE\tNAME\tREADY\tUP-TO-DATE\tAVAILABLE\tAGE")
	for _, dep := range deployments.Items {
		fmt.Fprintf(tw, "%s\t%s\t%d/%d\t%d\t%d\t%s\n",
			dep.Namespace, dep.Name,
			dep.Status.ReadyReplicas, dep.Status.Replicas,
			dep.Status.UpdatedReplicas, dep.Status.AvailableReplicas,
			HumanAge(dep.CreationTimestamp.Time),
		)
	}
	return tw.Flush()
}

func ListServices(w io.Writer, ctx context.Context, cs *kubernetes.Clientset, namespace string) error {
	svcs, err := cs.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list services: %w", err)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "NAMESPACE\tNAME\tTYPE\tCLUSTER-IP\tPORT(S)\tAGE")
	for _, svc := range svcs.Items {
		ports := FormatServicePorts(svc.Spec.Ports)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			svc.Namespace, svc.Name, svc.Spec.Type, svc.Spec.ClusterIP,
			ports, HumanAge(svc.CreationTimestamp.Time),
		)
	}
	return tw.Flush()
}

func ListConfigMaps(w io.Writer, ctx context.Context, cs *kubernetes.Clientset, namespace string) error {
	cms, err := cs.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list configmaps: %w", err)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "NAMESPACE\tNAME\tDATA\tAGE")
	for _, cm := range cms.Items {
		dataCount := len(cm.Data) + len(cm.BinaryData)
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", cm.Namespace, cm.Name, dataCount, HumanAge(cm.CreationTimestamp.Time))
	}
	return tw.Flush()
}

func ListNamespaces(w io.Writer, ctx context.Context, cs *kubernetes.Clientset) error {
	nsList, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list namespaces: %w", err)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "NAME\tSTATUS\tAGE")
	for _, ns := range nsList.Items {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", ns.Name, ns.Status.Phase, HumanAge(ns.CreationTimestamp.Time))
	}
	return tw.Flush()
}

func ListStatefulSets(w io.Writer, ctx context.Context, cs *kubernetes.Clientset, namespace string) error {
	ss, err := cs.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list statefulsets: %w", err)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "NAMESPACE\tNAME\tREADY\tAGE")
	for _, s := range ss.Items {
		fmt.Fprintf(tw, "%s\t%s\t%d/%d\t%s\n", s.Namespace, s.Name, s.Status.ReadyReplicas, s.Status.Replicas, HumanAge(s.CreationTimestamp.Time))
	}
	return tw.Flush()
}

func ListDaemonSets(w io.Writer, ctx context.Context, cs *kubernetes.Clientset, namespace string) error {
	ds, err := cs.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list daemonsets: %w", err)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "NAMESPACE\tNAME\tDESIRED\tCURRENT\tREADY\tAGE")
	for _, d := range ds.Items {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%s\n", d.Namespace, d.Name, d.Status.DesiredNumberScheduled, d.Status.CurrentNumberScheduled, d.Status.NumberReady, HumanAge(d.CreationTimestamp.Time))
	}
	return tw.Flush()
}

func ListIngresses(w io.Writer, ctx context.Context, cs *kubernetes.Clientset, namespace string) error {
	ing, err := cs.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list ingresses: %w", err)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "NAMESPACE\tNAME\tHOSTS\tADDRESS\tAGE")
	for _, i := range ing.Items {
		hosts := "<none>"
		address := "<none>"
		if len(i.Spec.Rules) > 0 {
			var h []string
			for _, r := range i.Spec.Rules {
				if r.Host != "" {
					h = append(h, r.Host)
				}
			}
			if len(h) > 0 {
				hosts = JoinStrings(h, ",")
			}
		}
		if len(i.Status.LoadBalancer.Ingress) > 0 {
			address = i.Status.LoadBalancer.Ingress[0].IP
			if address == "" {
				address = i.Status.LoadBalancer.Ingress[0].Hostname
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", i.Namespace, i.Name, hosts, address, HumanAge(i.CreationTimestamp.Time))
	}
	return tw.Flush()
}

func ListSecrets(w io.Writer, ctx context.Context, cs *kubernetes.Clientset, namespace string) error {
	secrets, err := cs.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list secrets: %w", err)
	}
	tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
	fmt.Fprintln(tw, "NAMESPACE\tNAME\tTYPE\tDATA\tAGE")
	for _, s := range secrets.Items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\n", s.Namespace, s.Name, s.Type, len(s.Data), HumanAge(s.CreationTimestamp.Time))
	}
	return tw.Flush()
}

func JoinStrings(items []string, sep string) string {
	result := ""
	for i, s := range items {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
