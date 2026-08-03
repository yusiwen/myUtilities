package scip

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/scip-code/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"
)

// Location identifies a symbol occurrence in the indexed workspace.
type Location struct {
	Path      string `json:"path"`
	Line      int    `json:"line"` // 1-based
	Character int    `json:"character"`
	Symbol    string `json:"symbol"`
	IsDef     bool   `json:"is_def,omitempty"`
}

func (l Location) String() string {
	return fmt.Sprintf("%s:%d:%d", l.Path, l.Line, l.Character)
}

// SymbolInfo holds hover-style information for a symbol.
type SymbolInfo struct {
	Symbol        string
	DisplayName   string
	Kind          string
	Signature     string
	Documentation string
}

// Index is an in-memory SCIP index with query helpers.
type Index struct {
	ProjectRoot string
	documents   map[string]*scip.Document
	bySymbol    map[string][]Location
	symbolInfo  map[string]*scip.SymbolInformation
}

// Load reads a SCIP protobuf index from disk.
func Load(path string) (*Index, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw scip.Index
	if err := proto.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse SCIP index %s: %w", path, err)
	}
	return NewIndex(&raw), nil
}

// NewIndex builds query structures from a raw SCIP index.
func NewIndex(raw *scip.Index) *Index {
	ix := &Index{
		documents:  make(map[string]*scip.Document),
		bySymbol:   make(map[string][]Location),
		symbolInfo: make(map[string]*scip.SymbolInformation),
	}
	if raw.Metadata != nil {
		ix.ProjectRoot = raw.Metadata.ProjectRoot
	}
	for _, doc := range raw.Documents {
		p := normalizePath(doc.RelativePath)
		ix.documents[p] = doc
		for _, occ := range doc.Occurrences {
			if occ.Symbol == "" {
				continue
			}
			sl, _, char := occLines(occ)
			loc := Location{
				Path:      p,
				Line:      sl + 1,
				Character: char,
				Symbol:    occ.Symbol,
				IsDef:     isDefinitionOcc(occ),
			}
			ix.bySymbol[occ.Symbol] = append(ix.bySymbol[occ.Symbol], loc)
		}
		for _, si := range doc.Symbols {
			ix.symbolInfo[si.Symbol] = si
		}
	}
	for _, si := range raw.ExternalSymbols {
		if _, ok := ix.symbolInfo[si.Symbol]; !ok {
			ix.symbolInfo[si.Symbol] = si
		}
	}
	return ix
}

// hasDoc reports whether the index contains the given relative path.
func (ix *Index) hasDoc(path string) bool {
	_, ok := ix.documents[normalizePath(path)]
	return ok
}

// symbolsAt returns the distinct symbols occurring on the given 1-based line.
func (ix *Index) symbolsAt(path string, line int) []string {
	doc, ok := ix.documents[normalizePath(path)]
	if !ok {
		return nil
	}
	target := line - 1
	seen := map[string]bool{}
	var out []string
	for _, occ := range doc.Occurrences {
		sl, el, _ := occLines(occ)
		if target >= sl && target <= el {
			if occ.Symbol != "" && !seen[occ.Symbol] {
				seen[occ.Symbol] = true
				out = append(out, occ.Symbol)
			}
		}
	}
	return out
}

// OccurrencesAt returns all locations of the given symbol.
func (ix *Index) occurrences(symbol string) []Location {
	return ix.bySymbol[symbol]
}

// FindDefinition returns the definition locations for the symbol at the
// given path/line (1-based). All distinct symbols on the line are resolved.
func (ix *Index) FindDefinition(path string, line int) []Location {
	return ix.locationsFor(path, line, true, "", maxLocations)
}

// FindReferences returns all locations (definition + usages) of the primary
// symbol on the given path/line. If no primary symbol exists, all symbols on
// the line are aggregated.
func (ix *Index) FindReferences(path string, line int) []Location {
	if sym := ix.primarySymbolAt(path, line); sym != "" {
		return ix.locationsFor(path, line, false, sym, maxLocations)
	}
	return ix.locationsFor(path, line, false, "", maxLocations)
}

const maxLocations = 50

// primarySymbolAt returns the most relevant symbol on the line: the first
// non-local symbol that has a definition occurrence at the line (e.g. the
// function being defined), falling back to the first non-local symbol.
func (ix *Index) primarySymbolAt(path string, line int) string {
	path = normalizePath(path)
	symbols := ix.symbolsAt(path, line)
	if len(symbols) == 0 {
		return ""
	}
	for _, sym := range symbols {
		if scip.IsLocalSymbol(sym) {
			continue
		}
		for _, loc := range ix.bySymbol[sym] {
			if loc.IsDef && loc.Path == path && loc.Line == line {
				return sym
			}
		}
	}
	for _, sym := range symbols {
		if !scip.IsLocalSymbol(sym) {
			return sym
		}
	}
	return symbols[0]
}

// locationsFor aggregates locations for symbols on a line. When symFilter is
// non-empty, only that symbol is considered; otherwise all symbols on the line
// are aggregated. Results are capped at max.
func (ix *Index) locationsFor(path string, line int, defsOnly bool, symFilter string, max int) []Location {
	symbols := ix.symbolsAt(path, line)
	if len(symbols) == 0 {
		return nil
	}
	queryPath := normalizePath(path)

	seen := map[string]bool{}
	var out []Location
	for _, sym := range symbols {
		if symFilter != "" && sym != symFilter {
			continue
		}
		for _, loc := range ix.bySymbol[sym] {
			// Local symbols have no stable identity across documents;
			// scope them to the querying file only.
			if scip.IsLocalSymbol(sym) && loc.Path != queryPath {
				continue
			}
			if defsOnly && !loc.IsDef {
				continue
			}
			key := loc.Path + ":" + fmt.Sprint(loc.Line) + ":" + sym
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, loc)
			if max > 0 && len(out) >= max {
				return out
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// EnclosingDefLine returns the 1-based line of the nearest non-local
// definition occurrence at or before line, preferring function-like symbols,
// or 0 if none. Useful to locate the enclosing function/method start.
func (ix *Index) EnclosingDefLine(path string, line int) int {
	doc, ok := ix.documents[normalizePath(path)]
	if !ok {
		return 0
	}
	target := line - 1
	best := -1
	bestFunc := -1
	for _, occ := range doc.Occurrences {
		if occ.Symbol == "" || !isDefinitionOcc(occ) || scip.IsLocalSymbol(occ.Symbol) {
			continue
		}
		sl, _, _ := occLines(occ)
		if sl > target {
			continue
		}
		if sl > best {
			best = sl
		}
		if isFunctionKind(ix.symbolInfoFor(occ.Symbol).Kind) && sl > bestFunc {
			bestFunc = sl
		}
	}
	if bestFunc >= 0 {
		return bestFunc + 1
	}
	return best + 1
}

func isFunctionKind(kind string) bool {
	// kind is already stripped of the "SymbolInformation_" prefix.
	return map[string]bool{
		"Function": true, "Method": true, "StaticMethod": true,
		"Constructor": true, "Accessor": true, "Getter": true, "Setter": true,
		"MethodAlias": true, "SingletonMethod": true, "ProtocolMethod": true,
		"PureVirtualMethod": true, "TraitMethod": true, "TypeClassMethod": true,
		"AbstractMethod": true,
	}[kind]
}

// SymbolInfoAt returns hover info for the primary symbol at path/line.
func (ix *Index) SymbolInfoAt(path string, line int) *SymbolInfo {
	sym := ix.primarySymbolAt(path, line)
	if sym == "" {
		return nil
	}
	return ix.symbolInfoFor(sym)
}

// SymbolsInRange returns hover info for all symbols occurring within the
// inclusive 1-based [startLine, endLine] range.
func (ix *Index) SymbolsInRange(path string, startLine, endLine int) []SymbolInfo {
	doc, ok := ix.documents[normalizePath(path)]
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	var out []SymbolInfo
	for _, occ := range doc.Occurrences {
		sl, el, _ := occLines(occ)
		if el < startLine-1 || sl > endLine-1 || occ.Symbol == "" || seen[occ.Symbol] {
			continue
		}
		seen[occ.Symbol] = true
		if si := ix.symbolInfoFor(occ.Symbol); si != nil {
			out = append(out, *si)
		}
	}
	return out
}

func (ix *Index) symbolInfoFor(symbol string) *SymbolInfo {
	si, ok := ix.symbolInfo[symbol]
	if !ok {
		return &SymbolInfo{Symbol: symbol}
	}
	info := &SymbolInfo{
		Symbol:      si.Symbol,
		DisplayName: si.DisplayName,
		Kind:        kindName(si.Kind),
	}
	if si.SignatureDocumentation != nil {
		info.Signature = si.SignatureDocumentation.Text
	}
	info.Documentation = strings.Join(si.Documentation, "\n")
	return info
}

func kindName(k scip.SymbolInformation_Kind) string {
	s := k.String()
	return strings.TrimPrefix(s, "SymbolInformation_")
}

func isDefinitionOcc(occ *scip.Occurrence) bool {
	role := scip.SymbolRole(occ.SymbolRoles)
	return role&scip.SymbolRole_Definition != 0 || role&scip.SymbolRole_ForwardDefinition != 0
}

// occLines returns the 0-based start/end line and start character of an
// occurrence, handling both typed ranges and the deprecated range field.
func occLines(occ *scip.Occurrence) (startLine, endLine int, startChar int) {
	if occ.TypedRange != nil {
		switch r := occ.TypedRange.(type) {
		case *scip.Occurrence_SingleLineRange:
			sr := r.SingleLineRange
			return int(sr.Line), int(sr.Line), int(sr.StartCharacter)
		case *scip.Occurrence_MultiLineRange:
			mr := r.MultiLineRange
			return int(mr.StartLine), int(mr.EndLine), int(mr.StartCharacter)
		}
	}
	if len(occ.Range) >= 3 {
		sl := int(occ.Range[0])
		el := sl
		if len(occ.Range) >= 4 {
			el = int(occ.Range[2])
		}
		return sl, el, int(occ.Range[1])
	}
	return 0, 0, 0
}

func normalizePath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(p, "./")
	return strings.TrimPrefix(p, "/")
}

/* ─── IndexSet ─── */

// IndexSet holds the per-language indexes for a workspace.
type IndexSet struct {
	repoRoot string
	indexes  map[string]*Index // lang → index
}

// NewIndexSet builds an empty IndexSet rooted at repoRoot.
func NewIndexSet(repoRoot string) *IndexSet {
	return &IndexSet{repoRoot: repoRoot, indexes: map[string]*Index{}}
}

// Add registers an index for lang.
func (s *IndexSet) Add(lang string, ix *Index) {
	s.indexes[lang] = ix
}

// Langs returns the loaded languages.
func (s *IndexSet) Langs() []string {
	var out []string
	for l := range s.indexes {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

// IndexFor returns the index that contains path, if any.
func (s *IndexSet) IndexFor(path string) (*Index, bool) {
	norm := normalizePath(path)
	// Prefer an index by language when the extension maps to one.
	if ext := filepath.Ext(norm); ext != "" {
		if ix, ok := s.indexes[langForExt(ext)]; ok && ix.hasDoc(norm) {
			return ix, true
		}
	}
	for _, ix := range s.indexes {
		if ix.hasDoc(norm) {
			return ix, true
		}
	}
	return nil, false
}

func langForExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".java":
		return "java"
	case ".c", ".cc", ".cpp", ".h", ".hpp":
		return "c"
	}
	return ""
}

// ErrNoIndex indicates a path is not covered by any loaded index.
type ErrNoIndex struct{ Path string }

func (e ErrNoIndex) Error() string {
	return fmt.Sprintf("no index available for %s", e.Path)
}

// FindDefinition resolves the symbol at path/line across the loaded indexes.
func (s *IndexSet) FindDefinition(path string, line int) ([]Location, error) {
	ix, ok := s.IndexFor(path)
	if !ok {
		return nil, ErrNoIndex{Path: path}
	}
	return ix.FindDefinition(path, line), nil
}

// FindReferences resolves all usages of the symbol at path/line.
func (s *IndexSet) FindReferences(path string, line int) ([]Location, error) {
	ix, ok := s.IndexFor(path)
	if !ok {
		return nil, ErrNoIndex{Path: path}
	}
	return ix.FindReferences(path, line), nil
}

// SymbolsInRange lists symbols within a line range of path.
func (s *IndexSet) SymbolsInRange(path string, startLine, endLine int) ([]SymbolInfo, error) {
	ix, ok := s.IndexFor(path)
	if !ok {
		return nil, ErrNoIndex{Path: path}
	}
	return ix.SymbolsInRange(path, startLine, endLine), nil
}

// SymbolInfoAt returns hover info for the symbol at path/line.
func (s *IndexSet) SymbolInfoAt(path string, line int) (*SymbolInfo, error) {
	ix, ok := s.IndexFor(path)
	if !ok {
		return nil, ErrNoIndex{Path: path}
	}
	info := ix.SymbolInfoAt(path, line)
	if info == nil {
		return nil, fmt.Errorf("no symbol found at %s:%d", path, line)
	}
	return info, nil
}
