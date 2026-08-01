package scip

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scip-code/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"
)

func fakeIndex() *Index {
	raw := &scip.Index{
		Metadata: &scip.Metadata{ProjectRoot: "/repo"},
		Documents: []*scip.Document{
			{
				RelativePath: "foo.go",
				Occurrences: []*scip.Occurrence{
					// line 1: func Foo(a int) { ... }  (0-based line 0)
					{TypedRange: &scip.Occurrence_SingleLineRange{SingleLineRange: &scip.SingleLineRange{Line: 0, StartCharacter: 5, EndCharacter: 8}}, Symbol: "repo Foo", SymbolRoles: int32(scip.SymbolRole_Definition)},
					// line 3: 0-based line 2, call site Foo(a)
					{TypedRange: &scip.Occurrence_SingleLineRange{SingleLineRange: &scip.SingleLineRange{Line: 2, StartCharacter: 4, EndCharacter: 7}}, Symbol: "repo Foo", SymbolRoles: int32(scip.SymbolRole_ReadAccess)},
					// line 5: local variable usage
					{TypedRange: &scip.Occurrence_SingleLineRange{SingleLineRange: &scip.SingleLineRange{Line: 4, StartCharacter: 0, EndCharacter: 5}}, Symbol: "local 1", SymbolRoles: int32(scip.SymbolRole_Definition)},
				},
				Symbols: []*scip.SymbolInformation{
					{Symbol: "repo Foo", DisplayName: "Foo", Kind: scip.SymbolInformation_Function,
						SignatureDocumentation: &scip.Signature{Text: "func Foo(a int)"}},
				},
			},
			{
				RelativePath: "bar.go",
				Occurrences: []*scip.Occurrence{
					// line 2: 0-based line 1, another call site of Foo
					{TypedRange: &scip.Occurrence_SingleLineRange{SingleLineRange: &scip.SingleLineRange{Line: 1, StartCharacter: 8, EndCharacter: 11}}, Symbol: "repo Foo", SymbolRoles: int32(scip.SymbolRole_ReadAccess)},
					// a definition of an unrelated local symbol that shares a
					// string key with foo.go's local — must NOT leak across files
					{TypedRange: &scip.Occurrence_SingleLineRange{SingleLineRange: &scip.SingleLineRange{Line: 0, StartCharacter: 0, EndCharacter: 5}}, Symbol: "local 1", SymbolRoles: int32(scip.SymbolRole_Definition)},
				},
			},
		},
	}
	return NewIndex(raw)
}

func TestFindDefinition(t *testing.T) {
	ix := fakeIndex()

	defs := ix.FindDefinition("foo.go", 1)
	if len(defs) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(defs))
	}
	d := defs[0]
	if d.Path != "foo.go" || d.Line != 1 || d.Character != 5 {
		t.Fatalf("unexpected definition: %+v", d)
	}
	if !d.IsDef {
		t.Fatalf("expected definition role, got %+v", d)
	}
}

func TestFindReferencesCrossFile(t *testing.T) {
	ix := fakeIndex()

	refs := ix.FindReferences("foo.go", 1)
	// definition (foo.go:1) + usage (foo.go:3) + usage (bar.go:2)
	if len(refs) != 3 {
		t.Fatalf("expected 3 references, got %d: %+v", len(refs), refs)
	}
	paths := map[string]bool{}
	for _, r := range refs {
		paths[r.Path] = true
	}
	if !paths["bar.go"] {
		t.Fatalf("expected cross-file reference in bar.go: %+v", refs)
	}
}

func TestLocalSymbolsScopedToFile(t *testing.T) {
	ix := fakeIndex()

	// foo.go:5 is a definition of "local 1"; bar.go also has a "local 1"
	// definition on line 1. References must not leak across files.
	refs := ix.FindReferences("foo.go", 5)
	if len(refs) != 1 {
		t.Fatalf("expected only the local definition in foo.go, got %d: %+v", len(refs), refs)
	}
	if refs[0].Path != "foo.go" || refs[0].Line != 5 {
		t.Fatalf("unexpected local reference: %+v", refs[0])
	}
}

func TestOffByOneLineMapping(t *testing.T) {
	ix := fakeIndex()

	// Query 1-based line 3 (0-based 2): the call site of Foo in foo.go.
	defs := ix.FindDefinition("foo.go", 3)
	if len(defs) == 0 || defs[0].Line != 1 {
		t.Fatalf("off-by-one: expected definition on line 1, got %+v", defs)
	}

	// line 2 has no occurrences → empty
	if got := ix.FindDefinition("foo.go", 2); len(got) != 0 {
		t.Fatalf("expected no symbols on line 2, got %+v", got)
	}
}

func TestSymbolInfoAt(t *testing.T) {
	ix := fakeIndex()
	info := ix.SymbolInfoAt("foo.go", 1)
	if info == nil {
		t.Fatal("expected symbol info")
	}
	if info.DisplayName != "Foo" || info.Kind != "Function" {
		t.Fatalf("unexpected symbol info: %+v", info)
	}
	if info.Signature != "func Foo(a int)" {
		t.Fatalf("unexpected signature: %q", info.Signature)
	}
}

func TestSymbolsInRange(t *testing.T) {
	ix := fakeIndex()
	got := ix.SymbolsInRange("foo.go", 1, 3)
	if len(got) != 1 {
		t.Fatalf("expected 1 symbol in range, got %d: %+v", len(got), got)
	}
}

func TestLoadFromDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.scip")
	raw := fakeIndex()
	// rebuild a raw index to marshal
	rawProto := &scip.Index{
		Metadata: &scip.Metadata{ProjectRoot: "/repo"},
		Documents: []*scip.Document{
			{
				RelativePath: "a.go",
				Occurrences: []*scip.Occurrence{
					{TypedRange: &scip.Occurrence_SingleLineRange{SingleLineRange: &scip.SingleLineRange{Line: 0, StartCharacter: 0, EndCharacter: 3}}, Symbol: "repo A", SymbolRoles: int32(scip.SymbolRole_Definition)},
				},
			},
		},
	}
	data, err := proto.Marshal(rawProto)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	ix, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !ix.hasDoc("a.go") {
		t.Fatal("expected a.go in loaded index")
	}
	_ = raw
}

func TestIndexSetRouting(t *testing.T) {
	set := NewIndexSet("/repo")
	set.Add("go", fakeIndex())

	if _, ok := set.IndexFor("bar.go"); !ok {
		t.Fatal("expected bar.go to route to the go index")
	}
	if _, err := set.FindReferences("bar.go", 2); err != nil {
		t.Fatalf("expected cross-file lookup to succeed: %v", err)
	}
	if _, err := set.FindReferences("missing.go", 1); err == nil {
		t.Fatal("expected ErrNoIndex for missing file")
	}
}
