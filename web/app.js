(function () {
  const API_URL = '/api/runs?limit=25';

  async function fetchDashboardData() {
    try {
      const res = await fetch(API_URL);
      if (!res.ok) {
        throw new Error(`HTTP ${res.status}`);
      }
      const data = await res.json();
      renderStats(data.aggregate, data.runs);
      renderTable(data.runs);
      renderChart(data.runs);
    } catch (err) {
      console.error('Failed to load runs:', err);
      const tbody = document.getElementById('runsTableBody');
      if (tbody) {
        tbody.innerHTML = `<tr><td colspan="6" class="empty-state">Error connecting to Diffr API: ${escapeHtml(err.message)}</td></tr>`;
      }
    }
  }

  function renderStats(aggregate, runs) {
    const totalRunsEl = document.getElementById('statTotalRuns');
    const avgPctEl = document.getElementById('statAvgPct');
    const totalMsEl = document.getElementById('statTotalMs');

    if (aggregate) {
      totalRunsEl.textContent = aggregate.total_runs ?? 0;
      avgPctEl.textContent = (aggregate.avg_pct_time_saved ?? 0).toFixed(1) + '%';
      totalMsEl.textContent = (aggregate.total_ms_saved ?? 0).toLocaleString() + ' ms';
    } else if (runs) {
      totalRunsEl.textContent = runs.length;
      avgPctEl.textContent = '0.0%';
      totalMsEl.textContent = '0 ms';
    }
  }

  function renderTable(runs) {
    const tbody = document.getElementById('runsTableBody');
    if (!runs || runs.length === 0) {
      tbody.innerHTML = `<tr><td colspan="6" class="empty-state">No test runs recorded yet. Execute 'diffr run' to generate data.</td></tr>`;
      return;
    }

    tbody.innerHTML = runs.map(run => {
      const shortRef1 = (run.ref1 || 'HEAD~1').substring(0, 7);
      const shortRef2 = (run.ref2 || 'HEAD').substring(0, 7);
      const commitPair = `${shortRef1}..${shortRef2}`;

      const date = run.timestamp ? new Date(run.timestamp) : new Date();
      const timeStr = date.toISOString().replace('T', ' ').substring(0, 19);

      const changedFuncsCount = (run.changed_functions || []).length;
      const impactedTestsCount = (run.impacted_tests || []).length;
      const totalTests = run.total_tests_in_repo || 0;

      const msSaved = (run.baseline_full_suite_ms || 0) - (run.actual_run_ms || 0);
      const pctSaved = run.pct_time_saved ?? 0;

      let savedHtml = '';
      if (msSaved > 0) {
        savedHtml = `<span class="time-positive">+${msSaved}ms (${pctSaved.toFixed(1)}%)</span>`;
      } else {
        savedHtml = `<span class="time-neutral">${msSaved}ms (${pctSaved.toFixed(1)}%)</span>`;
      }

      const cacheBadge = run.cache_hit
        ? `<span class="badge badge-cached">CACHED</span>`
        : `<span class="badge badge-full">FULL</span>`;

      return `
        <tr>
          <td>${escapeHtml(commitPair)}</td>
          <td>${timeStr}</td>
          <td>${changedFuncsCount}</td>
          <td>${impactedTestsCount} / ${totalTests}</td>
          <td>${savedHtml}</td>
          <td>${cacheBadge}</td>
        </tr>
      `;
    }).join('');
  }

  function renderChart(runs) {
    const canvas = document.getElementById('savingsChart');
    if (!canvas) return;

    // Handle high DPI displays
    const dpr = window.devicePixelRatio || 1;
    const rect = canvas.parentElement.getBoundingClientRect();
    const width = rect.width;
    const height = 200;

    canvas.width = width * dpr;
    canvas.height = height * dpr;
    canvas.style.width = width + 'px';
    canvas.style.height = height + 'px';

    const ctx = canvas.getContext('2d');
    ctx.scale(dpr, dpr);

    ctx.clearRect(0, 0, width, height);

    if (!runs || runs.length === 0) {
      ctx.fillStyle = '#8a8a86';
      ctx.font = '11px ui-monospace, monospace';
      ctx.textAlign = 'center';
      ctx.fillText('No historical execution data to plot', width / 2, height / 2);
      return;
    }

    // Chronological order: oldest to newest
    const chronologicalRuns = [...runs].reverse();
    let cumulative = 0;
    const dataPoints = chronologicalRuns.map(r => {
      const saved = Math.max(0, (r.baseline_full_suite_ms || 0) - (r.actual_run_ms || 0));
      cumulative += saved;
      return cumulative;
    });

    const padding = { top: 20, right: 24, bottom: 30, left: 60 };
    const chartW = width - padding.left - padding.right;
    const chartH = height - padding.top - padding.bottom;

    const maxVal = Math.max(...dataPoints, 100);
    const minVal = 0;

    // Draw minimal horizontal gridlines (3 lines)
    ctx.strokeStyle = '#2a2a2a';
    ctx.lineWidth = 1;
    ctx.fillStyle = '#8a8a86';
    ctx.font = '11px ui-monospace, monospace';
    ctx.textAlign = 'right';

    const gridSteps = 3;
    for (let i = 0; i <= gridSteps; i++) {
      const val = Math.round((maxVal / gridSteps) * i);
      const y = padding.top + chartH - (i / gridSteps) * chartH;

      ctx.beginPath();
      ctx.moveTo(padding.left, y);
      ctx.lineTo(padding.left + chartW, y);
      ctx.stroke();

      ctx.fillText(val + 'ms', padding.left - 8, y + 4);
    }

    // Draw single line chart (no fill/gradient)
    if (dataPoints.length === 1) {
      // Single run: draw dot
      const x = padding.left + chartW / 2;
      const y = padding.top + chartH - ((dataPoints[0] - minVal) / (maxVal - minVal)) * chartH;
      ctx.fillStyle = '#4ade80';
      ctx.beginPath();
      ctx.arc(x, y, 4, 0, Math.PI * 2);
      ctx.fill();
    } else {
      ctx.strokeStyle = '#4ade80';
      ctx.lineWidth = 1.5;
      ctx.beginPath();

      dataPoints.forEach((val, idx) => {
        const x = padding.left + (idx / (dataPoints.length - 1)) * chartW;
        const y = padding.top + chartH - ((val - minVal) / (maxVal - minVal)) * chartH;
        if (idx === 0) {
          ctx.moveTo(x, y);
        } else {
          ctx.lineTo(x, y);
        }
      });
      ctx.stroke();

      // Draw subtle point on latest data point
      const lastX = padding.left + chartW;
      const lastY = padding.top + chartH - ((dataPoints[dataPoints.length - 1] - minVal) / (maxVal - minVal)) * chartH;
      ctx.fillStyle = '#4ade80';
      ctx.beginPath();
      ctx.arc(lastX, lastY, 3, 0, Math.PI * 2);
      ctx.fill();
    }

    // X-axis label
    ctx.textAlign = 'left';
    ctx.fillText('Run 1', padding.left, height - 8);
    ctx.textAlign = 'right';
    ctx.fillText(`Run ${dataPoints.length}`, padding.left + chartW, height - 8);
  }

  function escapeHtml(str) {
    if (!str) return '';
    return String(str)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#039;');
  }

  // Initial fetch and auto-refresh every 8s
  fetchDashboardData();
  setInterval(fetchDashboardData, 8000);

  window.addEventListener('resize', () => {
    fetchDashboardData();
  });
})();
