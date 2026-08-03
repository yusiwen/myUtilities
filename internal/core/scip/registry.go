package scip

// InstallMethod describes how an indexer binary is distributed.
type InstallMethod string

const (
	// MethodGitHubRelease downloads a prebuilt binary from a GitHub release.
	// Reuses core/installer for asset resolution (OS/arch/SHA256).
	MethodGitHubRelease InstallMethod = "github_release"
	// MethodNpm installs via npx/npm -g (requires Node). Reserved for v2.
	MethodNpm InstallMethod = "npm"
	// MethodPip installs via pip (requires Python). Reserved for v2.
	MethodPip InstallMethod = "pip"
)

// Indexer describes a SCIP indexer for one language.
type Indexer struct {
	// Lang is the canonical language name, e.g. "go".
	Lang string
	// Detect lists signals matched against the repo root to identify
	// this language: manifest filenames ("go.mod") or globs ("*.go").
	Detect []string
	// GitHubRepo is the "owner/repo" used when Install == MethodGitHubRelease.
	GitHubRepo string
	// Version pins the release tag, e.g. "v0.4.0".
	Version string
	// Install is the distribution method for the indexer binary.
	Install InstallMethod
	// Requires lists runtimes the indexer needs on PATH at index time,
	// e.g. ["go"] because scip-go runs `go list` internally.
	Requires []string
	// BinaryName is the executable name after installation.
	BinaryName string
	// OutputFile is the name of the index file the indexer produces,
	// typically "index.scip".
	OutputFile string
	// Disable marks indexers that additionally require a build system
	// (e.g. compile_commands.json, a JVM build). They are registered but
	// not used by the auto-detection path.
	Disable bool
	// AssetName pins the exact GitHub release asset name to download,
	// bypassing OS/arch name matching. Used by cross-platform launchers
	// whose release assets do not encode the target platform.
	AssetName string
	// Prefix is the fixed leading subcommand arguments, e.g. ["index"] for
	// "scip-java index".
	Prefix []string
	// OutputFlag is the CLI flag that takes the index output path, e.g.
	// "-o" (scip-go) or "--output" (scip-java).
	OutputFlag string
	// QuietFlag is the CLI flag that suppresses indexer output, appended in
	// non-verbose mode. Empty for indexers without one (output is captured
	// to a temp file anyway).
	QuietFlag string
	// Trailing holds arguments appended after the output path.
	Trailing []string
	// FailHard marks indexers whose build failure aborts the review instead
	// of silently falling back to text tools (e.g. scip-java runs a real
	// build; a failure means semantic review is impossible).
	FailHard bool
}

// registry holds the known indexers, ordered so that more specific or
// zero-friction entries come first.
var registry = []*Indexer{
	{
		Lang:       "go",
		Detect:     []string{"go.mod", "go.sum", "*.go"},
		GitHubRepo: "scip-code/scip-go",
		Version:    "v0.2.7",
		Install:    MethodGitHubRelease,
		Requires:   []string{"go"},
		BinaryName: "scip-go",
		OutputFile: "index.scip",
		OutputFlag: "-o",
		QuietFlag:  "-q",
	},
	{
		Lang:       "typescript",
		Detect:     []string{"tsconfig.json", "package.json"},
		GitHubRepo: "scip-code/scip-typescript",
		Install:    MethodNpm,
		Requires:   []string{"node"},
		BinaryName: "scip-typescript",
		OutputFile: "index.scip",
	},
	{
		Lang:       "java",
		Detect:     []string{"pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle", "gradlew", "*.java"},
		GitHubRepo: "scip-code/scip-java",
		Version:    "v0.13.1",
		Install:    MethodGitHubRelease,
		Requires:   []string{"java"},
		BinaryName: "scip-java",
		OutputFile: "index.scip",
		AssetName:  "scip-java-v0.13.1",
		Prefix:     []string{"index"},
		OutputFlag: "--output",
		FailHard:   true,
	},
	{
		Lang:       "c",
		Detect:     []string{"compile_commands.json"},
		GitHubRepo: "scip-code/scip-clang",
		Install:    MethodGitHubRelease,
		Requires:   []string{"clang"},
		BinaryName: "scip-clang",
		OutputFile: "index.scip",
		Disable:    true,
	},
}

// Registry returns a copy of the known indexers.
func Registry() []*Indexer {
	out := make([]*Indexer, len(registry))
	copy(out, registry)
	return out
}

// LookupLang returns the indexer registered for lang, if any.
func LookupLang(lang string) (*Indexer, bool) {
	for _, ix := range registry {
		if ix.Lang == lang {
			return ix, true
		}
	}
	return nil, false
}

// Enabled returns the indexers available for the auto-detection path
// (not disabled, GitHub-release distributed for v1).
func Enabled() []*Indexer {
	var out []*Indexer
	for _, ix := range registry {
		if !ix.Disable && ix.Install == MethodGitHubRelease {
			out = append(out, ix)
		}
	}
	return out
}
