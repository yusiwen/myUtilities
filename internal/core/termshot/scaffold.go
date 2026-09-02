package termshot

import (
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"strings"

	"github.com/esimov/stackblur-go"
	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"github.com/gonvenience/bunt"
	"github.com/gonvenience/term"
	imgfont "golang.org/x/image/font"
)

const (
	red    = "#ED655A"
	yellow = "#E1C04C"
	green  = "#71BD47"
)

const (
	defaultFontSize = 12
	defaultFontDPI  = 144
)

// Embedded fonts. Hack is the primary monospace font; Noto Emoji (monochrome,
// Apache License 2.0, see fonts/NotoEmoji-LICENSE.txt) is a glyph fallback so
// that emoji Hack lacks (e.g. 😊) render as a single-color shape instead of a
// tofu box.
//
//go:embed fonts/Hack-Regular.ttf
var hackRegularFont []byte

//go:embed fonts/Hack-Bold.ttf
var hackBoldFont []byte

//go:embed fonts/Hack-Italic.ttf
var hackItalicFont []byte

//go:embed fonts/Hack-BoldItalic.ttf
var hackBoldItalicFont []byte

//go:embed fonts/NotoEmoji.ttf
var emojiFont []byte

// commandIndicator is the string used to indicate the command in the screenshot.
var commandIndicator = func() string {
	if val, ok := os.LookupEnv("TS_COMMAND_INDICATOR"); ok {
		return val
	}
	return "➜"
}()

// Scaffold holds the content and rendering settings for a terminal screenshot.
type Scaffold struct {
	content bunt.String

	factor float64

	columns int

	defaultForegroundColor color.Color

	clipCanvas bool

	drawDecorations bool
	drawShadow      bool

	shadowBaseColor string
	shadowRadius    uint8
	shadowOffsetX   float64
	shadowOffsetY   float64

	padding float64
	margin  float64

	regular     imgfont.Face
	bold        imgfont.Face
	italic      imgfont.Face
	boldItalic  imgfont.Face
	fallback    imgfont.Face
	hackFont    *truetype.Font
	lineSpacing float64
	tabSpaces   int
}

// NewImageCreator returns a Scaffold pre-configured with default look settings
// (Hack font, macOS-style window, shadow, margin) and a 2x scale factor.
func NewImageCreator() Scaffold {
	// The output is an image, not a terminal, so force bunt colors on to keep the
	// rendering (and the --show-cmd banner) colored regardless of the ambient TTY.
	bunt.SetColorSettings(bunt.ON, bunt.ON)

	f := 2.0

	fontFaceOptions := &truetype.Options{
		Size: f * defaultFontSize,
		DPI:  defaultFontDPI,
	}

	// Parse each font once and keep the parsed *truetype.Font so a character can
	// be checked for glyph coverage before choosing a face.
	regular, hackFont := newFace(hackRegularFont, fontFaceOptions)
	bold, _ := newFace(hackBoldFont, fontFaceOptions)
	italic, _ := newFace(hackItalicFont, fontFaceOptions)
	boldItalic, _ := newFace(hackBoldItalicFont, fontFaceOptions)
	fallback, _ := newFace(emojiFont, fontFaceOptions)

	return Scaffold{
		defaultForegroundColor: bunt.LightGray,

		factor: f,

		margin:  f * 48,
		padding: f * 24,

		drawDecorations: true,
		drawShadow:      true,

		shadowBaseColor: "#10101066",
		shadowRadius:    uint8(math.Min(f*16, 255)),
		shadowOffsetX:   f * 16,
		shadowOffsetY:   f * 16,

		regular:    regular,
		bold:       bold,
		italic:     italic,
		boldItalic: boldItalic,
		fallback:   fallback,
		hackFont:   hackFont,

		lineSpacing: 1.2,
		tabSpaces:   2,
	}
}

// newFace parses font bytes into a face and returns the face plus the parsed
// *truetype.Font. It returns (nil, nil) if the font cannot be parsed.
func newFace(data []byte, opts *truetype.Options) (imgfont.Face, *truetype.Font) {
	ft, err := truetype.Parse(data)
	if err != nil {
		return nil, nil
	}
	return truetype.NewFace(ft, opts), ft
}

// primaryFace returns the Hack face matching the character's bold/italic style.
func (s *Scaffold) primaryFace(settings uint64) imgfont.Face {
	switch settings & 0x1C {
	case 4:
		return s.bold
	case 8:
		return s.italic
	case 12:
		return s.boldItalic
	default:
		return s.regular
	}
}

// faceFor returns the font face to render a character with. If the Hack font
// covers the rune, the styled Hack face is used (so ➜, box-drawing, ASCII stay
// on Hack). If Hack lacks the glyph, the embedded emoji face is used so emoji
// (e.g. 😊) show as a single-color shape instead of a tofu box.
func (s *Scaffold) faceFor(cr bunt.ColoredRune) imgfont.Face {
	if s.hackFont != nil && s.hackFont.Index(cr.Symbol) != 0 {
		return s.primaryFace(cr.Settings)
	}
	if s.fallback != nil {
		return s.fallback
	}
	return s.primaryFace(cr.Settings)
}

// SetFontFaceRegular overrides the regular font face.
func (s *Scaffold) SetFontFaceRegular(face imgfont.Face) { s.regular = face }

// SetFontFaceBold overrides the bold font face.
func (s *Scaffold) SetFontFaceBold(face imgfont.Face) { s.bold = face }

// SetFontFaceItalic overrides the italic font face.
func (s *Scaffold) SetFontFaceItalic(face imgfont.Face) { s.italic = face }

// SetFontFaceBoldItalic overrides the bold-italic font face.
func (s *Scaffold) SetFontFaceBoldItalic(face imgfont.Face) { s.boldItalic = face }

// SetColumns enforces a fixed number of columns for content wrapping.
func (s *Scaffold) SetColumns(columns int) { s.columns = columns }

// SetMargin sets the margin (in "content units") around the window.
func (s *Scaffold) SetMargin(margin float64) { s.margin = margin * s.factor }

// SetPadding sets the padding (in "content units") inside the window.
func (s *Scaffold) SetPadding(padding float64) { s.padding = padding * s.factor }

// DrawDecorations toggles the window decoration buttons.
func (s *Scaffold) DrawDecorations(value bool) { s.drawDecorations = value }

// DrawShadow toggles the window shadow.
func (s *Scaffold) DrawShadow(value bool) { s.drawShadow = value }

// ClipCanvas toggles clipping the canvas to the visible image area.
func (s *Scaffold) ClipCanvas(value bool) { s.clipCanvas = value }

// GetFixedColumns returns the configured column count, or the current terminal
// width if no fixed value was set.
func (s *Scaffold) GetFixedColumns() int {
	if s.columns != 0 {
		return s.columns
	}
	columns, _ := term.GetTerminalSize()
	return columns
}

// AddCommand prepends the command line (with a prompt indicator) to the content.
func (s *Scaffold) AddCommand(args ...string) error {
	tokens := commandTokens(args)
	if len(tokens) == 0 {
		return nil
	}

	// Render the command banner with a self-contained syntax highlight so that
	// `--show-cmd` looks like the colored command line a shell would show:
	// green prompt, green command, then flags/paths/args in distinct colors.
	var b strings.Builder
	b.WriteString(bunt.Sprintf("Lime{%s}", commandIndicator))
	b.WriteByte(' ')
	b.WriteString(bunt.Sprintf("Green{%s}", tokens[0]))
	for _, arg := range tokens[1:] {
		b.WriteByte(' ')
		b.WriteString(colorCommandToken(arg))
	}
	b.WriteByte('\n')

	return s.AddContent(strings.NewReader(b.String()))
}

// commandTokens returns the command tokens to colorize. When the command was
// passed as a single shell-quoted string (e.g. `mu termshot --show-cmd 'ls -la'`),
// args[0] contains spaces, so it is split into tokens so each is colored.
func commandTokens(args []string) []string {
	if len(args) == 1 && strings.ContainsAny(args[0], " \t") {
		return splitShellLine(args[0])
	}
	return args
}

// splitShellLine splits a command line into tokens, keeping single- and
// double-quoted segments (and their quotes) together.
func splitShellLine(line string) []string {
	var tokens []string
	var cur strings.Builder
	var quote rune
	for _, r := range line {
		switch {
		case quote != 0:
			cur.WriteRune(r)
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
			cur.WriteRune(r)
		case r == ' ' || r == '\t':
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// colorCommandToken wraps a command-line argument in a bunt color based on its
// inferred type, mimicking shell syntax highlighting. Env assignments, flags,
// paths and quoted strings each get a distinct color; everything else uses the
// default text color.
func colorCommandToken(arg string) string {
	switch {
	case arg == "":
		return arg
	case isEnvAssignment(arg):
		return bunt.Sprintf("Magenta{%s}", arg)
	case isFlag(arg):
		return bunt.Sprintf("Yellow{%s}", arg)
	case isPath(arg):
		return bunt.Sprintf("Cyan{%s}", arg)
	case isQuoted(arg):
		return bunt.Sprintf("Yellow{%s}", arg)
	default:
		return bunt.Sprintf("LightGray{%s}", arg)
	}
}

// isEnvAssignment reports whether arg looks like a KEY=VALUE env assignment.
func isEnvAssignment(arg string) bool {
	key, _, ok := strings.Cut(arg, "=")
	if !ok || key == "" {
		return false
	}
	for i, r := range key {
		if r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func isFlag(arg string) bool {
	return len(arg) > 1 && arg[0] == '-'
}

func isPath(arg string) bool {
	return strings.HasPrefix(arg, "/") ||
		strings.HasPrefix(arg, "./") ||
		strings.HasPrefix(arg, "../") ||
		strings.HasPrefix(arg, "~/") ||
		strings.HasPrefix(arg, "$") ||
		strings.HasPrefix(arg, `\`)
}

func isQuoted(arg string) bool {
	return len(arg) >= 2 &&
		((arg[0] == '"' && arg[len(arg)-1] == '"') ||
			(arg[0] == '\'' && arg[len(arg)-1] == '\''))
}

// AddContent parses rich text (ANSI/bunt markup) from in and appends it to the
// scaffold, wrapping at the configured column count if needed.
func (s *Scaffold) AddContent(in io.Reader) error {
	parsed, err := bunt.ParseStream(in)
	if err != nil {
		return fmt.Errorf("failed to parse input stream: %w", err)
	}

	var tmp bunt.String
	var counter int
	for _, cr := range *parsed {
		counter++

		if cr.Symbol == '\n' {
			counter = 0
		}

		// Insert a newline when the column count is reached so content wraps.
		if counter > s.GetFixedColumns() {
			counter = 0
			tmp = append(tmp, bunt.ColoredRune{
				Settings: cr.Settings,
				Symbol:   '\n',
			})
		}

		tmp = append(tmp, cr)
	}

	s.content = append(s.content, tmp...)

	return nil
}

func (s *Scaffold) fontHeight() float64 {
	return float64(s.regular.Metrics().Height >> 6)
}

func (s *Scaffold) measureContent() (width float64, height float64) {
	tmp := make([]rune, len(s.content))
	for i, cr := range s.content {
		tmp[i] = cr.Symbol
	}

	lines := strings.Split(
		strings.TrimSuffix(string(tmp), "\n"),
		"\n",
	)

	// width, either by using longest line, or by fixed column value
	switch s.columns {
	case 0: // unlimited: max width of all lines, measured per-rune with fallback
		lineWidth := 0.0
		for _, cr := range s.content {
			if cr.Symbol == '\n' {
				if lineWidth > width {
					width = lineWidth
				}
				lineWidth = 0
				continue
			}
			if adv, ok := s.faceFor(cr).GlyphAdvance(cr.Symbol); ok {
				lineWidth += float64(adv >> 6)
			}
		}
		if lineWidth > width {
			width = lineWidth
		}

	default: // fixed: max width based on column count
		tmpDrawer := &imgfont.Drawer{Face: s.regular}
		width = float64(tmpDrawer.MeasureString(strings.Repeat("a", s.GetFixedColumns())) >> 6)
	}

	// height, lines times font height and line spacing
	height = float64(len(lines)) * s.fontHeight() * s.lineSpacing

	return width, height
}

func (s *Scaffold) image() (image.Image, error) {
	f := func(value float64) float64 { return s.factor * value }

	var (
		corner   = f(6)
		radius   = f(9)
		distance = f(25)
	)

	contentWidth, contentHeight := s.measureContent()

	// Make sure the output window is big enough in case no content or very few
	// content will be rendered.
	contentWidth = math.Max(contentWidth, 3*distance+3*radius)

	marginX, marginY := s.margin, s.margin
	paddingX, paddingY := s.padding, s.padding

	xOffset := marginX
	yOffset := marginY

	var titleOffset float64
	if s.drawDecorations {
		titleOffset = f(40)
	}

	width := contentWidth + 2*marginX + 2*paddingX
	height := contentHeight + 2*marginY + 2*paddingY + titleOffset

	dc := gg.NewContext(int(width), int(height))

	// Optional: Apply blurred rounded rectangle to mimic the window shadow.
	if s.drawShadow {
		xOffset -= s.shadowOffsetX / 2
		yOffset -= s.shadowOffsetY / 2

		bc := gg.NewContext(int(width), int(height))
		bc.DrawRoundedRectangle(xOffset+s.shadowOffsetX, yOffset+s.shadowOffsetY, width-2*marginX, height-2*marginY, corner)
		bc.SetHexColor(s.shadowBaseColor)
		bc.Fill()

		src := bc.Image()
		dst := image.NewNRGBA(src.Bounds())
		if err := stackblur.Process(dst, src, uint32(s.shadowRadius)); err != nil {
			return nil, err
		}

		dc.DrawImage(dst, 0, 0)
	}

	// Draw rounded rectangle with outline to produce impression of a window.
	dc.DrawRoundedRectangle(xOffset, yOffset, width-2*marginX, height-2*marginY, corner)
	dc.SetHexColor("#151515")
	dc.Fill()

	dc.DrawRoundedRectangle(xOffset, yOffset, width-2*marginX, height-2*marginY, corner)
	dc.SetHexColor("#404040")
	dc.SetLineWidth(f(1))
	dc.Stroke()

	// Optional: Draw window decorations (three buttons).
	if s.drawDecorations {
		for i, color := range []string{red, yellow, green} {
			dc.DrawCircle(xOffset+paddingX+float64(i)*distance+f(4), yOffset+paddingY+f(4), radius)
			dc.SetHexColor(color)
			dc.Fill()
		}
	}

	// Apply the actual text into the prepared content area of the window.
	x, y := xOffset+paddingX, yOffset+paddingY+titleOffset+s.fontHeight()
	for _, cr := range s.content {
		// Pick the styled Hack face, falling back to the emoji face for glyphs
		// Hack does not have (e.g. emoji), so they render as a shape instead of
		// a tofu box.
		dc.SetFontFace(s.faceFor(cr))

		str := string(cr.Symbol)
		w, h := dc.MeasureString(str)

		// background color
		switch cr.Settings & 0x02 { //nolint:gocritic
		case 2:
			dc.SetRGB255(
				int((cr.Settings>>32)&0xFF),
				int((cr.Settings>>40)&0xFF),
				int((cr.Settings>>48)&0xFF),
			)

			dc.DrawRectangle(x, y-h+12, w, h)
			dc.Fill()
		}

		// foreground color
		switch cr.Settings & 0x01 {
		case 1:
			dc.SetRGB255(
				int((cr.Settings>>8)&0xFF),
				int((cr.Settings>>16)&0xFF),
				int((cr.Settings>>24)&0xFF),
			)

		default:
			dc.SetColor(s.defaultForegroundColor)
		}

		switch str {
		case "\n":
			x = xOffset + paddingX
			// Advance by the primary font's line height so spacing stays even
			// even when the fallback emoji face was the last one selected.
			y += s.fontHeight() * s.lineSpacing
			continue

		case "\t":
			x += w * float64(s.tabSpaces)
			continue

		case "✗", "ˣ": // mitigate issue #1 by replacing it with a similar character
			str = "×"
		}

		dc.DrawString(str, x, y)

		// There is no font face based way to draw an underlined string, so
		// manually draw a line under each character.
		if cr.Settings&0x1C == 16 {
			dc.DrawLine(x, y+f(4), x+w, y+f(4))
			dc.SetLineWidth(f(1))
			dc.Stroke()
		}

		x += w
	}

	return dc.Image(), nil
}

// WritePNG writes the scaffold content as a PNG into the provided writer.
func (s *Scaffold) WritePNG(w io.Writer) error {
	img, err := s.image()
	if err != nil {
		return err
	}

	// Optional: Clip image to minimum size by removing surrounding transparent pixels.
	if s.clipCanvas {
		if imgRGBA, ok := img.(*image.RGBA); ok {
			var minX, minY = math.MaxInt, math.MaxInt
			var maxX, maxY = 0, 0

			bounds := imgRGBA.Bounds()
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
					r, g, b, a := imgRGBA.At(x, y).RGBA()
					isTransparent := r == 0 && g == 0 && b == 0 && a == 0

					if !isTransparent {
						if x < minX {
							minX = x
						}
						if y < minY {
							minY = y
						}
						if x > maxX {
							maxX = x
						}
						if y > maxY {
							maxY = y
						}
					}
				}
			}

			img = imgRGBA.SubImage(image.Rect(minX, minY, maxX, maxY))
		}
	}

	return png.Encode(w, img)
}

// WriteRaw writes the scaffold content as-is (raw ANSI text) into the writer.
func (s *Scaffold) WriteRaw(w io.Writer) error {
	_, err := w.Write([]byte(s.content.String()))
	return err
}
