package misc

import (
	"bytes"
	"encoding/json"
)

// FormatJSON pretty-prints the given JSON with two-space indentation.
func FormatJSON(input string) (string, error) {
	var v interface{}
	if err := json.Unmarshal([]byte(input), &v); err != nil {
		return "", err
	}
	out, _ := json.MarshalIndent(v, "", "  ")
	return string(out), nil
}

// ValidateJSON returns a human-readable verdict for whether input is valid JSON.
func ValidateJSON(input string) (string, error) {
	if json.Valid([]byte(input)) {
		return "✅ Valid JSON", nil
	}
	return "❌ Invalid JSON", nil
}

// MinifyJSON compacts the given JSON to a single line.
func MinifyJSON(input string) (string, error) {
	var v interface{}
	if err := json.Unmarshal([]byte(input), &v); err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(input)); err != nil {
		return "", err
	}
	return buf.String(), nil
}
