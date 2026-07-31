package installer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"

	coreinst "github.com/yusiwen/myUtilities/core/installer"
	"github.com/yusiwen/myUtilities/installer/templates"
)

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

	client := &coreinst.Client{Token: o.Token}
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
