package git

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	coregit "github.com/yusiwen/myUtilities/core/git"
)

type IgnoreOptions struct {
	Lang   string `arg:"" optional:"" name:"lang" help:"Language/technology (e.g., Go, Python, Node). Auto-detected when omitted."`
	List   bool   `short:"l" help:"List available .gitignore templates."`
	Output string `short:"o" default:".gitignore" help:"Output file path."`
	Merge  bool   `short:"m" help:"Merge with existing .gitignore instead of overwriting."`
	Force  bool   `short:"f" help:"Overwrite existing .gitignore without prompting."`
}

func (o *IgnoreOptions) Run() error {
	if o.List {
		templates, err := coregit.ListTemplates()
		if err != nil {
			return err
		}
		fmt.Println("Available .gitignore templates:")
		for _, t := range templates {
			fmt.Println("  " + t)
		}
		return nil
	}

	lang := o.Lang
	if lang == "" {
		lang = coregit.DetectLang(".")
		if lang == "" {
			return fmt.Errorf("could not detect language — specify with --lang or use --list to see available templates")
		}
		fmt.Printf("Detected %s.\n", lang)
	}

	template, err := coregit.DownloadTemplate(lang)
	if err != nil {
		return err
	}

	if _, statErr := os.Stat(o.Output); statErr == nil {
		switch {
		case o.Force:
		case o.Merge:
			existing, err := os.ReadFile(o.Output)
			if err != nil {
				return fmt.Errorf("read %s: %w", o.Output, err)
			}
			template = coregit.Merge(existing, template)
		default:
			fmt.Printf("%s already exists. Use --force to overwrite or --merge to merge.\n", o.Output)
			fmt.Print("Overwrite without merging? [y/N]: ")
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(answer)
			if strings.ToLower(answer) != "y" && strings.ToLower(answer) != "yes" {
				fmt.Println("Cancelled.")
				return nil
			}
		}
	}

	if err := os.WriteFile(o.Output, template, 0644); err != nil {
		return fmt.Errorf("write %s: %w", o.Output, err)
	}

	fmt.Printf("Written %d bytes to %s\n", len(template), o.Output)
	return nil
}
