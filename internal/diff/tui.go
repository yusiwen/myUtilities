package diff

import (
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/yusiwen/myUtilities/internal/core/patch"
	"golang.org/x/term"
)

// pane is which side of the TUI has focus.
type pane int

const (
	paneFiles pane = iota
	paneDiff
)

// fileStat is the one-letter status and line counts shown next to a file.
type fileStat struct {
	status     byte
	adds, dels int
}

// tuiModel is the interactive viewer for a parsed patch.
type tuiModel struct {
	files []patch.File
	lines [][]patch.DisplayLine
	stats []fileStat

	width, height int
	sel, scroll   int
	focus         pane
}

var (
	tuiHeaderStyle  = lipgloss.NewStyle().Bold(true)
	tuiFooterStyle  = lipgloss.NewStyle().Faint(true)
	tuiFocusTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	tuiTitleDim     = lipgloss.NewStyle().Faint(true)
	tuiSelected     = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("231")).Bold(true)
	tuiSelectedIdle = lipgloss.NewStyle().Background(lipgloss.Color("238")).Foreground(lipgloss.Color("252"))
	tuiAddStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("34"))
	tuiDelStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("160"))
	tuiHunkStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("37")).Bold(true)
	tuiPathOldStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("160"))
	tuiPathNewStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("34"))
	tuiMetaStyle    = lipgloss.NewStyle().Faint(true)
	tuiBinaryStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("178"))
	tuiNoteStyle    = lipgloss.NewStyle().Faint(true)
)

func newTUIModel(files []patch.File, opts patch.Options) tuiModel {
	m := tuiModel{files: files, focus: paneFiles}
	for i := range files {
		m.lines = append(m.lines, patch.Lines(files[i], opts))
		m.stats = append(m.stats, statsFor(files[i]))
	}
	return m
}

// Init implements tea.Model.
func (m tuiModel) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampScroll()
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "tab", "shift+tab":
			m.toggleFocus()
		case "left":
			m.focus = paneFiles
		case "right":
			m.focus = paneDiff
		case "enter":
			m.focus = paneDiff
		case "j", "down":
			m.move(1)
		case "k", "up":
			m.move(-1)
		case "g", "home":
			m.jumpTop()
		case "G", "end":
			m.jumpBottom()
		case "ctrl+d", "pgdown":
			m.page(1)
		case "ctrl+u", "pgup":
			m.page(-1)
		}
	}
	return m, nil
}

func (m *tuiModel) toggleFocus() {
	if m.focus == paneFiles {
		m.focus = paneDiff
	} else {
		m.focus = paneFiles
	}
}

func (m *tuiModel) move(delta int) {
	if m.focus == paneFiles {
		if len(m.files) == 0 {
			return
		}
		m.sel = clampInt(m.sel+delta, 0, len(m.files)-1)
		m.scroll = 0
		return
	}
	m.scroll = clampInt(m.scroll+delta, 0, m.maxScroll())
}

func (m *tuiModel) page(delta int) {
	step := m.contentHeight()
	if step < 1 {
		step = 1
	}
	if m.focus == paneFiles {
		if len(m.files) == 0 {
			return
		}
		m.sel = clampInt(m.sel+delta*step, 0, len(m.files)-1)
		m.scroll = 0
		return
	}
	m.scroll = clampInt(m.scroll+delta*step, 0, m.maxScroll())
}

func (m *tuiModel) jumpTop() {
	if m.focus == paneFiles {
		m.sel = 0
		m.scroll = 0
		return
	}
	m.scroll = 0
}

func (m *tuiModel) jumpBottom() {
	if m.focus == paneFiles {
		m.sel = max(0, len(m.files)-1)
		m.scroll = 0
		return
	}
	m.scroll = m.maxScroll()
}

func (m *tuiModel) bodyHeight() int {
	if m.height-2 < 1 {
		return 1
	}
	return m.height - 2
}

func (m *tuiModel) contentHeight() int {
	if m.bodyHeight()-1 < 1 {
		return 1
	}
	return m.bodyHeight() - 1
}

func (m *tuiModel) maxScroll() int {
	if m.sel < 0 || m.sel >= len(m.lines) {
		return 0
	}
	return max(0, len(m.lines[m.sel])-m.contentHeight())
}

func (m *tuiModel) clampScroll() {
	m.scroll = clampInt(m.scroll, 0, m.maxScroll())
}

// View implements tea.Model.
func (m tuiModel) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	if m.width < 30 || m.height < 5 {
		return truncate("terminal too small: resize to at least 30x5", m.width)
	}

	bodyH := m.bodyHeight()
	leftW := m.width / 3
	if leftW < 20 {
		leftW = 20
	}
	if leftW > 44 {
		leftW = 44
	}
	if leftW > m.width-25 {
		leftW = m.width - 25
	}
	if leftW < 10 {
		leftW = 10
	}
	rightW := m.width - leftW - 3
	if rightW < 1 {
		rightW = 1
	}

	left := m.filesColumn(leftW, bodyH)
	right := m.diffColumn(rightW, bodyH)

	lines := make([]string, 0, m.height)
	lines = append(lines, padTo(truncate(m.headerLine(), m.width), m.width))
	for i := 0; i < bodyH; i++ {
		lines = append(lines, left[i]+" │ "+right[i])
	}
	lines = append(lines, padTo(truncate(m.footerLine(), m.width), m.width))
	return strings.Join(lines, "\n")
}

func (m tuiModel) headerLine() string {
	focus := "files"
	if m.focus == paneDiff {
		focus = "diff"
	}
	return tuiHeaderStyle.Render(fmt.Sprintf(" mu diff patch  |  %d file(s)  |  focus: %s ", len(m.files), focus))
}

func (m tuiModel) footerLine() string {
	return tuiFooterStyle.Render(" tab/arrows switch focus | j/k move | g/G top/bottom | ctrl+d/u page | q quit ")
}

func (m tuiModel) filesColumn(width, height int) []string {
	title := tuiTitleDim
	if m.focus == paneFiles {
		title = tuiFocusTitle
	}
	out := []string{padTo(title.Render(truncate(fmt.Sprintf(" FILES (%d) ", len(m.files)), width)), width)}

	rows := height - 1
	if rows < 0 {
		rows = 0
	}
	start := 0
	if rows > 0 && m.sel >= rows {
		start = m.sel - rows + 1
	}
	for i := 0; i < rows; i++ {
		idx := start + i
		if idx >= len(m.files) {
			out = append(out, strings.Repeat(" ", width))
			continue
		}
		path := truncate(displayPath(m.files[idx]), width-2)
		plain := fmt.Sprintf("%c %s", m.stats[idx].status, path)
		plain = padTo(plain, width)
		switch {
		case idx == m.sel && m.focus == paneFiles:
			out = append(out, tuiSelected.Render(plain))
		case idx == m.sel:
			out = append(out, tuiSelectedIdle.Render(plain))
		default:
			status := statusStyle(m.stats[idx].status).Render(string(m.stats[idx].status))
			out = append(out, padTo(status+" "+path, width))
		}
	}
	return out
}

func (m tuiModel) diffColumn(width, height int) []string {
	title := tuiTitleDim
	if m.focus == paneDiff {
		title = tuiFocusTitle
	}
	titleText := " (no file) "
	if m.sel >= 0 && m.sel < len(m.files) {
		st := m.stats[m.sel]
		titleText = fmt.Sprintf(" %s  +%d -%d ", displayPath(m.files[m.sel]), st.adds, st.dels)
	}
	out := []string{padTo(title.Render(truncate(titleText, width)), width)}

	rows := height - 1
	if rows < 0 {
		rows = 0
	}
	var content []patch.DisplayLine
	if m.sel >= 0 && m.sel < len(m.lines) {
		content = m.lines[m.sel]
	}
	start := clampInt(m.scroll, 0, max(0, len(content)-rows))
	for i := 0; i < rows; i++ {
		idx := start + i
		if idx >= len(content) {
			out = append(out, strings.Repeat(" ", width))
			continue
		}
		line := content[idx]
		out = append(out, padTo(tuiLineStyle(line.Kind).Render(truncate(line.Gutter+line.Text, width)), width))
	}
	return out
}

func statsFor(f patch.File) fileStat {
	st := fileStat{status: 'M'}
	header := strings.Join(f.Header, "\n")
	switch {
	case f.Binary:
		st.status = 'B'
	case strings.Contains(header, "new file mode"):
		st.status = 'A'
	case strings.Contains(header, "deleted file mode"):
		st.status = 'D'
	case strings.Contains(header, "rename from"):
		st.status = 'R'
	case strings.Contains(header, "copy from"):
		st.status = 'C'
	}
	for _, h := range f.Hunks {
		for _, line := range h.Lines {
			switch line.Kind {
			case patch.Add:
				st.adds++
			case patch.Delete:
				st.dels++
			}
		}
	}
	return st
}

func displayPath(f patch.File) string {
	if f.NewPath != "" && f.NewPath != "/dev/null" {
		return f.NewPath
	}
	if f.OldPath != "" && f.OldPath != "/dev/null" {
		return f.OldPath
	}
	return "(unknown)"
}

func tuiLineStyle(kind patch.LineKind) lipgloss.Style {
	switch kind {
	case patch.LineHeader:
		return tuiHeaderStyle
	case patch.LineMeta:
		return tuiMetaStyle
	case patch.LineOldPath:
		return tuiPathOldStyle
	case patch.LineNewPath:
		return tuiPathNewStyle
	case patch.LineHunk:
		return tuiHunkStyle
	case patch.LineContext:
		return tuiMetaStyle
	case patch.LineAdd:
		return tuiAddStyle
	case patch.LineDelete:
		return tuiDelStyle
	case patch.LineNoNewline:
		return tuiNoteStyle
	case patch.LineBinary:
		return tuiBinaryStyle
	default:
		return lipgloss.NewStyle()
	}
}

func statusStyle(status byte) lipgloss.Style {
	switch status {
	case 'A':
		return lipgloss.NewStyle().Foreground(lipgloss.Color("34")).Bold(true)
	case 'D':
		return lipgloss.NewStyle().Foreground(lipgloss.Color("160")).Bold(true)
	case 'R', 'C':
		return lipgloss.NewStyle().Foreground(lipgloss.Color("37")).Bold(true)
	case 'B':
		return lipgloss.NewStyle().Foreground(lipgloss.Color("178")).Bold(true)
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("178")).Bold(true)
	}
}

// runTUI starts the interactive viewer. The patch is already parsed, so stdin
// can be used for keys when the patch arrived through a pipe; in that case the
// controlling terminal is opened instead. stdin and stdout are injected so the
// terminal checks are testable.
func runTUI(files []patch.File, opts patch.Options, stdin io.Reader, stdout *os.File) error {
	if !term.IsTerminal(int(stdout.Fd())) {
		return fmt.Errorf("--tui requires a terminal on stdout; omit --tui to write the rendered patch")
	}

	input := stdin
	var tty *os.File
	if f, ok := stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		input = f
	} else {
		f, err := openTTY()
		if err != nil {
			return fmt.Errorf("--tui needs a terminal for keyboard input: %w", err)
		}
		tty = f
		defer func() { _ = tty.Close() }()
		input = tty
	}

	program := tea.NewProgram(newTUIModel(files, opts),
		tea.WithAltScreen(),
		tea.WithInput(input),
		tea.WithOutput(stdout),
	)
	_, err := program.Run()
	return err
}

func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= width {
		return s
	}
	return ansi.Truncate(s, width, "…")
}

func padTo(s string, width int) string {
	if d := width - ansi.StringWidth(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
