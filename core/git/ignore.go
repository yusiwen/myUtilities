package git

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var langDetectors = []struct {
	markers []string
	lang    string
}{
	{[]string{"go.mod"}, "Go"},
	{[]string{"package.json"}, "Node"},
	{[]string{"Cargo.toml"}, "Rust"},
	{[]string{"pyproject.toml", "setup.py", "requirements.txt"}, "Python"},
	{[]string{"Gemfile"}, "Ruby"},
	{[]string{"pom.xml", "build.gradle"}, "Java"},
	{[]string{"composer.json"}, "PHP"},
	{[]string{"CMakeLists.txt"}, "CMake"},
	{[]string{"Dockerfile"}, "Docker"},
	{[]string{"stack.yaml"}, "Haskell"},
}

func DetectLang(dir string) string {
	for _, d := range langDetectors {
		for _, marker := range d.markers {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return d.lang
			}
		}
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.csproj"))
	if len(matches) > 0 {
		return "Csharp"
	}
	matches, _ = filepath.Glob(filepath.Join(dir, "*.cabal"))
	if len(matches) > 0 {
		return "Haskell"
	}
	return ""
}

type ghContentItem struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func ListTemplates() ([]string, error) {
	resp, err := http.Get("https://api.github.com/repos/github/gitignore/contents")
	if err != nil {
		return nil, fmt.Errorf("fetch template list: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned %s", resp.Status)
	}
	var items []ghContentItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	var templates []string
	for _, item := range items {
		if item.Type == "file" && strings.HasSuffix(item.Name, ".gitignore") {
			templates = append(templates, strings.TrimSuffix(item.Name, ".gitignore"))
		}
	}
	sort.Strings(templates)
	return templates, nil
}

func DownloadTemplate(lang string) ([]byte, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/github/gitignore/main/%s.gitignore", lang)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download template: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("template %q not found", lang)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download failed: %s", resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	return data, nil
}

func Merge(existing, template []byte) []byte {
	templateStr := string(template)
	existingStr := string(existing)

	templateLines := strings.Split(templateStr, "\n")
	existingLines := strings.Split(existingStr, "\n")

	templatePatterns := make(map[string]bool)
	for _, line := range templateLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		templatePatterns[trimmed] = true
	}

	var additions []string
	for _, line := range existingLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			additions = append(additions, line)
			continue
		}
		if !templatePatterns[trimmed] {
			additions = append(additions, line)
		}
	}

	result := templateStr
	if !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	if len(additions) > 0 {
		result += "\n### Local additions ###\n"
		result += strings.Join(additions, "\n")
		if !strings.HasSuffix(result, "\n") {
			result += "\n"
		}
	}
	return []byte(result)
}
