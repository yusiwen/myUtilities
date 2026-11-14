<script>
  import { onMount, onDestroy } from 'svelte'

  let inGateway = $state(typeof window !== 'undefined' && window.__MU_GATEWAY__)
  let mergeRef
  let mv
  let hasContent = $state(false)
  // source is the file pair preloaded by `mu diff serve file1 file2`.
  let source = $state(null)
  let sourceError = $state('')

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

  // loadSource returns the server-side file pair, or null when the page was not
  // started with files (204) — the gateway always answers 204.
  async function loadSource() {
    try {
      const res = await fetch('/api/diff/source', { cache: 'no-store' })
      if (res.status === 204) return null
      if (!res.ok) {
        sourceError = (await res.text()).trim() || `HTTP ${res.status}`
        return null
      }
      const data = await res.json()
      if (!data || (!data.text1 && !data.text2)) return null
      return data
    } catch (err) {
      sourceError = String(err)
      return null
    }
  }

  onMount(async () => {
    const { basicSetup, EditorView } = await import('codemirror')
    const { MergeView } = await import('@codemirror/merge')
    const saved = loadFromLS()
    const preloaded = await loadSource()
    source = preloaded

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

    // A preloaded pair wins over localStorage: the command named these files.
    const docA = preloaded ? preloaded.text1 : (saved?.a || '')
    const docB = preloaded ? preloaded.text2 : (saved?.b || '')

    mv = new MergeView({
      a: { doc: docA, extensions: [basicSetup, monoTheme, saveExt] },
      b: { doc: docB, extensions: [basicSetup, monoTheme, saveExt] },
      parent: mergeRef,
    })
    if (docA || docB) hasContent = true
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
    {#if source}
      <span class="src" title="{source.file1} ↔ {source.file2}">
        <span class="name">{source.file1}</span>
        <span class="arrow">↔</span>
        <span class="name">{source.file2}</span>
      </span>
    {/if}
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

  {#if sourceError}
    <div class="banner" role="alert">Cannot load the compared files: {sourceError}</div>
  {/if}

  <div class="merge-wrap" bind:this={mergeRef}></div>
</div>

<style>
  :global(body) { --page-max: 100vw; }
  :global(html), :global(body) { height: 100%; }
  :global(body) { margin: 0; overflow: hidden; }

  .diff-app { height: 100vh; height: 100svh; display: flex; flex-direction: column; }
  .merge-wrap { position: relative; flex: 1; min-height: 0; overflow: hidden; }
  .btn input { display: none; }

  .src {
    display: flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
    max-width: 46vw;
    padding: 3px 8px;
    border: 1px solid var(--border);
    border-radius: var(--r-sm);
    background: var(--surface2);
    color: var(--text2);
    font-family: var(--mono);
    font-size: 12px;
  }
  .src .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .src .arrow { flex: none; color: var(--text3); }

  .banner {
    padding: 6px var(--gap);
    border-bottom: 1px solid var(--border);
    background: rgba(233, 69, 96, 0.12);
    color: var(--text);
    font-size: 12px;
  }

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
