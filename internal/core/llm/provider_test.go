package llm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogResolve(t *testing.T) {
	cat := &Catalog{
		Source:    "ask-config.json",
		Providers: []Provider{{Name: "fast", BaseURL: "https://fast/v1", APIKey: "sk-f"}},
	}

	p, err := cat.Resolve("fast")
	if err != nil {
		t.Fatalf("Resolve(fast): %v", err)
	}
	if p.BaseURL != "https://fast/v1" {
		t.Errorf("Resolve(fast).BaseURL = %q, want https://fast/v1", p.BaseURL)
	}

	_, err = cat.Resolve("missing")
	if err == nil {
		t.Fatal("Resolve(missing): expected an error")
	}
	if !strings.Contains(err.Error(), "ask-config.json") {
		t.Errorf("Resolve error %q should name the source file", err)
	}

	// A catalog without a source still reports the miss, just without a file name.
	bare := &Catalog{}
	if _, err := bare.Resolve("missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("bare Resolve error = %v, want a not-found error", err)
	}
}

func TestCatalogContainsAndNames(t *testing.T) {
	cat := &Catalog{Providers: []Provider{{Name: "a"}, {Name: "b"}}}

	if !cat.Contains("a") || cat.Contains("c") {
		t.Errorf("Contains: got a=%v c=%v, want true/false", cat.Contains("a"), cat.Contains("c"))
	}
	names := cat.Names()
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Errorf("Names() = %#v, want [a b]", names)
	}
}

func TestCatalogAddRejectsDuplicate(t *testing.T) {
	cat := &Catalog{Providers: []Provider{{Name: "a"}}}

	if err := cat.Add(Provider{Name: "b"}); err != nil {
		t.Fatalf("Add(b): %v", err)
	}
	if len(cat.Providers) != 2 {
		t.Fatalf("after Add: %d providers, want 2", len(cat.Providers))
	}
	if err := cat.Add(Provider{Name: "a"}); err == nil {
		t.Error("Add(a) twice: expected an error")
	}
}

func TestCatalogSelfSufficient(t *testing.T) {
	cat := &Catalog{Providers: []Provider{
		{Name: "full", BaseURL: "https://full/v1"},
		{Name: "partial", Model: "override-only"},
	}}

	if !cat.SelfSufficient("full") {
		t.Error("SelfSufficient(full) = false, want true")
	}
	if cat.SelfSufficient("partial") {
		t.Error("SelfSufficient(partial) = true, want false: a model-only override still needs the shared base_url")
	}
	if cat.SelfSufficient("missing") {
		t.Error("SelfSufficient(missing) = true, want false")
	}
}

func TestCatalogRemove(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		cat := &Catalog{}
		if err := cat.Remove("nope", false); err == nil {
			t.Error("Remove(nope): expected an error")
		}
	})

	t.Run("refused while referenced", func(t *testing.T) {
		cat := &Catalog{Providers: []Provider{{Name: "a"}, {Name: "b"}}}
		err := cat.Remove("a", false, Reference{Module: "ask", Names: MultiProvider{"a"}})
		if err == nil {
			t.Fatal("Remove(a) while referenced: expected an error")
		}
		if !strings.Contains(err.Error(), `module "ask"`) {
			t.Errorf("error %q should name the referencing module", err)
		}
		if len(cat.Providers) != 2 {
			t.Errorf("refused removal still changed the catalog: %#v", cat.Providers)
		}
	})

	t.Run("force overrides the reference check", func(t *testing.T) {
		cat := &Catalog{Providers: []Provider{{Name: "a"}, {Name: "b"}}}
		if err := cat.Remove("a", true, Reference{Module: "ask", Names: MultiProvider{"a"}}); err != nil {
			t.Fatalf("Remove(a, force): %v", err)
		}
		if cat.Contains("a") || len(cat.Providers) != 1 {
			t.Errorf("after forced removal: %#v, want only b", cat.Providers)
		}
	})

	t.Run("unreferenced", func(t *testing.T) {
		cat := &Catalog{Providers: []Provider{{Name: "a"}, {Name: "b"}}}
		if err := cat.Remove("b", false, Reference{Module: "ask", Names: MultiProvider{"a"}}); err != nil {
			t.Fatalf("Remove(b): %v", err)
		}
		if cat.Contains("b") {
			t.Error("b is still present after Remove")
		}
	})
}

// The merge rule the whole design rests on: a module entry overrides the shared
// one field by field, so a module can change just the model of a shared provider.
func TestMergeCatalogsFieldLevelOverride(t *testing.T) {
	global := &Catalog{
		Source: "llm-config.json",
		Providers: []Provider{
			{Name: "shared", BaseURL: "https://global/v1", APIKey: "sk-global", Model: "global-model"},
			{Name: "global-only", BaseURL: "https://only/v1", APIKey: "sk-only"},
		},
	}
	module := &Catalog{
		Source: "ask-config.json",
		Providers: []Provider{
			{Name: "shared", Model: "module-model"},
			{Name: "module-only", BaseURL: "https://module/v1", APIKey: "sk-module"},
		},
	}

	got := MergeCatalogs(global, module)

	shared, err := got.Resolve("shared")
	if err != nil {
		t.Fatalf("Resolve(shared): %v", err)
	}
	if shared.Model != "module-model" {
		t.Errorf("shared.Model = %q, want the module value", shared.Model)
	}
	if shared.BaseURL != "https://global/v1" || shared.APIKey != "sk-global" {
		t.Errorf("shared = %#v, want the global base_url/api_key preserved", shared)
	}

	if _, err := got.Resolve("global-only"); err != nil {
		t.Errorf("Resolve(global-only): %v, want a globally defined provider to stay available", err)
	}
	if _, err := got.Resolve("module-only"); err != nil {
		t.Errorf("Resolve(module-only): %v, want a module-only provider to be available", err)
	}

	// Base order is preserved and override-only names are appended.
	wantOrder := []string{"shared", "global-only", "module-only"}
	if names := got.Names(); strings.Join(names, ",") != strings.Join(wantOrder, ",") {
		t.Errorf("Names() = %#v, want %#v", names, wantOrder)
	}

	// A merged miss names every file that was consulted.
	if _, err := got.Resolve("nope"); err == nil ||
		!strings.Contains(err.Error(), "llm-config.json") ||
		!strings.Contains(err.Error(), "ask-config.json") {
		t.Errorf("merged Resolve error = %v, want it to name both files", err)
	}

	// Neither input may be modified.
	if global.Providers[0].Model != "global-model" {
		t.Errorf("MergeCatalogs mutated its base: %#v", global.Providers[0])
	}
	if module.Providers[0].BaseURL != "" {
		t.Errorf("MergeCatalogs mutated its override: %#v", module.Providers[0])
	}
}

// A module that defines no providers at all must use the shared catalog as-is.
func TestMergeCatalogsEmptyOverrideKeepsGlobal(t *testing.T) {
	global := &Catalog{
		Source:    "llm-config.json",
		Providers: []Provider{{Name: "g", BaseURL: "https://g/v1", APIKey: "sk-g"}},
	}

	got := MergeCatalogs(global, &Catalog{Source: "ask-config.json"})
	if len(got.Providers) != 1 {
		t.Fatalf("got %d providers, want the global one", len(got.Providers))
	}
	if p, err := got.Resolve("g"); err != nil || p.APIKey != "sk-g" {
		t.Errorf("Resolve(g) = %#v, %v; want the untouched global provider", p, err)
	}
	if got.Source != "llm-config.json or ask-config.json" {
		t.Errorf("Source = %q, want both files named", got.Source)
	}
}

func TestMergeCatalogsNilInputs(t *testing.T) {
	got := MergeCatalogs(nil, nil)
	if got == nil || len(got.Providers) != 0 {
		t.Fatalf("MergeCatalogs(nil, nil) = %#v, want an empty catalog", got)
	}
	if got.Source != "" {
		t.Errorf("Source = %q, want empty", got.Source)
	}
}

func TestResolveNames(t *testing.T) {
	cat := &Catalog{Source: "llm-config.json", Providers: []Provider{{Name: "fast"}, {Name: "backup"}}}

	got, err := ResolveNames(cat, "backup", MultiProvider{"fast"})
	if err != nil {
		t.Fatalf("ResolveNames(backup): %v", err)
	}
	if len(got) != 1 || got[0] != "backup" {
		t.Errorf("ResolveNames(backup) = %#v, want [backup]", got)
	}

	if _, err := ResolveNames(cat, "typo", nil); err == nil {
		t.Error("ResolveNames(typo): expected an error")
	}

	got, err = ResolveNames(cat, "", MultiProvider{"fast", "backup"})
	if err != nil {
		t.Fatalf("ResolveNames(\"\"): %v", err)
	}
	if len(got) != 2 {
		t.Errorf("ResolveNames(\"\") = %#v, want the configured list", got)
	}

	got, err = ResolveNames(cat, "", nil)
	if err != nil || got != nil {
		t.Errorf("ResolveNames(\"\", nil) = %#v, %v; want nil, nil", got, err)
	}
}

func TestEffectiveClientConfig(t *testing.T) {
	p := &Provider{BaseURL: "https://p/v1", APIKey: "sk-p", Model: "p-model"}

	baseURL, apiKey, model := EffectiveClientConfig(p, "", "", "")
	if baseURL != "https://p/v1" || apiKey != "sk-p" || model != "p-model" {
		t.Errorf("no flags: got %q %q %q, want the provider values", baseURL, apiKey, model)
	}

	baseURL, apiKey, model = EffectiveClientConfig(p, "https://flag/v1", "sk-flag", "flag-model")
	if baseURL != "https://flag/v1" || apiKey != "sk-flag" || model != "flag-model" {
		t.Errorf("flags win: got %q %q %q", baseURL, apiKey, model)
	}

	baseURL, apiKey, model = EffectiveClientConfig(p, "", "sk-flag", "")
	if baseURL != "https://p/v1" || apiKey != "sk-flag" || model != "p-model" {
		t.Errorf("partial flags: got %q %q %q", baseURL, apiKey, model)
	}
}

func TestJoinSourcesDeduplicates(t *testing.T) {
	if got := joinSources("a.json", "a.json", "", "b.json"); got != "a.json or b.json" {
		t.Errorf("joinSources = %q, want %q", got, "a.json or b.json")
	}
	if got := joinSources("", ""); got != "" {
		t.Errorf("joinSources(empty) = %q, want empty", got)
	}
}

func TestInheritedNames(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "llm-config.json")
	global := &Catalog{Providers: []Provider{
		{Name: "g1", BaseURL: "https://g1/v1"},
		{Name: "overridden", BaseURL: "https://global/v1", Model: "global"},
		{Name: "fully-overridden", BaseURL: "https://other/v1"},
	}}
	if err := SaveGlobalCatalog(globalPath, global); err != nil {
		t.Fatalf("SaveGlobalCatalog: %v", err)
	}

	module := &Catalog{Providers: []Provider{
		{Name: "overridden", Model: "module"},
		{Name: "fully-overridden", BaseURL: "https://module/v1"},
	}}

	got := InheritedNames(module, globalPath)
	// g1 is not defined locally; "overridden" still needs the shared base_url.
	// "fully-overridden" stands on its own and is therefore not inherited.
	want := "g1,overridden"
	if strings.Join(got, ",") != want {
		t.Errorf("InheritedNames = %#v, want %q", got, want)
	}
}

func TestExplainInheritedProvider(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "llm-config.json")
	if err := SaveGlobalCatalog(globalPath, &Catalog{Providers: []Provider{{Name: "g1"}}}); err != nil {
		t.Fatalf("SaveGlobalCatalog: %v", err)
	}

	// Inherited: the message must point at the shared file and the removal command.
	err := ExplainInheritedProvider("g1", &Catalog{}, globalPath)
	if err == nil {
		t.Fatal("expected an error for an inherited provider")
	}
	if !strings.Contains(err.Error(), GlobalConfigFileName) || !strings.Contains(err.Error(), "mu set llm provider rm") {
		t.Errorf("error %q should name the shared file and the removal command", err)
	}

	// Defined by the module itself: removing it locally is legitimate.
	if err := ExplainInheritedProvider("g1", &Catalog{Providers: []Provider{{Name: "g1"}}}, globalPath); err != nil {
		t.Errorf("module-local provider: unexpected error %v", err)
	}

	// Unknown everywhere: let the normal not-found path report it.
	if err := ExplainInheritedProvider("nope", &Catalog{}, globalPath); err != nil {
		t.Errorf("unknown provider: unexpected error %v", err)
	}
}

func TestDependentReferences(t *testing.T) {
	// A complete module-local definition satisfies the reference on its own.
	module := &Catalog{Providers: []Provider{{Name: "local", BaseURL: "https://local/v1"}}}
	refs := []Reference{
		{Module: "commit", Names: MultiProvider{"local", "shared"}},
		{Module: "review", Names: MultiProvider{"local"}},
		{Module: "ask", Names: nil},
	}

	got := DependentReferences(module, refs)
	if len(got) != 1 {
		t.Fatalf("DependentReferences = %#v, want only the commit reference", got)
	}
	if got[0].Module != "commit" || len(got[0].Names) != 1 || got[0].Names[0] != "shared" {
		t.Errorf("DependentReferences = %#v, want commit → [shared]", got)
	}
}

// A module entry that only overrides a field still depends on the shared entry:
// removing it would leave the module with a provider that has no endpoint.
func TestDependentReferencesCountsPartialOverride(t *testing.T) {
	module := &Catalog{Providers: []Provider{{Name: "shared", Model: "override-only"}}}
	refs := []Reference{{Module: "ask", Names: MultiProvider{"shared"}}}

	got := DependentReferences(module, refs)
	if len(got) != 1 {
		t.Fatalf("DependentReferences = %#v, want the partially overridden provider kept", got)
	}
	if got[0].Names[0] != "shared" {
		t.Errorf("DependentReferences = %#v, want shared", got)
	}
}

func TestValidateProviderAdd(t *testing.T) {
	globalPath := filepath.Join(t.TempDir(), "llm-config.json")
	if err := SaveGlobalCatalog(globalPath, &Catalog{Providers: []Provider{
		{Name: "shared", BaseURL: "https://s/v1", APIKey: "sk-s", Model: "s-model"},
	}}); err != nil {
		t.Fatalf("SaveGlobalCatalog: %v", err)
	}

	t.Run("new name needs a base url and a key", func(t *testing.T) {
		if err := ValidateProviderAdd(&Catalog{}, "new", "", "", globalPath); err == nil {
			t.Error("no fields: expected an error")
		}
		if err := ValidateProviderAdd(&Catalog{}, "new", "https://n/v1", "", globalPath); err == nil {
			t.Error("no api key: expected an error")
		}
		if err := ValidateProviderAdd(&Catalog{}, "new", "https://n/v1", "sk-n", globalPath); err != nil {
			t.Errorf("complete new provider: %v", err)
		}
	})

	t.Run("a shared name may be overridden field by field", func(t *testing.T) {
		if err := ValidateProviderAdd(&Catalog{}, "shared", "", "", globalPath); err != nil {
			t.Errorf("model-only override: %v", err)
		}
		if err := ValidateProviderAdd(&Catalog{}, "shared", "", "sk-new", globalPath); err != nil {
			t.Errorf("key-only override: %v", err)
		}
	})

	t.Run("a duplicate in the module catalog is refused", func(t *testing.T) {
		module := &Catalog{Providers: []Provider{{Name: "shared", BaseURL: "https://own/v1"}}}
		if err := ValidateProviderAdd(module, "shared", "https://own/v1", "sk", globalPath); err == nil {
			t.Error("duplicate: expected an error")
		}
	})

	t.Run("an empty name is refused", func(t *testing.T) {
		if err := ValidateProviderAdd(&Catalog{}, "", "https://n/v1", "sk", globalPath); err == nil {
			t.Error("empty name: expected an error")
		}
	})
}

// An existing ask-config.json with inline providers must load exactly as before and
// must not be rewritten just because the shared catalog now exists.
func TestLegacyAskConfigStillLoadsAndIsNotRewritten(t *testing.T) {
	content := `{
  "base_url": "https://legacy/v1",
  "api_key": "sk-legacy",
  "model": "legacy-model",
  "providers": [
    {"name": "fast", "base_url": "https://fast/v1", "api_key": "sk-f", "model": "m"}
  ],
  "provider": ["fast"]
}`
	path := writeTemp(t, "ask-config.json", content)

	cfg, err := LoadConfigFromPath(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Providers) != 1 || cfg.Providers[0].Name != "fast" {
		t.Fatalf("providers = %#v, want the inline 'fast' provider", cfg.Providers)
	}
	if cfg.Source != "ask-config.json" {
		t.Errorf("Source = %q, want ask-config.json", cfg.Source)
	}
	if len(cfg.Provider) != 1 || cfg.Provider[0] != "fast" {
		t.Errorf("Provider = %#v, want [fast]", cfg.Provider)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(after) != content {
		t.Error("loading rewrote the config file; a read must never migrate silently")
	}
}

// The shared catalog round-trips with 0600 permissions and a 0700 directory.
func TestGlobalCatalogRoundTripPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mu")
	path := filepath.Join(dir, "llm-config.json")

	want := &Catalog{Providers: []Provider{{Name: "fast", BaseURL: "https://fast/v1", APIKey: "sk-f", Model: "m"}}}
	if err := SaveGlobalCatalog(path, want); err != nil {
		t.Fatalf("SaveGlobalCatalog: %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("file mode = %o, want 600", got)
	}
	di, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if got := di.Mode().Perm(); got != 0o700 {
		t.Errorf("dir mode = %o, want 700", got)
	}

	got, err := LoadGlobalCatalog(path)
	if err != nil {
		t.Fatalf("LoadGlobalCatalog: %v", err)
	}
	if len(got.Providers) != 1 || got.Providers[0] != want.Providers[0] {
		t.Errorf("round-trip = %#v, want %#v", got.Providers, want.Providers)
	}
	if got.Source != "llm-config.json" {
		t.Errorf("Source = %q, want llm-config.json", got.Source)
	}
}

// A missing shared catalog is not an error: modules keep working from their own
// providers, exactly as they did before llm-config.json existed.
func TestLoadGlobalCatalogMissingFile(t *testing.T) {
	cat, err := LoadGlobalCatalog(filepath.Join(t.TempDir(), "missing", "llm-config.json"))
	if err != nil {
		t.Fatalf("LoadGlobalCatalog: %v", err)
	}
	if cat == nil || len(cat.Providers) != 0 {
		t.Fatalf("got %#v, want an empty catalog", cat)
	}
}

func TestEffectiveCatalogMergesFiles(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "llm-config.json")
	if err := SaveGlobalCatalog(globalPath, &Catalog{Providers: []Provider{
		{Name: "shared", BaseURL: "https://global/v1", APIKey: "sk-global", Model: "global-model"},
	}}); err != nil {
		t.Fatalf("SaveGlobalCatalog: %v", err)
	}

	module := &Catalog{
		Source:    "git-config.json",
		Providers: []Provider{{Name: "shared", Model: "module-model"}},
	}

	got, err := EffectiveCatalog(globalPath, module)
	if err != nil {
		t.Fatalf("EffectiveCatalog: %v", err)
	}
	p, err := got.Resolve("shared")
	if err != nil {
		t.Fatalf("Resolve(shared): %v", err)
	}
	if p.Model != "module-model" || p.BaseURL != "https://global/v1" || p.APIKey != "sk-global" {
		t.Errorf("merged provider = %#v, want module model on the global endpoint", p)
	}
}
