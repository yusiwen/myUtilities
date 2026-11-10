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
      '&': { color: 'var(--text)', backgroundColor: 'var(--bg)' },
      '& .cm-content': { fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace', caretColor: 'var(--accent)' },
      '& .cm-line': { padding: '0 8px' },
      '& .cm-gutter': { fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace' },
      '& .cm-gutters': { backgroundColor: 'var(--bg)', color: 'var(--text3)', borderColor: 'var(--border)' },
      '& .cm-activeLine': { backgroundColor: 'rgba(233, 235, 242, 0.04)' },
      '& .cm-activeLineGutter': { backgroundColor: 'var(--surface2)', color: 'var(--text2)' },
      '& .cm-cursor': { borderLeftColor: 'var(--accent)' },
      '& .cm-selectionBackground, &.cm-focused .cm-selectionBackground': { backgroundColor: 'rgba(233, 69, 96, 0.28)' },
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

<div class="diff-app">
  <header class="topbar">
    <a class="brand" href={inGateway ? '/' : undefined} aria-label="mu gateway">
      <span class="mu">mu</span><span class="sep">/</span><span class="app-name">diff</span>
    </a>
    <div class="spacer"></div>
    <div class="actions">
      {#if hasContent}
        <button class="btn xs" onclick={clearAll}>Clear</button>
      {/if}
      <label class="btn xs">
        Upload A
        <input type="file" accept=".txt,.md,.js,.go,.py,.html,.css,.json,.yaml,.yml,.toml,.xml,.sh,.csv" onchange={loadFile('a')} />
      </label>
      <label class="btn xs">
        Upload B
        <input type="file" accept=".txt,.md,.js,.go,.py,.html,.css,.json,.yaml,.yml,.toml,.xml,.sh,.csv" onchange={loadFile('b')} />
      </label>
    </div>
  </header>

  <div class="merge-wrap" bind:this={mergeRef}></div>
</div>

<style>
  :global(body) { --page-max: 100vw; }
  :global(html), :global(body) { height: 100%; }
  :global(body) { margin: 0; overflow: hidden; }

  .diff-app { height: 100vh; height: 100svh; display: flex; flex-direction: column; }
  .merge-wrap { position: relative; flex: 1; min-height: 0; overflow: hidden; }
  .btn input { display: none; }

  .merge-wrap :global(.cm-mergeView) { height: 100%; overflow: auto; }
  .merge-wrap :global(.cm-mergeViewEditors) { display: flex; height: 100%; }
  .merge-wrap :global(.cm-mergeViewEditor) { flex: 1; min-width: 0; display: flex; }
  .merge-wrap :global(.cm-mergeViewEditor .cm-editor) { flex: 1; }
  .merge-wrap :global(.cm-editor) { height: 100%; }
  .merge-wrap :global(.cm-editor.cm-focused) { outline: none; }
  .merge-wrap :global(.cm-panel input) {
    width: auto;
    min-height: 26px;
    height: 26px;
    padding: 2px 8px;
    border-radius: var(--r-sm);
    background: var(--surface2);
  }
</style>
