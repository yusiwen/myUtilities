<script>
  import { onMount, onDestroy } from 'svelte'

  let inGateway = $state(typeof window !== 'undefined' && window.__MU_GATEWAY__)
  let mergeRef
  let mv
  let hasContent = $state(false)

  const LS_KEY = 'mu-diff'

  function saveToLS() {
    if (!mv) return
    try {
      localStorage.setItem(LS_KEY, JSON.stringify({
        a: mv.a.state.doc.toString(),
        b: mv.b.state.doc.toString(),
      }))
    } catch {}
  }

  function loadFromLS() {
    try {
      const raw = localStorage.getItem(LS_KEY)
      return raw ? JSON.parse(raw) : null
    } catch { return null }
  }

  function clearAll() {
    if (!mv) return
    mv.a.dispatch({ changes: { from: 0, to: mv.a.state.doc.length, insert: '' } })
    mv.b.dispatch({ changes: { from: 0, to: mv.b.state.doc.length, insert: '' } })
    localStorage.removeItem(LS_KEY)
    hasContent = false
  }

  onMount(async () => {
    const { basicSetup, EditorView } = await import('codemirror')
    const { MergeView } = await import('@codemirror/merge')
    const { EditorState } = await import('@codemirror/state')

    const saveExt = EditorView.updateListener.of(update => {
      if (update.docChanged) {
        saveToLS()
        hasContent = true
      }
    })

    const monoTheme = EditorView.theme({
      '& .cm-content': { fontFamily: "'SF Mono', 'Fira Code', 'Consolas', monospace" },
      '& .cm-gutter': { fontFamily: "'SF Mono', 'Fira Code', 'Consolas', monospace" },
      '& .cm-gutters': { backgroundColor: 'var(--bg)', color: 'var(--text2)' },
      '& .cm-activeLineGutter': { backgroundColor: 'var(--surface2)' },
    })

    const saved = loadFromLS()

    mv = new MergeView({
      a: { doc: saved?.a || '', extensions: [basicSetup, monoTheme, saveExt] },
      b: { doc: saved?.b || '', extensions: [basicSetup, monoTheme, saveExt] },
      parent: mergeRef,
    })
    if (saved?.a || saved?.b) hasContent = true
  })

  onDestroy(() => {
    mv?.destroy()
  })

  function loadFile(side) {
    return (e) => {
      const file = e.target.files[0]
      if (!file) return
      const reader = new FileReader()
      reader.onload = () => {
        const v = side === 'a' ? mv.a : mv.b
        v.dispatch({ changes: { from: 0, to: v.state.doc.length, insert: reader.result } })
      }
      reader.readAsText(file)
    }
  }
</script>

<div class="app">
  <div class="top-bar">
    {#if inGateway}
      <a href="/" class="home-link" title="Back to Home">&larr; Home</a>
    {/if}
    <h1>Diff Tool</h1>
    <div class="spacer"></div>
    {#if hasContent}
      <button class="clear-btn" onclick={clearAll}>Clear</button>
    {/if}
    <label class="upload-btn">&#128194; A<input type="file" accept=".txt,.md,.js,.go,.py,.html,.css,.json,.yaml,.yml,.toml,.xml,.sh,.csv" onchange={loadFile('a')} /></label>
    <label class="upload-btn">&#128194; B<input type="file" accept=".txt,.md,.js,.go,.py,.html,.css,.json,.yaml,.yml,.toml,.xml,.sh,.csv" onchange={loadFile('b')} /></label>
  </div>
  <div class="merge-wrap" bind:this={mergeRef}></div>
</div>

<style>
  :global(html) { height: 100%; }
  :global(body) { margin: 0; overflow: hidden; height: 100%; }
  .app { height: 100vh; display: flex; flex-direction: column; }
  .top-bar { display: flex; align-items: center; gap: 12px; padding: 8px 60px 8px 16px; background: var(--surface); border-bottom: 1px solid var(--border); flex-shrink: 0; }
  .top-bar h1 { font-size: 16px; font-weight: 600; }
  .spacer { flex: 1; }

  .home-link { display: inline-flex; align-items: center; gap: 4px; padding: 3px 10px 3px 6px; border: 1px solid var(--border); border-radius: 20px; background: var(--surface); color: var(--text2); text-decoration: none; font-size: 12px; margin-right: 4px; }
  .home-link:hover { border-color: var(--primary); color: var(--text); }

  .clear-btn { display: inline-flex; align-items: center; gap: 4px; padding: 3px 10px; border: 1px solid var(--border); border-radius: 20px; background: var(--surface); color: var(--text2); cursor: pointer; font-size: 12px; }
  .clear-btn:hover { border-color: var(--danger); color: var(--danger); }

  .upload-btn { display: inline-flex; align-items: center; gap: 4px; padding: 3px 10px; border: 1px solid var(--border); border-radius: 20px; background: var(--surface); color: var(--text2); cursor: pointer; font-size: 12px; }
  .upload-btn:hover { border-color: var(--primary); color: var(--text); }
  .upload-btn input { display: none; }

  .merge-wrap { flex: 1; overflow: hidden; position: relative; }
  .merge-wrap :global(.cm-mergeView) { height: 100%; overflow: auto; }
  .merge-wrap :global(.cm-mergeViewEditors) { display: flex; height: 100%; }
  .merge-wrap :global(.cm-mergeViewEditor) { flex: 1; min-width: 0; display: flex; }
  .merge-wrap :global(.cm-mergeViewEditor .cm-editor) { flex: 1; }
  .merge-wrap :global(.cm-editor) { height: 100%; }
  .merge-wrap :global(.cm-editor.cm-focused) { outline: none; }
</style>
