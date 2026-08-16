# k8s — Kubernetes utilities

Generate and decode Kubernetes Opaque Secret YAML files. Values are automatically
base64-encoded for the `data` section. List Kubernetes resources from your cluster
using your kubeconfig (`~/.kube/config` by default).

```bash
# Generate a Secret YAML from CLI arguments
mu k8s secret my-app DB_HOST=localhost DB_PASSWORD=s3cret

# Read key=value pairs from a .env file
mu k8s secret my-app --from-env .env

# Pipe key=value pairs from stdin
cat .env | mu k8s secret my-app

# Output to file
mu k8s secret my-app KEY=val -o secret.yaml

# Decode an existing Secret YAML back to plaintext
mu k8s secret secret.yaml --decode

# List resources from the current kubeconfig context
mu k8s get pods
mu k8s get pods -n kube-system
mu k8s get nodes
mu k8s get deployments
mu k8s get services
mu k8s get configmaps
mu k8s get namespaces
mu k8s get statefulsets
mu k8s get daemonsets
mu k8s get ingresses
mu k8s get secrets
mu k8s get pods --context my-cluster
mu k8s get pods --kubeconfig /path/to/config

# Show CPU/memory metrics (requires metrics-server)
mu k8s get nodes --metrics
mu k8s get pods --metrics -n default

# Describe a resource in detail
mu k8s describe pod my-pod -n default
mu k8s describe node my-node
mu k8s describe deployment my-deploy -n default
mu k8s describe service my-svc -n default
mu k8s describe configmap my-cm -n default
mu k8s describe namespace kube-system
mu k8s describe secret my-secret -n default

# Serve web UI (standalone)
mu k8s serve --port 8089
```

```bash
# Serve web UI with pre-loaded kubeconfig
mu k8s serve --port 8089 --kubeconfig ~/.kube/config
```

The web UI provides:
- **Secret** tab — encode/decode Secret YAML in one place, with mode switch, .env file loading, copy/download
- **Resources** tab — connect to a Kubernetes cluster by uploading or pasting your kubeconfig,
  list pods, nodes, deployments, services, configmaps, namespaces, statefulsets, daemonsets, ingresses, and secrets with namespace filtering and context switching (with optional metrics for pods and nodes);
  click any resource name to view detailed describe information in a modal dialog
  (kubeconfig is persisted at `~/.config/mu/kubeconfigs.yaml`, supports multiple saved configs)

Supports `key=value` format with `#` comments and blank lines in env files.
