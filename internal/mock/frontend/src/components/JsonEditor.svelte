<script>
  import { onMount, onDestroy } from 'svelte'

  let { value = '', onChange } = $props()

  let editorEl
  let view

  onMount(async () => {
    const { EditorView, basicSetup } = await import('codemirror')
    const { json, jsonParseLinter } = await import('@codemirror/lang-json')
    const { linter } = await import('@codemirror/lint')

    view = new EditorView({
      doc: value,
      extensions: [
        basicSetup,
        json(),
        linter(jsonParseLinter()),
        EditorView.updateListener.of(update => {
          if (update.docChanged) {
            onChange(update.state.doc.toString())
          }
        }),
      ],
      parent: editorEl,
    })
  })

  onDestroy(() => {
    view?.destroy()
  })

  function format() {
    try {
      const parsed = JSON.parse(view.state.doc.toString())
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: JSON.stringify(parsed, null, 2) },
      })
    } catch {
      // ignore format errors
    }
  }

  defineExpose({ format })
</script>

<div class="editor-wrapper">
  <div bind:this={editorEl} class="editor"></div>
  <button class="fmt-btn" onclick={format} title="Format JSON">Format</button>
</div>

<style>
  .editor-wrapper { position: relative; border: 1px solid var(--border); border-radius: 6px; overflow: hidden; }
  .editor { min-height: 180px; }
  .editor { font-size: 13px; }
  .editor { background: #0d1117; }
  .fmt-btn { position: absolute; top: 6px; right: 6px; font-size: 11px; padding: 3px 10px; border: 1px solid var(--border); border-radius: 4px; background: var(--surface2); color: var(--text); cursor: pointer; z-index: 10; }
  .fmt-btn:hover { background: var(--primary); }
</style>
