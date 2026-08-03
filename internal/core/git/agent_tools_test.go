package git

import (
	"os"
	"strings"
	"testing"

	scipb "github.com/scip-code/scip/bindings/go/scip"
	"github.com/yusiwen/myUtilities/internal/core/openai"
	"github.com/yusiwen/myUtilities/internal/core/scip"
)

/* ─── Test helpers ─── */

const scipTestFileContent = "package git\n\n// Worker runs tasks.\nfunc Worker() {\n    return\n}"

func occ(symbol string, line, startChar, endChar int, role scipb.SymbolRole) *scipb.Occurrence {
	return &scipb.Occurrence{
		TypedRange:  &scipb.Occurrence_SingleLineRange{SingleLineRange: &scipb.SingleLineRange{Line: int32(line), StartCharacter: int32(startChar), EndCharacter: int32(endChar)}},
		Symbol:      symbol,
		SymbolRoles: int32(role),
	}
}

// fakeWorkerIndex returns a two-file index where Worker is defined in lib.go
// and used from main.go, plus a file (x.go) whose symbol has no definition.
func fakeWorkerIndex() *scip.IndexSet {
	raw := &scipb.Index{
		Documents: []*scipb.Document{
			{
				RelativePath: "lib.go",
				Occurrences: []*scipb.Occurrence{
					occ("repo Worker", 0, 5, 11, scipb.SymbolRole_Definition),
					occ("repo Worker", 4, 2, 8, scipb.SymbolRole_ReadAccess),
				},
				Symbols: []*scipb.SymbolInformation{
					{Symbol: "repo Worker", DisplayName: "Worker", Kind: scipb.SymbolInformation_Function,
						SignatureDocumentation: &scipb.Signature{Text: "func Worker()"},
						Documentation:          []string{"doc line one", "doc line two"}},
				},
			},
			{
				RelativePath: "main.go",
				Occurrences: []*scipb.Occurrence{
					occ("repo Worker", 1, 0, 6, scipb.SymbolRole_ReadAccess),
				},
			},
			{
				// x.go's symbol appears only as a reference; no definition exists.
				RelativePath: "x.go",
				Occurrences: []*scipb.Occurrence{
					occ("repo Mystery", 9, 4, 11, scipb.SymbolRole_ReadAccess),
				},
			},
		},
	}
	set := scip.NewIndexSet("/repo")
	set.Add("go", scip.NewIndex(raw))
	return set
}

// fakeScipFuncIndex indexes the _scip_func.go test file with Worker defined
// on 0-based line 3 (1-based line 4).
func fakeScipFuncIndex() *scip.IndexSet {
	raw := &scipb.Index{
		Documents: []*scipb.Document{
			{
				RelativePath: "_scip_func.go",
				Occurrences: []*scipb.Occurrence{
					occ("repo Worker", 3, 5, 11, scipb.SymbolRole_Definition),
					occ("repo Worker", 5, 4, 10, scipb.SymbolRole_ReadAccess),
				},
				Symbols: []*scipb.SymbolInformation{
					{Symbol: "repo Worker", DisplayName: "Worker", Kind: scipb.SymbolInformation_Function},
				},
			},
		},
	}
	set := scip.NewIndexSet("/repo")
	set.Add("go", scip.NewIndex(raw))
	return set
}

// fakeLateDefIndex indexes the same test file but with the definition far
// below the queried lines, so EnclosingDefLine finds nothing above them.
func fakeLateDefIndex() *scip.IndexSet {
	raw := &scipb.Index{
		Documents: []*scipb.Document{
			{
				RelativePath: "_scip_func.go",
				Occurrences: []*scipb.Occurrence{
					occ("repo Worker", 50, 5, 11, scipb.SymbolRole_Definition),
				},
			},
		},
	}
	set := scip.NewIndexSet("/repo")
	set.Add("go", scip.NewIndex(raw))
	return set
}

func writeScipTestFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(name) })
}

/* ─── toolFindReferences ─── */

func TestToolFindReferencesFormatted(t *testing.T) {
	a := &ReviewAgent{indexSet: fakeWorkerIndex()}
	out := a.toolFindReferences("main.go", 2)

	if !strings.Contains(out, "references for symbol at main.go:2") {
		t.Fatalf("missing header in output: %q", out)
	}
	if !strings.Contains(out, "[definition]") {
		t.Fatalf("expected a [definition] location: %q", out)
	}
	if !strings.Contains(out, "[usage]") {
		t.Fatalf("expected a [usage] location: %q", out)
	}
	if !strings.Contains(out, "lib.go:1:") {
		t.Fatalf("expected the lib.go definition line in output: %q", out)
	}
}

func TestToolFindReferencesNilIndex(t *testing.T) {
	a := &ReviewAgent{} // indexSet nil
	got := a.toolFindReferences("x.go", 1)
	want := "error: semantic index is not available. Use search_code instead."
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestToolFindReferencesErrNoIndex(t *testing.T) {
	a := &ReviewAgent{indexSet: fakeWorkerIndex()}
	got := a.toolFindReferences("nothere.go", 1)
	if !strings.Contains(got, "error: no index available for nothere.go") {
		t.Fatalf("expected ErrNoIndex message, got %q", got)
	}
}

func TestToolFindReferencesNoneFound(t *testing.T) {
	a := &ReviewAgent{indexSet: fakeWorkerIndex()}
	// x.go:1 has no occurrences.
	got := a.toolFindReferences("x.go", 1)
	if !strings.Contains(got, "no references found for the symbol at x.go:1") {
		t.Fatalf("expected no-references message, got %q", got)
	}
}

/* ─── toolFindDefinition ─── */

func TestToolFindDefinitionFormatted(t *testing.T) {
	a := &ReviewAgent{indexSet: fakeWorkerIndex()}
	out := a.toolFindDefinition("main.go", 2)

	if !strings.Contains(out, "definitions for symbol at main.go:2") {
		t.Fatalf("missing header in output: %q", out)
	}
	if !strings.Contains(out, "lib.go:1:5 [definition]") {
		t.Fatalf("expected the definition location in output: %q", out)
	}
}

func TestToolFindDefinitionNilIndex(t *testing.T) {
	a := &ReviewAgent{}
	got := a.toolFindDefinition("x.go", 1)
	want := "error: semantic index is not available. Use search_code instead."
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestToolFindDefinitionErrNoIndex(t *testing.T) {
	a := &ReviewAgent{indexSet: fakeWorkerIndex()}
	got := a.toolFindDefinition("nothere.go", 1)
	if !strings.Contains(got, "error: no index available for nothere.go") {
		t.Fatalf("expected ErrNoIndex message, got %q", got)
	}
}

func TestToolFindDefinitionNoneFound(t *testing.T) {
	a := &ReviewAgent{indexSet: fakeWorkerIndex()}
	// x.go:10 references a symbol that has no definition anywhere.
	got := a.toolFindDefinition("x.go", 10)
	if !strings.Contains(got, "no definition found for the symbol at x.go:10") {
		t.Fatalf("expected no-definition message, got %q", got)
	}
}

/* ─── toolSymbolInfo ─── */

func TestToolSymbolInfoFull(t *testing.T) {
	a := &ReviewAgent{indexSet: fakeWorkerIndex()}
	out := a.toolSymbolInfo("lib.go", 1)

	for _, want := range []string{
		"symbol: repo Worker",
		"name: Worker",
		"kind: Function",
		"signature: func Worker()",
		"documentation:",
		"doc line one",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %q", want, out)
		}
	}
}

func TestToolSymbolInfoNilIndex(t *testing.T) {
	a := &ReviewAgent{}
	got := a.toolSymbolInfo("x.go", 1)
	want := "error: semantic index is not available."
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestToolSymbolInfoErrNoIndex(t *testing.T) {
	a := &ReviewAgent{indexSet: fakeWorkerIndex()}
	got := a.toolSymbolInfo("nothere.go", 1)
	if !strings.Contains(got, "error: no index available for nothere.go") {
		t.Fatalf("expected ErrNoIndex message, got %q", got)
	}
}

func TestToolSymbolInfoNoSymbolAtLine(t *testing.T) {
	a := &ReviewAgent{indexSet: fakeWorkerIndex()}
	got := a.toolSymbolInfo("x.go", 1)
	if !strings.Contains(got, "error: no symbol found at x.go:1") {
		t.Fatalf("expected no-symbol message, got %q", got)
	}
}

func TestToolSymbolInfoOmitsEmptyFields(t *testing.T) {
	a := &ReviewAgent{indexSet: fakeWorkerIndex()}
	out := a.toolSymbolInfo("x.go", 10) // Mystery symbol has no SymbolInformation
	if !strings.Contains(out, "symbol: repo Mystery") {
		t.Fatalf("expected bare symbol output, got %q", out)
	}
	for _, absent := range []string{"name:", "kind:", "signature:", "documentation:"} {
		if strings.Contains(out, absent) {
			t.Fatalf("output should omit %q for a symbol without info: %q", absent, out)
		}
	}
}

/* ─── toolReadFunction ─── */

func TestToolReadFunctionUsesScipDefLine(t *testing.T) {
	writeScipTestFile(t, "_scip_func.go", scipTestFileContent)
	a := &ReviewAgent{indexSet: fakeScipFuncIndex()}
	out := a.toolReadFunction("_scip_func.go", 6)

	if !strings.Contains(out, "(lines 4-6 of 6)") {
		t.Fatalf("expected read to start at the enclosing function def line 4, got header line: %q", out)
	}
	if !strings.Contains(out, "func Worker() {") {
		t.Fatalf("expected function body in output: %q", out)
	}
}

func TestToolReadFunctionFallbackNoIndex(t *testing.T) {
	writeScipTestFile(t, "_scip_func.go", scipTestFileContent)
	a := &ReviewAgent{} // no index
	out := a.toolReadFunction("_scip_func.go", 6)

	if !strings.Contains(out, "(lines 1-6 of 6)") {
		t.Fatalf("expected fallback window to start at line 1, got header line: %q", out)
	}
	if !strings.Contains(out, "func Worker() {") {
		t.Fatalf("expected function body in output: %q", out)
	}
}

func TestToolReadFunctionFallbackWhenNoEnclosingDef(t *testing.T) {
	writeScipTestFile(t, "_scip_func.go", scipTestFileContent)
	// The index has the def far below line 2, so EnclosingDefLine is 0 and
	// the tool must fall back to the ±30 window.
	a := &ReviewAgent{indexSet: fakeLateDefIndex()}
	out := a.toolReadFunction("_scip_func.go", 2)

	if !strings.Contains(out, "(lines 1-6 of 6)") {
		t.Fatalf("expected fallback window when no enclosing def exists, got header line: %q", out)
	}
}

func TestToolReadFunctionMissingFile(t *testing.T) {
	a := &ReviewAgent{indexSet: fakeWorkerIndex()}
	out := a.toolReadFunction("_does_not_exist.go", 5)
	if !strings.Contains(out, "error reading") {
		t.Fatalf("expected read error, got %q", out)
	}
}

/* ─── formatLocations ─── */

func TestFormatLocations(t *testing.T) {
	locs := []scip.Location{
		{Path: "a.go", Line: 1, Character: 0, Symbol: "repo S", IsDef: true},
		{Path: "b.go", Line: 2, Character: 5, Symbol: "repo S"},
	}
	out := formatLocations("references", "main.go", 3, locs)

	for _, want := range []string{
		"references for symbol at main.go:3 (2 locations):",
		"  a.go:1:0 [definition] repo S",
		"  b.go:2:5 [usage] repo S",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("formatLocations missing %q:\n%s", want, out)
		}
	}
}

func TestFormatLocationsEmpty(t *testing.T) {
	out := formatLocations("references", "a.go", 1, nil)
	if !strings.Contains(out, "references for symbol at a.go:1 (0 locations):") {
		t.Fatalf("unexpected empty header: %q", out)
	}
}

/* ─── executeTool dispatch ─── */

func TestExecuteToolDispatch(t *testing.T) {
	a := &ReviewAgent{indexSet: fakeWorkerIndex()}

	cases := []struct {
		name, args, wantSub string
	}{
		{"find_references", `{"path":"main.go","line":2}`, "references for symbol at main.go:2"},
		{"find_definition", `{"path":"main.go","line":2}`, "definitions for symbol at main.go:2"},
		{"symbol_info", `{"path":"lib.go","line":1}`, "symbol: repo Worker"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tc := openai.ToolCall{ID: "1", Function: openai.ToolCallFunc{Name: c.name, Arguments: c.args}}
			got := a.executeTool(tc)
			if !strings.Contains(got, c.wantSub) {
				t.Fatalf("executeTool(%s) = %q, want substring %q", c.name, got, c.wantSub)
			}
		})
	}
}

func TestExecuteToolReadFunctionDispatch(t *testing.T) {
	writeScipTestFile(t, "_scip_func.go", scipTestFileContent)
	a := &ReviewAgent{indexSet: fakeScipFuncIndex()}
	tc := openai.ToolCall{ID: "1", Function: openai.ToolCallFunc{Name: "read_function", Arguments: `{"path":"_scip_func.go","line":6}`}}
	got := a.executeTool(tc)
	if !strings.Contains(got, "file: _scip_func.go") || !strings.Contains(got, "func Worker()") {
		t.Fatalf("read_function dispatch produced unexpected output: %q", got)
	}
}

func TestExecuteToolUnknown(t *testing.T) {
	a := &ReviewAgent{}
	tc := openai.ToolCall{Function: openai.ToolCallFunc{Name: "bogus", Arguments: "{}"}}
	if got := a.executeTool(tc); got != "unknown tool: bogus" {
		t.Fatalf("got %q, want %q", got, "unknown tool: bogus")
	}
}

func TestExecuteToolBadJSON(t *testing.T) {
	a := &ReviewAgent{indexSet: fakeWorkerIndex()}
	for _, name := range []string{"find_references", "find_definition", "symbol_info", "read_function", "read_file"} {
		tc := openai.ToolCall{Function: openai.ToolCallFunc{Name: name, Arguments: `{bad json`}}
		if got := a.executeTool(tc); !strings.Contains(got, "error parsing args:") {
			t.Fatalf("executeTool(%s) with bad JSON = %q, want parsing error", name, got)
		}
	}
}

func TestExecuteToolEmptyArgsNoPanic(t *testing.T) {
	a := &ReviewAgent{indexSet: fakeWorkerIndex()}
	for _, name := range []string{"find_references", "find_definition", "symbol_info", "read_function"} {
		tc := openai.ToolCall{Function: openai.ToolCallFunc{Name: name, Arguments: `{}`}}
		if got := a.executeTool(tc); got == "" {
			t.Fatalf("executeTool(%s) with {} returned empty", name)
		}
	}
}

/* ─── formatToolArgs ─── */

func TestFormatToolArgs(t *testing.T) {
	cases := []struct {
		name, args, want string
	}{
		{"find_references", `{"path":"a.go","line":4}`, "a.go:4"},
		{"find_definition", `{"path":"a.go","line":4}`, "a.go:4"},
		{"symbol_info", `{"path":"a.go","line":4}`, "a.go:4"},
		{"read_function", `{"path":"a.go","line":4}`, "a.go:4"},
		{"read_file", `{"path":"a.go","line":3}`, "a.go:3"},
		{"read_file", `{"path":"a.go"}`, "a.go"},
		{"read_diff", `{"path":""}`, "full diff"},
		{"search_code", `{"pattern":"Foo"}`, "Foo"},
		{"bogus", `{}`, ""},
	}
	for _, c := range cases {
		if got := formatToolArgs(c.name, c.args); got != c.want {
			t.Errorf("formatToolArgs(%s) = %q, want %q", c.name, got, c.want)
		}
	}
}

/* ─── truncateToolResult ─── */

func TestTruncateToolResultShort(t *testing.T) {
	in := "hello world"
	if got := truncateToolResult(in); got != in {
		t.Fatalf("short input must pass through unchanged, got %q", got)
	}
}

func TestTruncateToolResultLong(t *testing.T) {
	in := strings.Repeat("a", 30001)
	got := truncateToolResult(in)
	if len(got) != 30000+len("\n...(tool result truncated at 30000 chars)") {
		t.Fatalf("unexpected truncated length: %d", len(got))
	}
	if !strings.HasPrefix(got, strings.Repeat("a", 30000)) {
		t.Fatal("truncation must keep the first 30000 chars")
	}
	if !strings.HasSuffix(got, "...(tool result truncated at 30000 chars)") {
		t.Fatalf("missing truncation note: %q", got[len(got)-60:])
	}
}

/* ─── agent tool registration ─── */

func TestNewReviewAgentRegistersScipTools(t *testing.T) {
	agent, err := NewReviewAgent(nil, &DiffResult{}, nil, "en", "", "repo", "main", "abc1234", 10, false, fakeWorkerIndex())
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, td := range agent.tools {
		names[td.Function.Name] = true
	}
	for _, want := range []string{"read_file", "read_diff", "search_code", "read_function", "find_references", "find_definition", "symbol_info"} {
		if !names[want] {
			t.Errorf("expected tool %q to be registered, got %v", want, names)
		}
	}
}

func TestNewReviewAgentWithoutIndexSkipsScipTools(t *testing.T) {
	agent, err := NewReviewAgent(nil, &DiffResult{}, nil, "en", "", "repo", "main", "abc1234", 10, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, td := range agent.tools {
		if td.Function.Name == "find_references" || td.Function.Name == "find_definition" || td.Function.Name == "symbol_info" {
			t.Errorf("semantic tool %q must not be registered without an index", td.Function.Name)
		}
	}
}

/* ─── sanity: output is stable across runs ─── */

func TestAgentSemanticToolsConsistency(t *testing.T) {
	a := &ReviewAgent{indexSet: fakeWorkerIndex()}
	first := a.toolFindReferences("main.go", 2)
	second := a.toolFindReferences("main.go", 2)
	if first != second {
		t.Fatalf("semantic tool output must be deterministic:\n%q\n%q", first, second)
	}
}

func TestNoChangesErrStaged(t *testing.T) {
	// The staged branch is pure (no git subprocess) and deterministic.
	err := noChangesErr([]string{"--staged"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "git add") {
		t.Fatalf("expected a staging hint, got %q", err)
	}
}

func TestReadFileCached(t *testing.T) {
	writeScipTestFile(t, "_scip_cache.go", "package git\n\nfunc Alpha() {}\nfunc Beta() {}\n")
	a := &ReviewAgent{}

	first := a.readFileCached("_scip_cache.go", 0, 0)
	if !strings.Contains(first, "func Alpha() {}") || strings.HasPrefix(first, "note:") {
		t.Fatalf("first read should return full content, got %q", first)
	}

	// Same whole-file read → cached note, no content.
	second := a.readFileCached("_scip_cache.go", 0, 0)
	if !strings.HasPrefix(second, "note:") {
		t.Fatalf("re-read should return a cache note, got %q", second)
	}
	if strings.Contains(second, "func Alpha") {
		t.Fatalf("cache note must not repeat content, got %q", second)
	}

	// A range fully inside the already-read span is also served from cache.
	third := a.readFileCached("_scip_cache.go", 2, 3)
	if !strings.HasPrefix(third, "note:") {
		t.Fatalf("sub-range of read file should be cached, got %q", third)
	}

	// A different file is not cached.
	writeScipTestFile(t, "_scip_cache2.go", "package git\n\nfunc Gamma() {}\n")
	other := a.readFileCached("_scip_cache2.go", 0, 0)
	if !strings.Contains(other, "func Gamma") || strings.HasPrefix(other, "note:") {
		t.Fatalf("different file should be read fresh, got %q", other)
	}

	// Missing file errors and is not cached.
	if got := a.readFileCached("_scip_missing.go", 0, 0); !strings.Contains(got, "error reading") {
		t.Fatalf("expected read error, got %q", got)
	}
}
