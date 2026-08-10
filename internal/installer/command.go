package installer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"text/template"

	coreinst "github.com/yusiwen/myUtilities/internal/core/installer"
	"github.com/yusiwen/myUtilities/internal/installer/templates"
)

// safeNameRe constrains user-controlled values that are interpolated into the
// generated shell script. Anything else could break out of the script's quoted
// strings and inject arbitrary shell commands.
var safeNameRe = regexp.MustCompile(`^[A-Za-z0-9._\-/@+]+$`)

func sanitize(name, what string) error {
	if !safeNameRe.MatchString(name) {
		return fmt.Errorf("invalid %s %q: allowed characters are letters, digits and ._-/@+", what, name)
	}
	return nil
}

func (o Options) Run() error {
	script := ""
	switch o.Output {
	case "json":
		script = ""
	case "shell":
		script = string(templates.Shell)
	default:
		return fmt.Errorf("unknown type: %s", o.Output)
	}

	q := coreinst.Query{
		User:      "",
		Program:   "",
		Release:   "",
		Insecure:  o.Insecure,
		AsProgram: o.AsProgram,
		Select:    o.Select,
		OS:        o.Os,
		Arch:      o.Arch,
	}
	if o.Move {
		q.MoveToPath = true
	}

	var rest string
	q.User, rest = coreinst.SplitHalf(o.Repo, "/")
	q.Program, q.Release = coreinst.SplitHalf(rest, "@")
	if q.Program == "" {
		q.Program = q.User
		q.Search = true
	}
	if q.Release == "" {
		q.Release = "latest"
	}

	// Token precedence: explicit --token (or GITHUB_TOKEN env) over the
	// installer-config.json value.
	token := o.Token
	if token == "" {
		cfg, err := coreinst.LoadConfig(o.Config)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
		token = cfg.Token
	}

	if err := sanitize(q.User, "user"); err != nil {
		return err
	}
	if err := sanitize(q.Program, "program"); err != nil {
		return err
	}
	if err := sanitize(q.Release, "release"); err != nil {
		return err
	}
	if q.AsProgram != "" {
		if err := sanitize(q.AsProgram, "as-program"); err != nil {
			return err
		}
	}

	client := &coreinst.Client{Token: token}
	result, err := client.QueryAssets(q)
	if err != nil {
		return fmt.Errorf("query failed: %s", err)
	}

	if script == "" {
		b, _ := json.MarshalIndent(result, "", "  ")
		fmt.Printf("%s\n", b)
		return nil
	}

	t, err := template.New("installer").Parse(script)
	if err != nil {
		return fmt.Errorf("template.New() error: %s", err)
	}
	buff := bytes.Buffer{}
	if err := t.Execute(&buff, result); err != nil {
		return fmt.Errorf("template.execute() error: %s", err)
	}
	fmt.Printf("%s\n", buff.Bytes())
	return nil
}
