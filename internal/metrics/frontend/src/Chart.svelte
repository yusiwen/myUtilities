<script>
  import { onDestroy } from 'svelte'
  import { Chart, LineController, LineElement, PointElement, LinearScale, TimeScale, Tooltip, Filler } from 'chart.js'
  import 'chartjs-adapter-date-fns'

  Chart.register(LineController, LineElement, PointElement, LinearScale, TimeScale, Tooltip, Filler)

  let { lines = [], height = 160, from, to } = $props()

  let canvas
  let chart

  function toPlain(pts) {
    return pts.map((p) => ({ x: p.x, y: p.y }))
  }

  function xUnit() {
    const span = to && from ? to - from : 0
    if (span <= 3600e3) return 'minute'
    if (span <= 86400e3) return 'hour'
    return 'day'
  }

  function render() {
    if (!canvas) return
    if (chart) {
      chart.destroy()
      chart = null
    }
    chart = new Chart(canvas, {
      type: 'line',
      data: {
        datasets: lines.map((l) => ({
          label: l.label || '',
          data: toPlain(l.points || []),
          borderColor: l.color,
          backgroundColor: (l.color || '#4a9eff') + '33',
          borderWidth: 1.5,
          pointRadius: 0,
          pointHitRadius: 8,
          fill: lines.length === 1,
          tension: 0.2
        }))
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        animation: false,
        interaction: { mode: 'index', intersect: false },
        scales: {
          x: {
            type: 'time',
            time: { unit: xUnit() },
            min: from,
            max: to,
            grid: { color: 'rgba(128,128,128,0.15)' },
            ticks: { color: 'rgba(128,128,128,0.7)', maxTicksLimit: 6, maxRotation: 0 }
          },
          y: {
            beginAtZero: false,
            grid: { color: 'rgba(128,128,128,0.15)' },
            ticks: { color: 'rgba(128,128,128,0.7)', maxTicksLimit: 5 }
          }
        },
        plugins: {
          legend: { display: lines.length > 1 },
          tooltip: { mode: 'index', intersect: false }
        }
      }
    })
  }

  $effect(() => {
    lines
    from
    to
    render()
  })

  onDestroy(() => {
    if (chart) chart.destroy()
  })
</script>

<div class="chart-wrap" style="height: {height}px">
  <canvas bind:this={canvas}></canvas>
</div>

<style>
  .chart-wrap {
    position: relative;
    width: 100%;
  }
</style>