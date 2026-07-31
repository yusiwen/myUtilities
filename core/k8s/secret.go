package k8s

import (
	"encoding/base64"
	"fmt"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

const secretTmpl = `apiVersion: v1
kind: Secret
metadata:
  name: {{.Name}}
type: Opaque
data:
{{- range .Entries}}
  {{.Key}}: {{.Value}}
{{- end}}
`

type SecretEntry struct {
	Key   string
	Value string
}

type SecretTemplateData struct {
	Name    string
	Entries []SecretEntry
}

// EncodeSecretYAML renders a Kubernetes Secret YAML from key=value pairs.
func EncodeSecretYAML(name string, pairs []string) (string, error) {
	var entries []SecretEntry
	for _, p := range pairs {
		parts := strings.SplitN(p, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			return "", fmt.Errorf("invalid pair: %s (expected key=value)", p)
		}
		encoded := base64.StdEncoding.EncodeToString([]byte(parts[1]))
		entries = append(entries, SecretEntry{Key: parts[0], Value: encoded})
	}

	tmpl, err := template.New("secret").Parse(secretTmpl)
	if err != nil {
		return "", fmt.Errorf("template: %w", err)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, SecretTemplateData{Name: name, Entries: entries}); err != nil {
		return "", fmt.Errorf("render: %w", err)
	}
	return buf.String(), nil
}

// DecodeSecretYAML decodes a Kubernetes Secret YAML back to plaintext key=value pairs.
func DecodeSecretYAML(data []byte) (map[string]string, error) {
	var secret struct {
		Data map[string]string `yaml:"data"`
	}
	if err := yaml.Unmarshal(data, &secret); err != nil {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}
	if secret.Data == nil {
		return nil, fmt.Errorf("no data field found in YAML")
	}

	result := make(map[string]string)
	for k, v := range secret.Data {
		decoded, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			result[k] = v // not base64, return as-is
		} else {
			result[k] = string(decoded)
		}
	}
	return result, nil
}

// EncodeSecretYAMLMap renders a Secret YAML from a name and key→value map.
func EncodeSecretYAMLMap(name string, data map[string]string) (string, error) {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	var entries []SecretEntry
	for _, k := range keys {
		entries = append(entries, SecretEntry{
			Key:   k,
			Value: base64.StdEncoding.EncodeToString([]byte(data[k])),
		})
	}

	tmpl, err := template.New("secret").Parse(secretTmpl)
	if err != nil {
		return "", fmt.Errorf("template: %w", err)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, SecretTemplateData{Name: name, Entries: entries}); err != nil {
		return "", fmt.Errorf("render: %w", err)
	}
	return buf.String(), nil
}

func ParseEnvFile(content string) []string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}
