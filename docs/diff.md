# diff — Text comparison tool

Compare two files or text strings with a side-by-side diff viewer. Supports both CLI and web UI.

```bash
# Compare two files with line numbers and colour
mu diff a.txt b.txt

# Render a patch from git, or from a file
git diff | mu diff
mu diff change.patch

# Compare text strings inline
mu diff text "old text" "new text"

# Serve the web UI (standalone)
mu diff serve

# Compare two files in the browser: opens the default browser
mu diff serve a.txt b.txt

# Three-way merge of text files
mu diff merge --base base.txt --ours local.txt --theirs remote.txt

# Render a unified diff produced by git or diff -u
git diff | mu diff patch
```


## Comparing two files

With two file paths, `mu diff` builds a unified diff in process and renders it
with the same gutter, colours and themes as `mu diff patch`:

```bash
mu diff old.txt new.txt
mu diff --tui old.txt new.txt
```

No `git` or `diff` process is started, so it behaves the same on Windows. The
rules match `diff -U3`:

- three lines of context; two changes whose unchanged gap is at most six lines
  share one hunk, a seven-line gap starts a second hunk;
- an empty or missing side (`/dev/null`, or `NUL` on Windows) renders as an added
  or deleted file, and the TUI shows `A`/`D`;
- a NUL byte on either side produces a `Binary files ... differ` notice instead of
  hunks;
- a missing final newline is marked with `\ No newline at end of file`;
- CRLF input renders without the carriage return, and identical files print
  nothing.

Each side is capped at 50 MiB, the same limit as the patch renderer. A path that
does not exist exits 1; a directory is refused because tree comparison is not
supported.

With one path, `mu diff` renders that unified diff file; with none, it renders a
diff from stdin. The `patch` subcommand remains as the explicit form.


## Patch rendering

`mu diff` renders a unified diff that another tool produced. It reads a file or
stdin, so it works with `git diff`, `git show`, `git format-patch` and plain
`diff -u`; `mu diff patch` is the explicit spelling of the same path:

```bash
git diff | mu diff
git show HEAD~1 | mu diff --no-line-numbers
diff -u old.txt new.txt | mu diff --color=always
mu diff change.patch
mu diff patch change.patch
```

The output keeps the original headers, adds an old/new line-number gutter, and
colours additions, deletions, context and hunk headers:

```
@@ -1,3 +1,3 @@
1 1 |  one
2   | -old
  2 | +new
3 3 |  two
```

| Flag | Default | Meaning |
|---|---|---|
| `--color` | `auto` | `auto` (colour only on a terminal), `always`, `never` |
| `--[no-]line-numbers` | on | show the old/new line-number gutter |
| `--theme` | `nord` | `nord` (24-bit Nord palette) or `default` (classic 16-colour) |

Plain mode has no ANSI escapes, so it can be redirected. A patch that cannot be
parsed exits 2 with the offending line number; text that is not a diff exits 2
with `input does not look like a unified diff`; binary input (a NUL byte) and
patches over 50 MiB are refused. Combined diffs (`@@@`, `diff --cc`) are not
supported.


## Interactive viewer

`mu diff --tui` opens a full-screen viewer: the changed files on the left, the
selected file's diff on the right. The same flag works on `mu diff patch`.

```bash
mu diff --tui change.patch
mu diff --tui old.txt new.txt
git diff | mu diff --tui
```

| Key | Action |
|---|---|
| `tab`, `shift+tab`, left/right | switch focus between the file list and the diff |
| `j`/`k`, up/down | move the file selection, or scroll the diff |
| `g`/`G` | first/last file, top/bottom of the diff |
| `ctrl+d`/`ctrl+u`, PgDn/PgUp | page through the list or the diff |
| `enter` | jump from the list into the diff |
| `q`, `esc`, `ctrl+c` | quit |

The focused pane's title and the header's `focus:` label show which side is
active. File rows carry a status letter (`A` added, `D` deleted, `M` modified,
`R` renamed, `C` copied, `B` binary), and the right title shows `+adds -deletes`.

`--tui` requires a terminal on stdout; redirecting it exits 2. When the patch
arrives on stdin the viewer reads keys from the controlling terminal, so
`git diff | mu diff patch --tui` works. `--no-line-numbers` applies to the
viewer too; colour is always on inside the TUI and follows `--theme`
(Nord by default).


## Themes

`mu diff`, `mu diff file`, `mu diff text` and `mu diff patch` accept
`--theme nord|default`.
Nord is the default; `--theme default` restores the classic green/red/cyan
palette. Nord uses the [Nord](https://www.nordtheme.com) palette:

| Diff element | Nord colour |
|---|---|
| additions, `+++` path | nord14 `#A3BE8C` |
| deletions, `---` path | nord11 `#BF616A` |
| hunk header, focused pane title | nord8 `#88C0D0` |
| binary notice, modified status | nord13 `#EBCB8B` |
| metadata, gutter | nord3 `#4C566A` |
| context | nord4 `#D8DEE9`, dimmed |

Plain output uses 24-bit ANSI colour, so a terminal without truecolor support
will approximate the palette. The TUI resolves the same values through lipgloss
and follows the terminal's colour profile; `--tui` uses the selected theme for
the file list, status letters, selection and diff pane together.

## Three-way merge

`mu diff merge` combines a common ancestor with two edited versions and writes the
result to standard output. It is git-agnostic: the inputs are three files (or one
`-` for stdin), and none of the inputs are modified.

```bash
mu diff merge --base base.txt --ours local.txt --theirs remote.txt
mu diff merge --base base.txt --ours local.txt --theirs remote.txt --style diff3
mu diff merge --base base.txt --ours local.txt --theirs remote.txt --check
```

```
$ mu diff merge --base base.txt --ours ours.txt --theirs theirs.txt
A
<<<<<<< ours.txt
X
=======
Y
>>>>>>> theirs.txt
C
```

| Flag | Default | Meaning |
|---|---|---|
| `--base` / `--ours` / `--theirs` | required | the three inputs; `-` reads stdin (only one may) |
| `--style` | `merge` | `merge` (two-sided markers) or `diff3` (also shows the ancestor) |
| `--ours-label` / `--base-label` / `--theirs-label` | file name | text written after the marker |
| `--marker-size` | `7` | marker width in characters |
| `--check` | `false` | write nothing; only report whether there are conflicts |

Exit codes follow `git merge-file`: `0` clean, `1` conflicts, `2` unreadable input
or usage error. Output is plain text with no ANSI escapes, so it can be redirected
into a file. Edits on abutting lines are treated as one conflict, matching diff3
and `git merge-file`. Binary inputs (a NUL byte) and inputs over 10 MiB are
rejected with a message naming the file.

To resolve a git conflict without teaching `mu` about git:

```bash
git show :1:path > /tmp/base
git show :2:path > /tmp/ours
git show :3:path > /tmp/theirs
mu diff merge --base /tmp/base --ours /tmp/ours --theirs /tmp/theirs --style diff3 > path
```

## Git integration

`git difftool` invokes the tool with two file paths (`$LOCAL` and `$REMOTE`), which
is exactly the two-argument form of `mu diff`:

```bash
git config --global difftool.mu.cmd 'mu diff "$LOCAL" "$REMOTE"'
git config --global difftool.prompt false
git difftool --tool=mu
```

`mu diff` exits 0 even when the files differ, so `--trust-exit-code` is safe. For
an added file git passes `/dev/null` on the left, which renders as an addition.
`git difftool -d` passes two directories instead and is not supported: it fails
with `comparing two trees is not supported`.

As a pager for patch output:

```bash
git -c pager.diff='mu diff' diff
git -c pager.show='mu diff' show
```

Combined diffs (`diff --cc`, `@@@`) are rejected, so a pager configured this way
shows an error for conflicted files during a merge; use `git diff` directly there.
`git difftool` itself never sends a combined diff.

## Browser comparison

With two file arguments, `mu diff serve` starts the server, preloads both files
into the merge view and opens the default browser:

```
$ mu diff serve a.txt b.txt
Diff tool server listening on http://127.0.0.1:8088/
Comparing a.txt and b.txt
Press Ctrl+C to stop.
```

The page shows the file names in the header and both files side by side. A
reload re-reads the files, so saving an edit and refreshing shows the new
content. Without file arguments the server behaves as before: the merge view
starts empty and content comes from localStorage or the upload buttons.

| Flag | Default | Meaning |
|---|---|---|
| `--port` | `8088` | Port to listen on; `0` picks a free port |
| `--host` | loopback with files, all interfaces without | Address to bind to |
| `--[no-]open` | `true` with files | Open the default browser once the port is bound |

Binding only loopback in file mode keeps local file contents off the network.
`--host 0.0.0.0` still works for a deliberate share, and prints a warning that
anyone who can reach the port can read the compared files.

## Input limits

The web view renders text, so anything that is not readable UTF-8 text is
rejected with a message naming the file:

- a directory or device instead of a regular file;
- a file over 5 MiB, since CodeMirror's merge view cannot render it usefully — it
  is refused rather than truncated;
- a binary file (one containing a NUL byte);
- text that is not valid UTF-8, such as a GBK file — convert it first:
  `iconv -f GBK -t UTF-8 input.txt > output.txt`.

## HTTP API

`mu diff serve` also backs the gateway's `/diff/` page, which serves an empty
editor.

| Request | Response |
|---|---|
| `GET /` | the merge view page |
| `GET /api/diff/source` | `{"file1","file2","text1","text2"}` for the preloaded pair; `204 No Content` when started without files |
| `POST /api/diff` | diff entries for a `{"text1","text2"}` body |

## Web UI

The page provides a full-page CodeMirror merge view with:

- side-by-side editors with real-time diff highlighting
- the preloaded file pair, or file upload for both sides
- synchronized scrolling between panes
- auto-save to localStorage, used when the server preloaded nothing
