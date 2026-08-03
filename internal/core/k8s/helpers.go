package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func HumanAge(t time.Time) string {
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

func CountReady(statuses []corev1.ContainerStatus) int {
	ready := 0
	for _, s := range statuses {
		if s.Ready {
			ready++
		}
	}
	return ready
}

func CountRestarts(statuses []corev1.ContainerStatus) int {
	total := int32(0)
	for _, s := range statuses {
		total += s.RestartCount
	}
	return int(total)
}

func ExtractRoles(labels map[string]string) string {
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

func FormatServicePorts(ports []corev1.ServicePort) string {
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

func FindContainerStatus(statuses []corev1.ContainerStatus, name string) *corev1.ContainerStatus {
	for i := range statuses {
		if statuses[i].Name == name {
			return &statuses[i]
		}
	}
	return nil
}

func FormatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return "<none>"
	}
	var parts []string
	for k, v := range labels {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, ", ")
}

func GetEvents(cs *kubernetes.Clientset, namespace, name, kind string) (string, error) {
	events, err := cs.CoreV1().Events(namespace).List(context.Background(), metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.name=%s,involvedObject.kind=%s", name, kind),
	})
	if err != nil || len(events.Items) == 0 {
		return "", err
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "  %-24s %-10s %-10s %s\n", "Type", "Reason", "Age", "From")
	for _, e := range events.Items {
		fmt.Fprintf(&sb, "  %-24s %-10s %-10s %s\n", e.Type, e.Reason, e.LastTimestamp, e.Source.Component)
	}
	return sb.String(), nil
}

/* ─── Metrics ─── */

func FetchPodMetrics(ctx context.Context, cs *kubernetes.Clientset, namespace string) map[string]string {
	result := make(map[string]string)
	var metrics struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Containers []struct {
				Usage struct {
					CPU    string `json:"cpu"`
					Memory string `json:"memory"`
				} `json:"usage"`
			} `json:"containers"`
		} `json:"items"`
	}
	data, err := cs.DiscoveryClient.RESTClient().Get().
		RequestURI("/apis/metrics.k8s.io/v1beta1/pods").
		Do(ctx).
		Raw()
	if err != nil {
		return result
	}
	json.Unmarshal(data, &metrics)
	for _, item := range metrics.Items {
		if namespace != "" && item.Metadata.Namespace != namespace {
			continue
		}
		totalCPU := ""
		totalMem := ""
		for _, c := range item.Containers {
			totalCPU = c.Usage.CPU
			totalMem = c.Usage.Memory
		}
		if totalCPU != "" && totalMem != "" {
			result[item.Metadata.Namespace+"/"+item.Metadata.Name] = totalCPU + "/" + totalMem
		}
	}
	return result
}

func FetchNodeMetrics(ctx context.Context, cs *kubernetes.Clientset) map[string][2]string {
	result := make(map[string][2]string)
	var metrics struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
			Usage struct {
				CPU    string `json:"cpu"`
				Memory string `json:"memory"`
			} `json:"usage"`
		} `json:"items"`
	}
	data, err := cs.DiscoveryClient.RESTClient().Get().
		RequestURI("/apis/metrics.k8s.io/v1beta1/nodes").
		Do(ctx).
		Raw()
	if err != nil {
		return result
	}
	json.Unmarshal(data, &metrics)
	for _, item := range metrics.Items {
		result[item.Metadata.Name] = [2]string{item.Usage.CPU, item.Usage.Memory}
	}
	return result
}

func FetchNodeCapacity(ctx context.Context, cs *kubernetes.Clientset) map[string][2]string {
	result := make(map[string][2]string)
	nodes, err := cs.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return result
	}
	for _, node := range nodes.Items {
		cpu := node.Status.Capacity.Cpu().String()
		mem := node.Status.Capacity.Memory().String()
		result[node.Name] = [2]string{cpu, mem}
	}
	return result
}

func CalcPercent(value, total string) string {
	v := ParseQuantity(value)
	t := ParseQuantity(total)
	if t == 0 {
		return "-"
	}
	return fmt.Sprintf("%d%%", v*100/t)
}

func ParseQuantity(s string) int64 {
	if s == "" || s == "-" {
		return 0
	}
	var mult int64 = 1
	if strings.HasSuffix(s, "m") {
		mult = 1
		s = s[:len(s)-1]
	} else if strings.HasSuffix(s, "Gi") {
		mult = 1024 * 1024 * 1024
		s = s[:len(s)-2]
	} else if strings.HasSuffix(s, "Mi") {
		mult = 1024 * 1024
		s = s[:len(s)-2]
	} else if strings.HasSuffix(s, "Ki") {
		mult = 1024
		s = s[:len(s)-2]
	} else if strings.HasSuffix(s, "Ti") {
		mult = 1024 * 1024 * 1024 * 1024
		s = s[:len(s)-2]
	}
	var val int64
	fmt.Sscanf(s, "%d", &val)
	return val * mult
}
