package git

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/alecthomas/kong"
	"github.com/yusiwen/myUtilities/internal/core/config"
	coregit "github.com/yusiwen/myUtilities/internal/core/git"
	"github.com/yusiwen/myUtilities/internal/core/llm"
)

type gitSetter struct{}

func init() {
	config.Register(&gitSetter{})
}

func (s *gitSetter) Name() string {
	return "git"
}

func (s *gitSetter) Set(args []string) error {
	opts := &GitSetCmd{}
	parser, err := kong.New(opts,
		kong.Name("mu set git"),
		kong.Description("Update git module configuration (git-config.json)."))
	if err != nil {
		return err
	}
	ctx, err := parser.Parse(args)
	if err != nil {
		return err
	}
	return ctx.Run()
}

type GitSetCmd struct {
	Provider ProviderCmd     `cmd:"" name:"provider" help:"Manage LLM providers."`
	Commit   CommitModuleCmd `cmd:"" name:"commit" help:"Configure commit module settings."`
	Review   ReviewModuleCmd `cmd:"" name:"review" help:"Configure review module settings."`
}

/* ─── Provider Subcommands ─── */

type ProviderCmd struct {
	Add  ProviderAddCmd  `cmd:"" name:"add" help:"Add a new LLM provider."`
	Rm   ProviderRmCmd   `cmd:"" name:"rm" help:"Remove an LLM provider."`
	List ProviderListCmd `cmd:"" name:"list" help:"List all configured providers."`
}

type ProviderAddCmd struct {
	Name    string `help:"Provider name (e.g. default, advanced)." required:""`
	BaseURL string `help:"Base URL of the AI service." required:""`
	APIKey  string `help:"API key for the AI service." required:""`
	Model   string `help:"Model name (e.g. gpt-4o-mini, deepseek-chat)."`
}

func (o *ProviderAddCmd) Run() error {
	gc, err := coregit.LoadGitConfig()
	if err != nil {
		return err
	}

	for _, p := range gc.Providers {
		if p.Name == o.Name {
			return fmt.Errorf("provider %q already exists", o.Name)
		}
	}

	gc.Providers = append(gc.Providers, coregit.Provider{
		Name:    o.Name,
		BaseURL: o.BaseURL,
		APIKey:  o.APIKey,
		Model:   o.Model,
	})
	return coregit.SaveGitConfig(gc)
}

type ProviderRmCmd struct {
	Name string `help:"Provider name to remove." required:""`
}

func (o *ProviderRmCmd) Run() error {
	gc, err := coregit.LoadGitConfig()
	if err != nil {
		return err
	}

	idx := -1
	for i, p := range gc.Providers {
		if p.Name == o.Name {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("provider %q not found", o.Name)
	}

	for name, mc := range map[string]coregit.ModuleConfig{"commit": gc.Commit, "review": gc.Review} {
		if mc.Provider.Contains(o.Name) {
			return fmt.Errorf("cannot remove provider %q: module %q is using it", o.Name, name)
		}
	}

	gc.Providers = append(gc.Providers[:idx], gc.Providers[idx+1:]...)
	return coregit.SaveGitConfig(gc)
}

type ProviderListCmd struct{}

func (o *ProviderListCmd) Run() error {
	gc, err := coregit.LoadGitConfig()
	if err != nil {
		return err
	}

	if len(gc.Providers) == 0 {
		fmt.Println("No providers configured.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "Name\tBase URL\tModel")
	fmt.Fprintln(w, "----\t---------\t-----")
	for _, p := range gc.Providers {
		fmt.Fprintf(w, "%s\t%s\t%s\n", p.Name, p.BaseURL, p.Model)
	}
	w.Flush()

	fmt.Fprintln(os.Stderr, "\nModule references:")
	for name, mc := range map[string]coregit.ModuleConfig{"commit": gc.Commit, "review": gc.Review} {
		names := mc.Provider.Names()
		if len(names) > 0 {
			fmt.Fprintf(os.Stderr, "  %s → %s (lang: %s)\n", name, strings.Join(names, ", "), mc.Lang)
		}
	}
	return nil
}

/* ─── Module Config Subcommands ─── */

type CommitModuleCmd struct {
	Provider string `help:"Provider name(s) to use, comma-separated for fallback (e.g. 'fast,advanced')."`
	Lang     string `help:"Output language (en, cn)."`
}

func validLang(lang string) bool {
	return lang == "en" || lang == "cn"
}

func (o *CommitModuleCmd) Run() error {
	gc, err := coregit.LoadGitConfig()
	if err != nil {
		return err
	}
	if o.Provider != "" {
		names, err := llm.ParseProviderList(o.Provider)
		if err != nil {
			return err
		}
		for _, name := range names {
			if _, err := coregit.ResolveProvider(gc, name); err != nil {
				return err
			}
		}
		gc.Commit.Provider = names
	}
	if o.Lang != "" {
		if !validLang(o.Lang) {
			return fmt.Errorf("invalid language %q, must be 'en' or 'cn'", o.Lang)
		}
		gc.Commit.Lang = o.Lang
	}
	return coregit.SaveGitConfig(gc)
}

type ReviewModuleCmd struct {
	Provider      string `help:"Provider name(s) to use, comma-separated for fallback." name:"provider"`
	Lang          string `help:"Output language (en, cn)."`
	ReviewsDir    string `help:"Directory to store review reports." name:"reviews-dir"`
	ScipVersion   string `help:"Set SCIP indexer version override as lang=tag (e.g. go=v0.3.0)." name:"scip-version"`
	ScipVersionRm string `help:"Remove SCIP version override for a language (e.g. go)." name:"scip-version-rm"`
}

func (o *ReviewModuleCmd) Run() error {
	gc, err := coregit.LoadGitConfig()
	if err != nil {
		return err
	}
	if o.Provider != "" {
		names, err := llm.ParseProviderList(o.Provider)
		if err != nil {
			return err
		}
		for _, name := range names {
			if _, err := coregit.ResolveProvider(gc, name); err != nil {
				return err
			}
		}
		gc.Review.Provider = names
	}
	if o.Lang != "" {
		if !validLang(o.Lang) {
			return fmt.Errorf("invalid language %q, must be 'en' or 'cn'", o.Lang)
		}
		gc.Review.Lang = o.Lang
	}
	if o.ReviewsDir != "" {
		gc.Review.ReviewsDir = o.ReviewsDir
	}
	if o.ScipVersion != "" {
		lang, tag, ok := strings.Cut(o.ScipVersion, "=")
		if !ok || lang == "" || tag == "" {
			return fmt.Errorf("invalid --scip-version %q, expected lang=tag (e.g. go=v0.3.0)", o.ScipVersion)
		}
		if gc.Review.Scip.Versions == nil {
			gc.Review.Scip.Versions = map[string]string{}
		}
		gc.Review.Scip.Versions[lang] = tag
	}
	if o.ScipVersionRm != "" {
		if gc.Review.Scip.Versions != nil {
			delete(gc.Review.Scip.Versions, o.ScipVersionRm)
		}
	}
	return coregit.SaveGitConfig(gc)
}
