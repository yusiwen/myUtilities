package diff

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
	"golang.org/x/term"
)

type Options struct {
	File  FileOptions  `cmd:"" name:"file" aliases:"f" help:"Compare two files."`
	Text  TextOptions  `cmd:"" name:"text" aliases:"t" help:"Compare two text strings."`
	Serve ServeOptions `cmd:"" name:"serve" help:"Start diff tool HTTP server."`
}

type FileOptions struct {
	File1 string `arg:"" name:"file1" help:"First file."`
	File2 string `arg:"" name:"file2" help:"Second file."`
}

type TextOptions struct {
	Text1 string `arg:"" name:"text1" help:"First text."`
	Text2 string `arg:"" optional:"" name:"text2" help:"Second text (omit for stdin)."`
}

type ServeOptions struct {
	Port int `help:"Port to listen on." default:"8088"`
}

func (o *FileOptions) Run() error {
	a, err := os.ReadFile(o.File1)
	if err != nil {
		return fmt.Errorf("read %s: %w", o.File1, err)
	}
	b, err := os.ReadFile(o.File2)
	if err != nil {
		return fmt.Errorf("read %s: %w", o.File2, err)
	}
	fmt.Print(diffText(string(a), string(b)))
	return nil
}

func (o *TextOptions) Run() error {
	text1 := o.Text1
	text2 := o.Text2
	if text2 == "" && !term.IsTerminal(int(os.Stdin.Fd())) {
		data, _ := io.ReadAll(os.Stdin)
		text2 = strings.TrimRight(string(data), "\n\r")
	}
	fmt.Print(diffText(text1, text2))
	return nil
}

func (o *ServeOptions) Run() error {
	mux := http.NewServeMux()
	mux.Handle("/", FrontendHandler())
	RegisterHandlers(mux)
	fmt.Printf("Diff tool server listening on :%d\n", o.Port)
	return http.ListenAndServe(fmt.Sprintf(":%d", o.Port), mux)
}

func RegisterHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/api/diff", handleDiff)
}

func handleDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Text1 string `json:"text1"`
		Text2 string `json:"text2"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	dmp := diffmatchpatch.New()
	rawDiffs := dmp.DiffMain(req.Text1, req.Text2, true)
	dmp.DiffCleanupSemantic(rawDiffs)

	type diffEntry struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	diffs := make([]diffEntry, 0, len(rawDiffs))
	for _, d := range rawDiffs {
		typ := "equal"
		switch d.Type {
		case diffmatchpatch.DiffInsert:
			typ = "insert"
		case diffmatchpatch.DiffDelete:
			typ = "delete"
		}
		diffs = append(diffs, diffEntry{Type: typ, Text: d.Text})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"diffs": diffs,
	})
}

func diffText(a, b string) string {
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(a, b, true)
	dmp.DiffCleanupSemantic(diffs)
	return dmp.DiffPrettyText(diffs)
}
