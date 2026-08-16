# diff — Text comparison tool

Compare two files or text strings with a side-by-side diff viewer. Supports both CLI and web UI.

```bash
# Compare two files
mu diff file a.txt b.txt

# Compare text strings
mu diff text "old text" "new text"

# Serve web UI (standalone)
mu diff serve --port 8088
```

The web UI provides a full-page CodeMirror-based merge view with:
- Side-by-side editors with real-time diff highlighting
- File upload for both sides
- Synchronized scrolling between panes
- Auto-save to localStorage (content persists across page reloads)
