package git

import (
	"testing"

	scipb "github.com/scip-code/scip/bindings/go/scip"
	"github.com/yusiwen/myUtilities/core/scip"
)

func fakeIndexSet() *scip.IndexSet {
	raw := &scipb.Index{
		Documents: []*scipb.Document{
			{
				RelativePath: "lib.go",
				Occurrences: []*scipb.Occurrence{
					{TypedRange: &scipb.Occurrence_SingleLineRange{SingleLineRange: &scipb.SingleLineRange{Line: 0, StartCharacter: 5, EndCharacter: 8}}, Symbol: "repo Worker", SymbolRoles: int32(scipb.SymbolRole_Definition)},
					{TypedRange: &scipb.Occurrence_SingleLineRange{SingleLineRange: &scipb.SingleLineRange{Line: 4, StartCharacter: 2, EndCharacter: 5}}, Symbol: "repo Worker", SymbolRoles: int32(scipb.SymbolRole_ReadAccess)},
				},
				Symbols: []*scipb.SymbolInformation{
					{Symbol: "repo Worker", DisplayName: "Worker", Kind: scipb.SymbolInformation_Function,
						SignatureDocumentation: &scipb.Signature{Text: "func Worker()"}},
				},
			},
			{
				RelativePath: "main.go",
				Occurrences: []*scipb.Occurrence{
					{TypedRange: &scipb.Occurrence_SingleLineRange{SingleLineRange: &scipb.SingleLineRange{Line: 1, StartCharacter: 0, EndCharacter: 3}}, Symbol: "repo Worker", SymbolRoles: int32(scipb.SymbolRole_ReadAccess)},
				},
			},
		},
	}
	set := scip.NewIndexSet("/repo")
	set.Add("go", scip.NewIndex(raw))
	return set
}

func TestAgentSemanticTools(t *testing.T) {
	a := &ReviewAgent{indexSet: fakeIndexSet()}

	refs := a.toolFindReferences("main.go", 2)
	if refs == "" {
		t.Fatal("expected references output")
	}
	// main.go:2 usage + lib.go:1 def + lib.go:5 usage
	if len(refs) < 20 {
		t.Fatalf("unexpected references output: %q", refs)
	}

	defs := a.toolFindDefinition("main.go", 2)
	if defs == "" {
		t.Fatal("expected definitions output")
	}

	info := a.toolSymbolInfo("lib.go", 1)
	if info == "" {
		t.Fatal("expected symbol info output")
	}
}

func TestAgentSemanticToolsNoIndex(t *testing.T) {
	a := &ReviewAgent{} // indexSet nil
	if got := a.toolFindReferences("x.go", 1); got == "" {
		t.Fatal("expected graceful error for missing index")
	}
	if got := a.toolFindDefinition("x.go", 1); got == "" {
		t.Fatal("expected graceful error for missing index")
	}
}
