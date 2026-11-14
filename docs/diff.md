# diff — Text comparison tool

Compare two files or text strings with a side-by-side diff viewer. Supports both CLI and web UI.

```bash
# Compare two files in the terminal
mu diff file a.txt b.txt

# Compare text strings
mu diff text "old text" "new text"

# Serve the web UI (standalone)
mu diff serve

# Compare two files in the browser: opens the default browser
mu diff serve a.txt b.txt
```

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
