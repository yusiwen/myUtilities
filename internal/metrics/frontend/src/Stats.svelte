<script>
  let { card = {}, host = 'all' } = $props()

  function primaryLine() {
    if (!card) return null
    if (host !== 'all') {
      const l = card.lines.find((x) => x.host === host && x.points.length > 0)
      if (l) return l
    }
    return card.lines.find((x) => x.points.length > 0) || null
  }

  function fmt(v) {
    if (v === null || v === undefined) return '-'
    return Number(v).toFixed(2)
  }

  let line = $derived(primaryLine())
</script>

{#if line}
  <div class="metric-stats">
    <span>latest <b>{fmt(line.points[line.points.length - 1]?.y)}</b></span>
    <span>max <b>{fmt(line.points.reduce((m, p) => Math.max(m, p.y), -Infinity))}</b></span>
    <span>min <b>{fmt(line.points.reduce((m, p) => Math.min(m, p.y), Infinity))}</b></span>
    <span class="points-count">{line.points.length} pts</span>
  </div>
{/if}

<style>
  .metric-stats {
    display: flex;
    gap: 16px;
    font-size: 12px;
    color: var(--text3);
    margin-top: 10px;
    flex-wrap: wrap;
  }

  .metric-stats b {
    color: var(--text2);
    font-family: 'Menlo', 'Consolas', monospace;
  }

  .points-count {
    margin-left: auto;
  }
</style>