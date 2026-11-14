package diff

import (
	"fmt"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/yusiwen/myUtilities/internal/core/patch"
)

const tuiPatch = `diff --git a/alpha.txt b/alpha.txt
index 1111111..2222222 100644
--- a/alpha.txt
+++ b/alpha.txt
@@ -1,3 +1,3 @@
 one
-old alpha
+new alpha
 three
diff --git a/beta.txt b/beta.txt
index 3333333..4444444 100644
--- a/beta.txt
+++ b/beta.txt
@@ -1,4 +1,5 @@
 one
+beta-added
 two
 three
 four
`

func parseForTUI(t *testing.T, text string) tuiModel {
	t.Helper()
	files, err := patch.Parse([]byte(text))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no files parsed")
	}
	return newTUIModel(files, patch.Options{LineNumbers: true})
}

func sendKey(t *testing.T, m tuiModel, msg tea.Msg) (tuiModel, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(msg)
	model, ok := updated.(tuiModel)
	if !ok {
		t.Fatalf("Update returned %T", updated)
	}
	return model, cmd
}

func runeKey(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestTUILayoutFitsTerminal(t *testing.T) {
	m := parseForTUI(t, tuiPatch)
	m, _ = sendKey(t, m, tea.WindowSizeMsg{Width: 100, Height: 20})

	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != 20 {
		t.Fatalf("view has %d lines, want exactly 20", len(lines))
	}
	for i, line := range lines {
		if w := ansi.StringWidth(line); w > 100 {
			t.Errorf("line %d is %d cells wide, want <= 100: %q", i, w, ansi.Strip(line))
		}
	}
	if !strings.Contains(view, " │ ") {
		t.Errorf("view has no pane separator:\n%s", ansi.Strip(view))
	}

	plain := ansi.Strip(view)
	for _, want := range []string{
		"FILES (2)",
		"alpha.txt",
		"beta.txt",
		"focus: files",
		"new alpha", // the selected file's diff is on the right
		"q quit",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("view lacks %q:\n%s", want, plain)
		}
	}
}

func TestTUIFocusSwitching(t *testing.T) {
	m := parseForTUI(t, tuiPatch)
	m, _ = sendKey(t, m, tea.WindowSizeMsg{Width: 100, Height: 20})

	if m.focus != paneFiles {
		t.Fatalf("initial focus = %v, want files", m.focus)
	}
	if !strings.Contains(ansi.Strip(m.View()), "focus: files") {
		t.Error("header does not report the files focus")
	}

	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != paneDiff || !strings.Contains(ansi.Strip(m.View()), "focus: diff") {
		t.Errorf("tab did not move focus to the diff: focus=%v", m.focus)
	}

	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.focus != paneFiles {
		t.Errorf("shift+tab did not move focus back: focus=%v", m.focus)
	}

	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if m.focus != paneDiff {
		t.Error("right arrow did not focus the diff")
	}
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyLeft})
	if m.focus != paneFiles {
		t.Error("left arrow did not focus the file list")
	}

	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.focus != paneDiff {
		t.Error("enter did not jump from the list to the diff")
	}
}

func TestTUIListNavigation(t *testing.T) {
	m := parseForTUI(t, tuiPatch)
	m, _ = sendKey(t, m, tea.WindowSizeMsg{Width: 100, Height: 20})

	m, _ = sendKey(t, m, runeKey("j"))
	if m.sel != 1 {
		t.Fatalf("sel after j = %d, want 1", m.sel)
	}
	if plain := ansi.Strip(m.View()); !strings.Contains(plain, "beta-added") {
		t.Errorf("right pane did not switch to beta.txt:\n%s", plain)
	}

	m, _ = sendKey(t, m, runeKey("j"))
	if m.sel != 1 {
		t.Errorf("j past the last file moved to %d", m.sel)
	}

	m, _ = sendKey(t, m, runeKey("k"))
	if m.sel != 0 {
		t.Errorf("sel after k = %d, want 0", m.sel)
	}
	if plain := ansi.Strip(m.View()); !strings.Contains(plain, "new alpha") {
		t.Errorf("right pane did not switch back to alpha.txt:\n%s", plain)
	}

	m, _ = sendKey(t, m, runeKey("G"))
	if m.sel != 1 {
		t.Errorf("G selected %d, want the last file", m.sel)
	}
	m, _ = sendKey(t, m, runeKey("g"))
	if m.sel != 0 {
		t.Errorf("g selected %d, want the first file", m.sel)
	}

	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.sel != 1 {
		t.Errorf("down arrow selected %d, want 1", m.sel)
	}
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if m.sel != 0 {
		t.Errorf("up arrow selected %d, want 0", m.sel)
	}
}

func longPatch() string {
	var b strings.Builder
	b.WriteString("diff --git a/long.txt b/long.txt\n--- a/long.txt\n+++ b/long.txt\n@@ -1,40 +1,41 @@\n")
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(&b, " line %02d\n", i)
	}
	b.WriteString("+inserted line\n")
	for i := 21; i <= 40; i++ {
		fmt.Fprintf(&b, " line %02d\n", i)
	}
	return b.String()
}

func TestTUIDiffScrolling(t *testing.T) {
	m := parseForTUI(t, longPatch())
	m, _ = sendKey(t, m, tea.WindowSizeMsg{Width: 80, Height: 12})
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyTab}) // focus the diff

	maxScroll := m.maxScroll()
	if maxScroll <= 0 {
		t.Fatalf("maxScroll = %d, want a scrollable file", maxScroll)
	}

	m, _ = sendKey(t, m, runeKey("j"))
	if m.scroll != 1 {
		t.Fatalf("scroll after j = %d, want 1", m.scroll)
	}
	m, _ = sendKey(t, m, runeKey("k"))
	if m.scroll != 0 {
		t.Errorf("scroll after k = %d, want 0", m.scroll)
	}

	m, _ = sendKey(t, m, runeKey("G"))
	if m.scroll != maxScroll {
		t.Errorf("G scrolled to %d, want %d", m.scroll, maxScroll)
	}
	if plain := ansi.Strip(m.View()); !strings.Contains(plain, "line 40") {
		t.Errorf("bottom of the diff is not visible after G:\n%s", plain)
	}

	m, _ = sendKey(t, m, runeKey("g"))
	if m.scroll != 0 {
		t.Errorf("g scrolled to %d, want 0", m.scroll)
	}
	if plain := ansi.Strip(m.View()); !strings.Contains(plain, "line 01") {
		t.Errorf("top of the diff is not visible after g:\n%s", plain)
	}

	page := m.contentHeight()
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlD})
	if m.scroll != page {
		t.Errorf("ctrl+d scrolled to %d, want %d", m.scroll, page)
	}
	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyCtrlU})
	if m.scroll != 0 {
		t.Errorf("ctrl+u scrolled to %d, want 0", m.scroll)
	}

	m, _ = sendKey(t, m, tea.KeyMsg{Type: tea.KeyPgDown})
	if m.scroll != page {
		t.Errorf("pgdown scrolled to %d, want %d", m.scroll, page)
	}
}

func TestTUIQuit(t *testing.T) {
	m := parseForTUI(t, tuiPatch)
	m, _ = sendKey(t, m, tea.WindowSizeMsg{Width: 100, Height: 20})

	for _, key := range []tea.KeyMsg{
		runeKey("q"),
		{Type: tea.KeyEsc},
		{Type: tea.KeyCtrlC},
	} {
		_, cmd := sendKey(t, m, key)
		if cmd == nil {
			t.Fatalf("%q returned no command", key.String())
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("%q returned %T, want tea.QuitMsg", key.String(), cmd())
		}
	}
}

func TestTUISmallTerminal(t *testing.T) {
	m := parseForTUI(t, tuiPatch)
	m, _ = sendKey(t, m, tea.WindowSizeMsg{Width: 20, Height: 4})
	view := m.View()
	if !strings.Contains(ansi.Strip(view), "terminal too small") {
		t.Errorf("small terminal is not reported:\n%s", ansi.Strip(view))
	}
	if w := ansi.StringWidth(view); w > 20 {
		t.Errorf("view is %d cells wide, want <= 20", w)
	}
}

func TestTUIStatusLetters(t *testing.T) {
	tests := []struct {
		name  string
		patch string
		want  byte
	}{
		{"modified", "diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-a\n+b\n", 'M'},
		{"added", "diff --git a/x b/x\nnew file mode 100644\n--- /dev/null\n+++ b/x\n@@ -0,0 +1 @@\n+a\n", 'A'},
		{"deleted", "diff --git a/x b/x\ndeleted file mode 100644\n--- a/x\n+++ /dev/null\n@@ -1 +0,0 @@\n-a\n", 'D'},
		{"renamed", "diff --git a/old b/new\nsimilarity index 90%\nrename from old\nrename to new\n", 'R'},
		{"binary", "diff --git a/x b/x\nBinary files a/x and b/x differ\n", 'B'},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, err := patch.Parse([]byte(tt.patch))
			if err != nil || len(files) == 0 {
				t.Fatalf("parse: %v", err)
			}
			if got := statsFor(files[0]).status; got != tt.want {
				t.Errorf("status = %c, want %c", got, tt.want)
			}
		})
	}
}

func TestTUIAddDeleteCounts(t *testing.T) {
	m := parseForTUI(t, tuiPatch)
	if m.stats[0].adds != 1 || m.stats[0].dels != 1 {
		t.Errorf("alpha stats = +%d -%d, want +1 -1", m.stats[0].adds, m.stats[0].dels)
	}
	if m.stats[1].adds != 1 || m.stats[1].dels != 0 {
		t.Errorf("beta stats = +%d -%d, want +1 -0", m.stats[1].adds, m.stats[1].dels)
	}
}

func TestTUIEmptyModel(t *testing.T) {
	m := newTUIModel(nil, patch.Options{LineNumbers: true})
	m, _ = sendKey(t, m, tea.WindowSizeMsg{Width: 80, Height: 10})
	plain := ansi.Strip(m.View())
	for _, want := range []string{"FILES (0)", "(no file)"} {
		if !strings.Contains(plain, want) {
			t.Errorf("empty model lacks %q:\n%s", want, plain)
		}
	}
}

func TestTUIRequiresTerminal(t *testing.T) {
	files, err := patch.Parse([]byte(tuiPatch))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	out, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatalf("temp: %v", err)
	}
	defer func() { _ = out.Close() }()

	err = runTUI(files, patch.Options{LineNumbers: true}, strings.NewReader(""), out)
	if err == nil || !strings.Contains(err.Error(), "requires a terminal") {
		t.Fatalf("err = %v, want the terminal requirement", err)
	}
}

func TestTUIFlagParsing(t *testing.T) {
	o, err := parsePatch(t, "--tui")
	if err != nil {
		t.Fatalf("parse --tui: %v", err)
	}
	if !o.TUI {
		t.Error("--tui left TUI disabled")
	}

	o, err = parsePatch(t)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if o.TUI {
		t.Error("TUI should default to false")
	}
}

func TestTUILineNumbersToggle(t *testing.T) {
	files, err := patch.Parse([]byte(tuiPatch))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	m := newTUIModel(files, patch.Options{LineNumbers: false})
	m, _ = sendKey(t, m, tea.WindowSizeMsg{Width: 100, Height: 20})
	plain := ansi.Strip(m.View())
	if !strings.Contains(plain, "-old alpha") {
		t.Errorf("diff content missing:\n%s", plain)
	}
	if strings.Contains(plain, "2   | -old alpha") {
		t.Errorf("gutter shown even though LineNumbers=false:\n%s", plain)
	}
}
