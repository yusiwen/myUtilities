package llm

import (
	"fmt"
	"strings"
)

// Provider is one named LLM endpoint. The same struct backs the shared
// llm-config.json catalog and the per-module overrides, so an endpoint can be
// defined once globally and adjusted per module by name.
type Provider struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

// Catalog is a set of named providers. Every module config embeds one (ask, git)
// and the global llm-config.json is loaded as one, so lookups, overrides and
// removal checks have a single implementation.
type Catalog struct {
	Providers []Provider `json:"providers,omitempty"`

	// Source names the file this catalog was read from. It appears in error
	// messages only and is never serialized.
	Source string `json:"-"`
}

// NewCatalog returns an empty catalog whose errors name source.
func NewCatalog(source string) *Catalog {
	return &Catalog{Source: source}
}

// Resolve returns the provider with the given name.
func (c *Catalog) Resolve(name string) (*Provider, error) {
	for i := range c.Providers {
		if c.Providers[i].Name == name {
			return &c.Providers[i], nil
		}
	}
	if c.Source != "" {
		return nil, fmt.Errorf("provider %q not found in %s", name, c.Source)
	}
	return nil, fmt.Errorf("provider %q not found", name)
}

// Names returns the configured provider names in file order.
func (c *Catalog) Names() []string {
	names := make([]string, 0, len(c.Providers))
	for _, p := range c.Providers {
		names = append(names, p.Name)
	}
	return names
}

// Contains reports whether a provider with that name exists.
func (c *Catalog) Contains(name string) bool {
	for _, p := range c.Providers {
		if p.Name == name {
			return true
		}
	}
	return false
}

// SelfSufficient reports whether this catalog's own definition of name is complete
// enough to keep working once a shared entry of the same name is gone. An entry
// that only overrides a field — typically just the model — is not self-sufficient:
// it still needs the shared catalog for its base_url.
func (c *Catalog) SelfSufficient(name string) bool {
	p, err := c.Resolve(name)
	return err == nil && p.BaseURL != ""
}

// Add appends a provider, refusing a duplicate name.
func (c *Catalog) Add(p Provider) error {
	if c.Contains(p.Name) {
		return fmt.Errorf("provider %q already exists", p.Name)
	}
	c.Providers = append(c.Providers, p)
	return nil
}

// Reference names one module's provider list. It is what keeps a removal check
// module-aware without the catalog having to know about individual modules.
type Reference struct {
	Module string
	Names  MultiProvider
}

// Remove deletes the named provider. Unless force is set, removal is refused
// while any given reference still names it, so a config cannot be left pointing
// at a provider that no longer exists.
func (c *Catalog) Remove(name string, force bool, refs ...Reference) error {
	idx := -1
	for i, p := range c.Providers {
		if p.Name == name {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("provider %q not found", name)
	}
	if !force {
		for _, r := range refs {
			if r.Names.Contains(name) {
				return fmt.Errorf("cannot remove provider %q: module %q is using it (--force overrides)", name, r.Module)
			}
		}
	}
	c.Providers = append(c.Providers[:idx], c.Providers[idx+1:]...)
	return nil
}

// MergeCatalogs returns the union of base (normally the global llm-config.json)
// and override (a module config) without modifying either input.
//
// A name present in both is merged field by field: a non-empty override field
// wins, an empty one leaves the base value in place. That is what lets a module
// change just the model of a shared provider without restating its endpoint and
// key. A name defined only in override or only in base is available unchanged;
// base entries keep their position and override-only entries are appended in
// override order.
func MergeCatalogs(base, override *Catalog) *Catalog {
	if base == nil {
		base = &Catalog{}
	}
	if override == nil {
		override = &Catalog{}
	}

	out := &Catalog{Source: joinSources(base.Source, override.Source)}
	index := make(map[string]int, len(base.Providers)+len(override.Providers))
	for _, p := range base.Providers {
		index[p.Name] = len(out.Providers)
		out.Providers = append(out.Providers, p)
	}
	for _, p := range override.Providers {
		i, ok := index[p.Name]
		if !ok {
			index[p.Name] = len(out.Providers)
			out.Providers = append(out.Providers, p)
			continue
		}
		merged := out.Providers[i]
		if p.BaseURL != "" {
			merged.BaseURL = p.BaseURL
		}
		if p.APIKey != "" {
			merged.APIKey = p.APIKey
		}
		if p.Model != "" {
			merged.Model = p.Model
		}
		out.Providers[i] = merged
	}
	return out
}

// joinSources renders the files a merged catalog was assembled from, so a lookup
// failure names every file that was consulted.
func joinSources(sources ...string) string {
	seen := make(map[string]bool, len(sources))
	var unique []string
	for _, s := range sources {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		unique = append(unique, s)
	}
	return strings.Join(unique, " or ")
}

// ResolveNames returns the provider chain to use: the explicit override when it
// is non-empty (resolved eagerly, so a typo fails before any request is made),
// otherwise the module's configured list.
func ResolveNames(cat *Catalog, override string, configured MultiProvider) (MultiProvider, error) {
	if override == "" {
		return configured, nil
	}
	p, err := cat.Resolve(override)
	if err != nil {
		return nil, err
	}
	return MultiProvider{p.Name}, nil
}

// EffectiveClientConfig merges a provider's own settings with per-invocation flag
// overrides; a non-empty flag always wins.
func EffectiveClientConfig(p *Provider, flagBaseURL, flagAPIKey, flagModel string) (baseURL, apiKey, model string) {
	baseURL = p.BaseURL
	apiKey = p.APIKey
	model = p.Model
	if flagBaseURL != "" {
		baseURL = flagBaseURL
	}
	if flagAPIKey != "" {
		apiKey = flagAPIKey
	}
	if flagModel != "" {
		model = flagModel
	}
	return
}
