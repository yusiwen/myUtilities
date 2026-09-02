# mu termshot

Generate a screenshot of terminal command output that looks like a terminal
window. It runs the given command in a pseudo terminal, captures the (ANSI
rich) output, and renders it as a PNG — no manual pasting, no extra tooling.

Inspired by [homeport/termshot](https://github.com/homeport/termshot); the core
renderer and PTY exec are ported into `internal/core/termshot/`.

## Usage

Like `time`, `watch`, or `perf`, prefix any command with `mu termshot`:

```bash
mu termshot ls -a
```

This produces `out.png` in the current directory.

Because `mu termshot` accepts its own flags, use `--` (or quote the whole
command) so everything after it belongs to the target command:

```bash
mu termshot -- "ls -1 | grep go"
```

`mu termshot` is CLI-only (no web UI); it is not exposed through the gateway.

## Flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--edit` | `-e` | off | Edit the output in `$EDITOR` (fallback `vi`) before rendering |
| `--show-cmd` | `-c` | off | Include the command line in the screenshot |
| `--columns` | `-C` | `0` | Force a fixed number of columns (wraps output) |
| `--margin` | `-m` | `48` | Margin around the window |
| `--padding` | `-p` | `24` | Padding around the content inside the window |
| `--no-decoration` | | off | Do not draw window decoration buttons |
| `--no-shadow` | | off | Do not draw the window shadow |
| `--clip-canvas` | `-s` | off | Clip the canvas to the visible image area |
| `--filename` | `-f` | `out.png` | Output PNG path (must end in `.png`) |
| `--clipboard` | `-b` | off | Copy the image to the OS clipboard (macOS only) |
| `--raw-write` | | | Write raw ANSI output to a file (`-` for stdout) instead of an image |
| `--raw-read` | | | Read raw ANSI input from a file (`-` for stdin) instead of running a command |

## Examples

```bash
# Basic screenshot
mu termshot ls -a

# Show the command in the screenshot, save to a specific path
mu termshot --show-cmd --filename shot.png -- ls -la

# Force a fixed width so the screenshot does not grow too wide
mu termshot --columns 80 -- "neofetch"

# Render an existing raw ANSI capture (no command is executed)
mu termshot --raw-read capture.txt --filename shot.png

# Write the raw ANSI output as-is, without generating an image
mu termshot -- "ls -la" --raw-write raw.txt

# Remove sensitive output before rendering
mu termshot --edit -- "docker logs my-container"
```

> **Command banner:** With `--show-cmd`, the command line is rendered with a
> self-contained syntax highlight modeled on the **fast-syntax-highlighting
> default theme**: lime prompt, green command, cyan options/flags, magenta
> paths, yellow quoted strings and reserved words, light-green `KEY=value`
> variables. The colors are approximate and do not depend on your shell's
> highlighter or terminal palette.

> **Fonts:** The primary font is the embedded Hack monospace font. Emoji and
> emoji-like symbols (`😊`, `☺`, `❤`, `⚡`, `🚀`, `✅`, …) that Hack lacks are
> rendered with an embedded monochrome Noto Emoji font (Apache License 2.0,
> see `internal/core/termshot/fonts/NotoEmoji-LICENSE.txt`), shown as a
> single-color shape matching the text color rather than a tofu box. A few bare
> text symbols (e.g. `✓`, U+2713) are in neither font and still render as a box.
