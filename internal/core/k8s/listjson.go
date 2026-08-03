package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func ListPodsJSON(ctx context.Context, cs *kubernetes.Clientset, namespace string, showMetrics bool, w http.ResponseWriter) {
	pods, err := cs.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		http.Error(w, fmt.Sprintf("list pods: %v", err), http.StatusInternalServerError)
		return
	}
	columns := []string{"NAMESPACE", "NAME", "READY", "STATUS", "RESTARTS", "AGE"}
	if showMetrics {
		columns = []string{"NAMESPACE", "NAME", "READY", "STATUS", "RESTARTS", "CPU", "MEMORY", "AGE"}
	}
	podMetrics := FetchPodMetrics(ctx, cs, namespace)
	var rows [][]string
	for _, pod := range pods.Items {
		ready := CountReady(pod.Status.ContainerStatuses)
		restarts := CountRestarts(pod.Status.ContainerStatuses)
		cpu, mem := "-", "-"
		if showMetrics {
			key := pod.Namespace + "/" + pod.Name
			if v, ok := podMetrics[key]; ok {
				parts := strings.SplitN(v, "/", 2)
				if len(parts) == 2 {
					cpu, mem = parts[0], parts[1]
				}
			}
			rows = append(rows, []string{
				pod.Namespace, pod.Name,
				fmt.Sprintf("%d/%d", ready, len(pod.Status.ContainerStatuses)),
				string(pod.Status.Phase),
				fmt.Sprintf("%d", restarts),
				cpu, mem,
				HumanAge(pod.CreationTimestamp.Time),
			})
		} else {
			rows = append(rows, []string{
				pod.Namespace, pod.Name,
				fmt.Sprintf("%d/%d", ready, len(pod.Status.ContainerStatuses)),
				string(pod.Status.Phase),
				fmt.Sprintf("%d", restarts),
				HumanAge(pod.CreationTimestamp.Time),
			})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"columns": columns, "rows": rows})
}

func ListNodesJSON(ctx context.Context, cs *kubernetes.Clientset, showMetrics bool, w http.ResponseWriter) {
	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		http.Error(w, fmt.Sprintf("list nodes: %v", err), http.StatusInternalServerError)
		return
	}
	columns := []string{"NAME", "STATUS", "ROLES", "VERSION", "AGE"}
	if showMetrics {
		columns = []string{"NAME", "STATUS", "ROLES", "CPU", "CPU%", "MEMORY", "MEM%", "AGE"}
	}
	nodeMetrics := FetchNodeMetrics(ctx, cs)
	nodeCap := FetchNodeCapacity(ctx, cs)
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
		if showMetrics {
			cpu, mem, cpuPct, memPct := "-", "-", "-", "-"
			if m, ok := nodeMetrics[node.Name]; ok {
				cpu, mem = m[0], m[1]
				if cap, ok := nodeCap[node.Name]; ok {
					cpuPct = CalcPercent(cpu, cap[0])
					memPct = CalcPercent(mem, cap[1])
				}
			}
			rows = append(rows, []string{
				node.Name, status, ExtractRoles(node.Labels),
				cpu, cpuPct, mem, memPct,
				HumanAge(node.CreationTimestamp.Time),
			})
		} else {
			rows = append(rows, []string{
				node.Name, status, ExtractRoles(node.Labels),
				node.Status.NodeInfo.KubeletVersion,
				HumanAge(node.CreationTimestamp.Time),
			})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"columns": columns, "rows": rows})
}

func ListDeploymentsJSON(ctx context.Context, cs *kubernetes.Clientset, namespace string, w http.ResponseWriter) {
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
			HumanAge(dep.CreationTimestamp.Time),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"columns": columns, "rows": rows})
}

func ListServicesJSON(ctx context.Context, cs *kubernetes.Clientset, namespace string, w http.ResponseWriter) {
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
			svc.Spec.ClusterIP, JoinStrings(ports, ","),
			HumanAge(svc.CreationTimestamp.Time),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"columns": columns, "rows": rows})
}

func ListConfigMapsJSON(ctx context.Context, cs *kubernetes.Clientset, namespace string, w http.ResponseWriter) {
	cms, err := cs.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		http.Error(w, fmt.Sprintf("list configmaps: %v", err), http.StatusInternalServerError)
		return
	}
	columns := []string{"NAMESPACE", "NAME", "DATA", "AGE"}
	var rows [][]string
	for _, cm := range cms.Items {
		dataCount := len(cm.Data) + len(cm.BinaryData)
		rows = append(rows, []string{cm.Namespace, cm.Name, fmt.Sprintf("%d", dataCount), HumanAge(cm.CreationTimestamp.Time)})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"columns": columns, "rows": rows})
}

func ListNamespacesJSON(ctx context.Context, cs *kubernetes.Clientset, w http.ResponseWriter) {
	nsList, err := cs.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		http.Error(w, fmt.Sprintf("list namespaces: %v", err), http.StatusInternalServerError)
		return
	}
	columns := []string{"NAME", "STATUS", "AGE"}
	var rows [][]string
	for _, ns := range nsList.Items {
		rows = append(rows, []string{ns.Name, string(ns.Status.Phase), HumanAge(ns.CreationTimestamp.Time)})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"columns": columns, "rows": rows})
}

func ListStatefulSetsJSON(ctx context.Context, cs *kubernetes.Clientset, namespace string, w http.ResponseWriter) {
	ss, err := cs.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		http.Error(w, fmt.Sprintf("list statefulsets: %v", err), http.StatusInternalServerError)
		return
	}
	columns := []string{"NAMESPACE", "NAME", "READY", "AGE"}
	var rows [][]string
	for _, s := range ss.Items {
		rows = append(rows, []string{s.Namespace, s.Name, fmt.Sprintf("%d/%d", s.Status.ReadyReplicas, s.Status.Replicas), HumanAge(s.CreationTimestamp.Time)})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"columns": columns, "rows": rows})
}

func ListDaemonSetsJSON(ctx context.Context, cs *kubernetes.Clientset, namespace string, w http.ResponseWriter) {
	ds, err := cs.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		http.Error(w, fmt.Sprintf("list daemonsets: %v", err), http.StatusInternalServerError)
		return
	}
	columns := []string{"NAMESPACE", "NAME", "DESIRED", "CURRENT", "READY", "AGE"}
	var rows [][]string
	for _, d := range ds.Items {
		rows = append(rows, []string{
			d.Namespace, d.Name,
			fmt.Sprintf("%d", d.Status.DesiredNumberScheduled),
			fmt.Sprintf("%d", d.Status.CurrentNumberScheduled),
			fmt.Sprintf("%d", d.Status.NumberReady),
			HumanAge(d.CreationTimestamp.Time),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"columns": columns, "rows": rows})
}

func ListIngressesJSON(ctx context.Context, cs *kubernetes.Clientset, namespace string, w http.ResponseWriter) {
	ing, err := cs.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		http.Error(w, fmt.Sprintf("list ingresses: %v", err), http.StatusInternalServerError)
		return
	}
	columns := []string{"NAMESPACE", "NAME", "HOSTS", "ADDRESS", "AGE"}
	var rows [][]string
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
		rows = append(rows, []string{i.Namespace, i.Name, hosts, address, HumanAge(i.CreationTimestamp.Time)})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"columns": columns, "rows": rows})
}

func ListSecretsJSON(ctx context.Context, cs *kubernetes.Clientset, namespace string, w http.ResponseWriter) {
	secrets, err := cs.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		http.Error(w, fmt.Sprintf("list secrets: %v", err), http.StatusInternalServerError)
		return
	}
	columns := []string{"NAMESPACE", "NAME", "TYPE", "DATA", "AGE"}
	var rows [][]string
	for _, s := range secrets.Items {
		rows = append(rows, []string{s.Namespace, s.Name, string(s.Type), fmt.Sprintf("%d", len(s.Data)), HumanAge(s.CreationTimestamp.Time)})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"columns": columns, "rows": rows})
}
