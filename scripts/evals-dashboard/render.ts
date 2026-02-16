import type { ReportData } from "./types.ts";

export function renderDashboard(report: ReportData): string {
  const models = [...new Set(report.rows.map((r) => r.model))].sort();
  const modelOptions = models.map((m) => `<option value="${escapeHtml(m)}">${escapeHtml(m)}</option>`).join("");

  const rowsJson = JSON.stringify(
    report.rows.map((row) => ({
      folder: row.folder,
      prompt: row.prompt,
      promptNumber: row.promptNumber,
      model: row.model,
      success: row.success,
      durationSeconds: row.durationSeconds,
      completedAt: row.completedAt,
      completedAtEpoch: row.completedAtEpoch,
      costUsd: row.costUsd,
      error: row.error,
      hasPreview: !!row.previewPath,
      hasScript: !!row.scriptPath,
    }))
  ).replaceAll("<", "\\u003c").replaceAll(">", "\\u003e");

  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>High-Evals Dashboard</title>
  <link rel="preconnect" href="https://fonts.googleapis.com" />
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
  <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;600;700&family=Space+Grotesk:wght@400;500;600;700&display=swap" rel="stylesheet" />
  <style>
    :root {
      --bg: #0f0f1a;
      --fg: #e2e2f5;
      --card: #1a1a2e;
      --card-fg: #e2e2f5;
      --primary: #a48fff;
      --primary-dim: rgba(164, 143, 255, 0.15);
      --secondary: #2d2b55;
      --secondary-fg: #c4c2ff;
      --muted: #222244;
      --muted-fg: #a0a0c0;
      --accent: #303060;
      --accent-fg: #e2e2f5;
      --border: #303052;
      --ring: #a48fff;
      --ok: #4db6ac;
      --ok-dim: rgba(77, 182, 172, 0.15);
      --python: #64b5f6;
      --python-dim: rgba(100, 181, 246, 0.15);
      --fail: #ff5470;
      --fail-dim: rgba(255, 84, 112, 0.15);
      --chart-1: #a48fff;
      --chart-2: #7986cb;
      --chart-3: #64b5f6;
      --chart-4: #4db6ac;
      --chart-5: #ff79c6;
    }

    * { box-sizing: border-box; margin: 0; padding: 0; }

    body {
      font-family: "JetBrains Mono", monospace;
      color: var(--fg);
      background: var(--bg);
      min-height: 100vh;
      padding: 32px 24px;
      position: relative;
      overflow-x: hidden;
    }

    body::before {
      content: "";
      position: fixed;
      top: -40%;
      left: -20%;
      width: 80vw;
      height: 80vw;
      border-radius: 50%;
      background: radial-gradient(circle, rgba(164,143,255,0.07) 0%, transparent 70%);
      pointer-events: none;
      z-index: 0;
    }

    body::after {
      content: "";
      position: fixed;
      bottom: -30%;
      right: -15%;
      width: 60vw;
      height: 60vw;
      border-radius: 50%;
      background: radial-gradient(circle, rgba(255,121,198,0.05) 0%, transparent 70%);
      pointer-events: none;
      z-index: 0;
    }

    .wrap {
      max-width: 1320px;
      margin: 0 auto;
      display: grid;
      gap: 20px;
      position: relative;
      z-index: 1;
    }

    .hero {
      background: linear-gradient(135deg, #1a1a2e 0%, #222244 50%, #1a1a2e 100%);
      border: 1px solid var(--border);
      border-radius: 16px;
      padding: 32px 28px;
      position: relative;
      overflow: hidden;
    }

    .hero::before {
      content: "";
      position: absolute;
      top: 0;
      left: 0;
      right: 0;
      height: 2px;
      background: linear-gradient(90deg, transparent, var(--primary), var(--chart-5), transparent);
    }

    .hero::after {
      content: "";
      position: absolute;
      top: -60px;
      right: -40px;
      width: 200px;
      height: 200px;
      border-radius: 50%;
      background: radial-gradient(circle, rgba(164,143,255,0.12) 0%, transparent 70%);
      pointer-events: none;
    }

    h1 {
      font-family: "Space Grotesk", sans-serif;
      font-size: clamp(1.8rem, 3vw, 2.6rem);
      font-weight: 700;
      line-height: 1.1;
      letter-spacing: -0.02em;
      background: linear-gradient(135deg, #e2e2f5 0%, #a48fff 50%, #ff79c6 100%);
      -webkit-background-clip: text;
      -webkit-text-fill-color: transparent;
      background-clip: text;
    }

    .subtitle {
      margin-top: 10px;
      color: var(--muted-fg);
      font-size: 0.82rem;
      letter-spacing: 0.01em;
    }

    .metrics {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(185px, 1fr));
      gap: 14px;
    }

    .metric {
      background: var(--card);
      border: 1px solid var(--border);
      border-radius: 12px;
      padding: 18px 16px;
      position: relative;
      overflow: hidden;
      transition: border-color 0.2s ease, transform 0.15s ease;
    }

    .metric:hover {
      border-color: var(--primary);
      transform: translateY(-2px);
    }

    .metric::before {
      content: "";
      position: absolute;
      top: 0;
      left: 0;
      right: 0;
      height: 1px;
      background: linear-gradient(90deg, transparent, var(--primary), transparent);
      opacity: 0;
      transition: opacity 0.2s ease;
    }

    .metric:hover::before {
      opacity: 1;
    }

    .metric::after {
      content: "";
      position: absolute;
      bottom: -40px;
      right: -30px;
      width: 100px;
      height: 100px;
      border-radius: 50%;
      background: var(--primary-dim);
      pointer-events: none;
    }

    .metric-label {
      font-size: 0.68rem;
      letter-spacing: 0.1em;
      text-transform: uppercase;
      color: var(--muted-fg);
      margin-bottom: 10px;
      font-weight: 500;
    }

    .metric-value {
      font-family: "Space Grotesk", sans-serif;
      font-weight: 700;
      font-size: 1.4rem;
      color: var(--fg);
      position: relative;
      z-index: 1;
    }

    .table-card {
      background: var(--card);
      border: 1px solid var(--border);
      border-radius: 14px;
      overflow: hidden;
    }

    .table-wrap {
      overflow: auto;
    }

    .table-wrap::-webkit-scrollbar {
      height: 6px;
    }

    .table-wrap::-webkit-scrollbar-track {
      background: var(--muted);
    }

    .table-wrap::-webkit-scrollbar-thumb {
      background: var(--border);
      border-radius: 3px;
    }

    table {
      width: 100%;
      border-collapse: collapse;
      min-width: 1060px;
    }

    thead {
      background: var(--secondary);
    }

    th {
      text-align: left;
      padding: 12px 14px;
      font-size: 0.68rem;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      color: var(--secondary-fg);
      font-weight: 600;
      border-bottom: 1px solid var(--border);
    }

    td {
      text-align: left;
      padding: 12px 14px;
      border-bottom: 1px solid rgba(48, 48, 82, 0.5);
      vertical-align: top;
      font-size: 0.82rem;
      color: var(--fg);
    }

    tbody tr {
      transition: background 0.15s ease;
    }

    tbody tr:hover {
      background: rgba(164, 143, 255, 0.05);
    }

    .tag {
      display: inline-block;
      padding: 3px 9px;
      border-radius: 6px;
      border: 1px solid var(--border);
      background: var(--accent);
      color: var(--accent-fg);
      font-size: 0.75rem;
      font-weight: 600;
    }

    .prompt-col {
      max-width: 300px;
      line-height: 1.4;
      color: var(--muted-fg);
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      cursor: default;
    }

    .folder-col {
      color: var(--muted-fg);
      font-size: 0.72rem;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      max-width: 220px;
      cursor: default;
    }

    .actions-col {
      white-space: nowrap;
    }

    .filters {
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
      align-items: center;
      padding: 14px 16px;
      border-bottom: 1px solid var(--border);
      background: var(--secondary);
    }

    .filter-group {
      display: flex;
      align-items: center;
      gap: 6px;
    }

    .filter-label {
      font-size: 0.65rem;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      color: var(--muted-fg);
      font-weight: 500;
    }

    .filter-select, .filter-input {
      background: var(--card);
      border: 1px solid var(--border);
      color: var(--fg);
      border-radius: 6px;
      padding: 5px 10px;
      font-family: inherit;
      font-size: 0.75rem;
      outline: none;
      transition: border-color 0.15s ease;
    }

    .filter-select:focus, .filter-input:focus {
      border-color: var(--primary);
    }

    .filter-input {
      width: 160px;
    }

    .filter-reset {
      background: none;
      border: 1px solid var(--border);
      color: var(--muted-fg);
      border-radius: 6px;
      padding: 5px 10px;
      font-family: inherit;
      font-size: 0.72rem;
      cursor: pointer;
      transition: color 0.15s ease, border-color 0.15s ease;
      margin-left: auto;
    }

    .filter-reset:hover {
      color: var(--fg);
      border-color: var(--fg);
    }

    .filter-count {
      font-size: 0.72rem;
      color: var(--muted-fg);
    }

    th.sortable {
      cursor: pointer;
      user-select: none;
      position: relative;
    }

    th.sortable:hover {
      color: var(--fg);
    }

    th .sort-arrow {
      margin-left: 4px;
      font-size: 0.6rem;
      opacity: 0.3;
    }

    th.sort-asc .sort-arrow,
    th.sort-desc .sort-arrow {
      opacity: 1;
      color: var(--primary);
    }

    .tooltip {
      position: fixed;
      z-index: 9999;
      max-width: 450px;
      padding: 10px 14px;
      background: var(--secondary);
      border: 1px solid var(--border);
      border-radius: 8px;
      color: var(--fg);
      font-size: 0.76rem;
      line-height: 1.45;
      pointer-events: none;
      white-space: pre-wrap;
      word-break: break-word;
      box-shadow: 0 8px 24px rgba(0,0,0,0.4);
      opacity: 0;
      transform: translateY(4px);
      transition: opacity 0.15s ease, transform 0.15s ease;
    }

    .tooltip.visible {
      opacity: 1;
      transform: translateY(0);
    }

    .status {
      display: inline-flex;
      align-items: center;
      gap: 5px;
      border-radius: 6px;
      padding: 3px 10px;
      font-weight: 700;
      font-size: 0.72rem;
      letter-spacing: 0.03em;
      text-transform: uppercase;
    }

    .status-wrap {
      display: inline-flex;
      align-items: center;
      gap: 6px;
    }

    .info-btn {
      width: 18px;
      height: 18px;
      border-radius: 999px;
      border: 1px solid rgba(100, 181, 246, 0.5);
      background: rgba(100, 181, 246, 0.15);
      color: var(--python);
      font-family: "JetBrains Mono", monospace;
      font-size: 0.66rem;
      font-weight: 700;
      line-height: 1;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      cursor: pointer;
      transition: background 0.15s ease, border-color 0.15s ease, transform 0.1s ease;
    }

    .info-btn:hover {
      background: rgba(100, 181, 246, 0.26);
      border-color: var(--python);
      transform: translateY(-1px);
    }

    .info-btn:active {
      transform: translateY(0);
    }

    .info-btn:focus-visible {
      outline: 2px solid var(--python);
      outline-offset: 1px;
    }

    .status-ok {
      background: var(--ok-dim);
      color: var(--ok);
      border: 1px solid rgba(77, 182, 172, 0.3);
    }

    .status-fail {
      background: var(--fail-dim);
      color: var(--fail);
      border: 1px solid rgba(255, 84, 112, 0.3);
    }

    .empty {
      padding: 48px 24px;
      text-align: center;
      color: var(--muted-fg);
      font-size: 0.88rem;
    }

    .footer-note {
      color: var(--muted-fg);
      font-size: 0.7rem;
      margin-top: 4px;
      opacity: 0.7;
    }

    .action-btn {
      border-radius: 6px;
      width: 78px;
      height: 28px;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      font-family: inherit;
      font-size: 0.72rem;
      font-weight: 600;
      letter-spacing: 0.03em;
      cursor: pointer;
      transition: background 0.15s ease, border-color 0.15s ease;
    }

    .preview-btn {
      background: var(--primary-dim);
      color: var(--primary);
      border: 1px solid rgba(164, 143, 255, 0.3);
    }

    .preview-btn:hover {
      background: rgba(164, 143, 255, 0.25);
      border-color: var(--primary);
    }

    .run-btn {
      background: var(--python-dim);
      color: var(--python);
      border: 1px solid rgba(100, 181, 246, 0.35);
    }

    .run-btn:hover {
      background: rgba(100, 181, 246, 0.25);
      border-color: var(--python);
    }

    .run-btn.running {
      opacity: 0.5;
      pointer-events: none;
    }

    .pick-btn {
      background: rgba(160, 160, 192, 0.12);
      color: var(--muted-fg);
      border: 1px solid var(--border);
      border-radius: 6px;
      width: 52px;
      height: 28px;
      font-family: inherit;
      font-size: 0.7rem;
      font-weight: 600;
      letter-spacing: 0.02em;
      cursor: pointer;
      transition: background 0.15s ease, border-color 0.15s ease, color 0.15s ease;
    }

    .pick-btn:hover {
      background: rgba(160, 160, 192, 0.22);
      border-color: var(--muted-fg);
      color: var(--fg);
    }

    .no-preview {
      color: var(--muted-fg);
      opacity: 0.4;
    }

    .preview-overlay {
      position: fixed;
      inset: 0;
      background: rgba(15, 15, 26, 0.85);
      backdrop-filter: blur(8px);
      z-index: 1000;
      display: flex;
      align-items: center;
      justify-content: center;
      opacity: 0;
      pointer-events: none;
      transition: opacity 0.25s ease;
    }

    .preview-overlay.active {
      opacity: 1;
      pointer-events: all;
    }

    .preview-modal {
      width: 92vw;
      height: 88vh;
      max-width: 1400px;
      background: var(--card);
      border: 1px solid var(--border);
      border-radius: 14px;
      overflow: hidden;
      display: flex;
      flex-direction: column;
      transform: scale(0.95) translateY(10px);
      transition: transform 0.25s ease;
    }

    .preview-overlay.active .preview-modal {
      transform: scale(1) translateY(0);
    }

    .preview-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 12px 18px;
      border-bottom: 1px solid var(--border);
      background: var(--secondary);
      flex-shrink: 0;
    }

    .preview-title {
      font-size: 0.78rem;
      font-weight: 600;
      color: var(--secondary-fg);
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
      max-width: 60%;
    }

    .preview-actions {
      display: flex;
      align-items: center;
      gap: 12px;
    }

    .preview-open {
      color: var(--primary);
      font-size: 0.72rem;
      font-weight: 600;
      text-decoration: none;
      font-family: inherit;
    }

    .preview-open:hover {
      text-decoration: underline;
    }

    .preview-close {
      background: var(--accent);
      border: 1px solid var(--border);
      color: var(--fg);
      width: 28px;
      height: 28px;
      border-radius: 6px;
      font-size: 0.9rem;
      cursor: pointer;
      display: flex;
      align-items: center;
      justify-content: center;
      transition: background 0.15s ease;
    }

    .preview-close:hover {
      background: var(--fail-dim);
      border-color: rgba(255, 84, 112, 0.3);
      color: var(--fail);
    }

    .preview-iframe {
      flex: 1;
      width: 100%;
      border: none;
      background: #fff;
    }

    .run-modal {
      max-width: 900px;
      height: 70vh;
    }

    .run-output {
      flex: 1;
      margin: 0;
      padding: 18px;
      overflow: auto;
      font-family: "JetBrains Mono", monospace;
      font-size: 0.78rem;
      line-height: 1.5;
      color: var(--fg);
      background: var(--bg);
      white-space: pre-wrap;
      word-break: break-all;
    }

    @keyframes fadeIn {
      from { opacity: 0; transform: translateY(8px); }
      to { opacity: 1; transform: translateY(0); }
    }

    .hero, .metrics, .table-card, .footer-note {
      animation: fadeIn 0.4s ease-out both;
    }
    .metrics { animation-delay: 0.05s; }
    .table-card { animation-delay: 0.1s; }
    .footer-note { animation-delay: 0.15s; }
  </style>
</head>
<body>
  <main class="wrap">
    <section class="hero">
      <h1>High-Evals Runboard</h1>
      <div class="subtitle">Live summary of local eval folders, runtime, and available cost metadata.</div>
    </section>

    <section class="metrics">
      <article class="metric">
        <div class="metric-label">Total Runs</div>
        <div class="metric-value">${report.totalEvals}</div>
      </article>
      <article class="metric">
        <div class="metric-label">Success Rate</div>
        <div class="metric-value">${formatPercent(report.successRate)}</div>
      </article>
      <article class="metric">
        <div class="metric-label">Total Runtime</div>
        <div class="metric-value">${formatDuration(report.totalDurationSeconds)}</div>
      </article>
      <article class="metric">
        <div class="metric-label">Avg Runtime</div>
        <div class="metric-value">${formatDuration(Math.round(report.averageDurationSeconds))}</div>
      </article>
      <article class="metric">
        <div class="metric-label">Known Cost</div>
        <div class="metric-value">${formatCost(report.totalKnownCostUsd)}</div>
      </article>
      <article class="metric">
        <div class="metric-label">Cost Coverage</div>
        <div class="metric-value">${report.knownCostCount}/${report.totalEvals}</div>
      </article>
    </section>

    <section class="table-card">
      <div class="filters" id="filtersBar">
        <div class="filter-group">
          <span class="filter-label">Model</span>
          <select class="filter-select" id="filterModel" onchange="applyFilters()">
            <option value="">All</option>
            ${modelOptions}
          </select>
        </div>
        <div class="filter-group">
          <span class="filter-label">Status</span>
          <select class="filter-select" id="filterStatus" onchange="applyFilters()">
            <option value="">All</option>
            <option value="success">Success</option>
            <option value="failed">Failed</option>
          </select>
        </div>
        <div class="filter-group">
          <span class="filter-label">Search</span>
          <input class="filter-input" id="filterSearch" type="text" placeholder="prompt, folder…" oninput="applyFilters()" />
        </div>
        <span class="filter-count" id="filterCount"></span>
        <button class="filter-reset" onclick="resetFilters()">Reset</button>
      </div>
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Prompt</th>
              <th>Prompt Preview</th>
              <th class="sortable" data-sort="model" onclick="toggleSort('model')">Model<span class="sort-arrow">↕</span></th>
              <th class="sortable" data-sort="duration" onclick="toggleSort('duration')">Runtime<span class="sort-arrow">↕</span></th>
              <th class="sortable" data-sort="cost" onclick="toggleSort('cost')">Cost<span class="sort-arrow">↕</span></th>
              <th class="sortable" data-sort="status" onclick="toggleSort('status')">Status<span class="sort-arrow">↕</span></th>
              <th class="sortable" data-sort="date" onclick="toggleSort('date')">Completed<span class="sort-arrow">↕</span></th>
              <th>Folder</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody id="evalsBody"></tbody>
        </table>
      </div>
    </section>

    <div class="footer-note">Cost values are shown when present in result metadata fields: cost_usd, total_cost, or cost.</div>
  </main>

  <div class="preview-overlay" id="previewOverlay" onclick="closePreview(event)">
    <div class="preview-modal">
      <div class="preview-header">
        <span class="preview-title" id="previewTitle"></span>
        <div class="preview-actions">
          <a class="preview-open" id="previewOpenLink" href="#" target="_blank">Open in tab ↗</a>
          <button class="preview-close" onclick="closePreview()">✕</button>
        </div>
      </div>
      <iframe class="preview-iframe" id="previewIframe" sandbox="allow-scripts allow-same-origin"></iframe>
    </div>
  </div>

  <div class="preview-overlay" id="runOverlay" onclick="closeRunOutput(event)">
    <div class="preview-modal run-modal">
      <div class="preview-header">
        <span class="preview-title" id="runTitle"></span>
        <div class="preview-actions">
          <button class="preview-close" onclick="closeRunOutput()">✕</button>
        </div>
      </div>
      <pre class="run-output" id="runOutput"></pre>
    </div>
  </div>

  <div class="tooltip" id="tooltip"></div>

  <script>
    var ALL_ROWS = ${rowsJson};
    var currentSort = { key: 'date', dir: 'desc' };

    function esc(s) {
      var d = document.createElement('div');
      d.textContent = s;
      return d.innerHTML;
    }

    function fmtDuration(s) {
      if (!s || s <= 0) return '0s';
      s = Math.floor(s);
      var h = Math.floor(s / 3600), m = Math.floor((s % 3600) / 60), r = s % 60;
      if (h > 0) return h + 'h ' + m + 'm ' + r + 's';
      if (m > 0) return m + 'm ' + r + 's';
      return r + 's';
    }

    function fmtCost(c) {
      if (c === null || c === undefined) return 'N/A';
      return '$' + c.toFixed(4);
    }

    function fmtDate(s) {
      if (!s) return '-';
      var d = new Date(s);
      return isNaN(d.getTime()) ? s : d.toLocaleString();
    }

    function getFiltered() {
      var model = document.getElementById('filterModel').value;
      var status = document.getElementById('filterStatus').value;
      var search = document.getElementById('filterSearch').value.toLowerCase().trim();
      return ALL_ROWS.filter(function(r) {
        if (model && r.model !== model) return false;
        if (status === 'success' && !r.success) return false;
        if (status === 'failed' && r.success) return false;
        if (search && r.prompt.toLowerCase().indexOf(search) === -1 && r.folder.toLowerCase().indexOf(search) === -1 && r.model.toLowerCase().indexOf(search) === -1) return false;
        return true;
      });
    }

    function getSorted(rows) {
      var k = currentSort.key, d = currentSort.dir === 'asc' ? 1 : -1;
      return rows.slice().sort(function(a, b) {
        var av, bv;
        if (k === 'model') { av = a.model.toLowerCase(); bv = b.model.toLowerCase(); return av < bv ? -d : av > bv ? d : 0; }
        if (k === 'duration') return (a.durationSeconds - b.durationSeconds) * d;
        if (k === 'cost') { av = (a.costUsd === null || a.costUsd === undefined) ? -1 : a.costUsd; bv = (b.costUsd === null || b.costUsd === undefined) ? -1 : b.costUsd; return (av - bv) * d; }
        if (k === 'status') { av = a.success ? 1 : 0; bv = b.success ? 1 : 0; return (av - bv) * d; }
        if (k === 'date') { av = a.completedAtEpoch || 0; bv = b.completedAtEpoch || 0; return (av - bv) * d; }
        return 0;
      });
    }

    function buildActions(row) {
      var el = document.createElement('td');
      el.className = 'actions-col';
      if (row.hasPreview) {
        var pb = document.createElement('button');
        pb.className = 'action-btn preview-btn';
        pb.textContent = 'Preview';
        pb.onclick = function() { openPreview('/preview/' + row.folder + '/', row.folder); };
        el.appendChild(pb);
      }
      if (row.hasScript) {
        if (row.hasPreview) el.appendChild(document.createTextNode(' '));
        var rb = document.createElement('button');
        rb.className = 'action-btn run-btn';
        rb.textContent = 'Run';
        rb.onclick = function() { runScript(row.folder, rb); };
        el.appendChild(rb);
        el.appendChild(document.createTextNode(' '));
        var cb = document.createElement('button');
        cb.className = 'pick-btn';
        cb.textContent = 'Pick';
        cb.onclick = function() { chooseRunTarget(row.folder, rb); };
        el.appendChild(cb);
      }
      if (!row.hasPreview && !row.hasScript) {
        el.innerHTML = '<span class="no-preview">\u2014</span>';
      }
      return el;
    }

    function renderRows() {
      var filtered = getFiltered();
      var sorted = getSorted(filtered);
      var body = document.getElementById('evalsBody');
      document.getElementById('filterCount').textContent = sorted.length + ' / ' + ALL_ROWS.length;
      body.innerHTML = '';

      if (sorted.length === 0) {
        body.innerHTML = '<tr><td colspan="9" class="empty">No matching results.</td></tr>';
        return;
      }

      for (var i = 0; i < sorted.length; i++) {
        var row = sorted[i];
        var tr = document.createElement('tr');
        var tag = row.promptNumber === null ? 'p?' : 'p' + row.promptNumber;
        var sc = row.success ? 'status-ok' : 'status-fail';
        var sl = row.success ? 'Success' : 'Failed';
        var statusInfo = row.error
          ? '<button class="info-btn" type="button" aria-label="Show status details" data-info="' + esc(row.error) + '" onclick="showInfo(event, this)">i</button>'
          : '';

        tr.innerHTML =
          '<td><span class="tag">' + esc(tag) + '</span></td>' +
          '<td class="prompt-col" data-tip="' + esc(row.prompt) + '">' + esc(row.prompt) + '</td>' +
          '<td>' + esc(row.model) + '</td>' +
          '<td>' + fmtDuration(row.durationSeconds) + '</td>' +
          '<td>' + fmtCost(row.costUsd) + '</td>' +
          '<td><span class="status-wrap"><span class="status ' + sc + '">' + sl + '</span>' + statusInfo + '</span></td>' +
          '<td>' + esc(fmtDate(row.completedAt)) + '</td>' +
          '<td class="folder-col" data-tip="' + esc(row.folder) + '">' + esc(row.folder) + '</td>';
        tr.appendChild(buildActions(row));
        body.appendChild(tr);
      }
    }

    function applyFilters() { renderRows(); updateSortHeaders(); }

    function resetFilters() {
      document.getElementById('filterModel').value = '';
      document.getElementById('filterStatus').value = '';
      document.getElementById('filterSearch').value = '';
      applyFilters();
    }

    function toggleSort(key) {
      if (currentSort.key === key) {
        currentSort.dir = currentSort.dir === 'asc' ? 'desc' : 'asc';
      } else {
        currentSort.key = key;
        currentSort.dir = key === 'date' ? 'desc' : 'asc';
      }
      applyFilters();
    }

    function updateSortHeaders() {
      document.querySelectorAll('th.sortable').forEach(function(th) {
        th.classList.remove('sort-asc', 'sort-desc');
        var k = th.getAttribute('data-sort');
        if (k === currentSort.key) {
          th.classList.add(currentSort.dir === 'asc' ? 'sort-asc' : 'sort-desc');
          th.querySelector('.sort-arrow').textContent = currentSort.dir === 'asc' ? '↑' : '↓';
        } else {
          th.querySelector('.sort-arrow').textContent = '↕';
        }
      });
    }

    // Tooltip
    var tip = document.getElementById('tooltip');
    var activeInfoBtn = null;
    function positionTooltip(el) {
      var rect = el.getBoundingClientRect();
      var tipW = tip.offsetWidth, tipH = tip.offsetHeight;
      var left = rect.left + (rect.width / 2) - (tipW / 2);
      var top = rect.bottom + 8;
      if (left + tipW > window.innerWidth - 12) left = window.innerWidth - tipW - 12;
      if (left < 8) left = 8;
      if (top + tipH > window.innerHeight - 12) top = rect.top - tipH - 8;
      tip.style.left = left + 'px';
      tip.style.top = top + 'px';
    }
    function showInfo(e, btn) {
      if (e) e.stopPropagation();
      var text = btn.getAttribute('data-info');
      if (!text) return;
      if (activeInfoBtn === btn && tip.classList.contains('visible')) {
        hideInfo();
        return;
      }
      activeInfoBtn = btn;
      tip.textContent = text;
      tip.classList.add('visible');
      positionTooltip(btn);
    }
    function hideInfo() {
      tip.classList.remove('visible');
      activeInfoBtn = null;
    }
    document.addEventListener('click', function(e) {
      if (!e.target.closest('.info-btn')) hideInfo();
    });
    window.addEventListener('resize', function() {
      if (activeInfoBtn && tip.classList.contains('visible')) positionTooltip(activeInfoBtn);
    });
    document.addEventListener('mouseover', function(e) {
      if (activeInfoBtn) return;
      var el = e.target.closest('[data-tip]');
      if (!el) { tip.classList.remove('visible'); return; }
      var text = el.getAttribute('data-tip');
      if (!text || text === el.textContent) { tip.classList.remove('visible'); return; }
      tip.textContent = text;
      tip.classList.add('visible');
      var rect = el.getBoundingClientRect();
      var tipW = tip.offsetWidth, tipH = tip.offsetHeight;
      var left = rect.left;
      var top = rect.bottom + 6;
      if (left + tipW > window.innerWidth - 12) left = window.innerWidth - tipW - 12;
      if (left < 8) left = 8;
      if (top + tipH > window.innerHeight - 12) top = rect.top - tipH - 6;
      tip.style.left = left + 'px';
      tip.style.top = top + 'px';
    });
    document.addEventListener('mouseout', function(e) {
      if (activeInfoBtn) return;
      var el = e.target.closest('[data-tip]');
      if (el) tip.classList.remove('visible');
    });

    // Preview modal
    function openPreview(url, folder) {
      document.getElementById('previewIframe').src = url;
      document.getElementById('previewTitle').textContent = folder;
      document.getElementById('previewOpenLink').href = url;
      document.getElementById('previewOverlay').classList.add('active');
      document.body.style.overflow = 'hidden';
    }
    function closePreview(e) {
      if (e && e.target !== e.currentTarget) return;
      document.getElementById('previewOverlay').classList.remove('active');
      document.getElementById('previewIframe').src = 'about:blank';
      document.body.style.overflow = '';
    }

    // Run script
    async function chooseRunTarget(folder, runBtn) {
      try {
        var optionsRes = await fetch('/api/run-options/' + encodeURIComponent(folder) + '/');
        var optionsData = await optionsRes.json();
        if (!optionsRes.ok || !optionsData.targets || optionsData.targets.length === 0) {
          alert('No runnable Python targets found for this folder.');
          return;
        }

        var defaultIndex = optionsData.defaultTarget ? optionsData.targets.indexOf(optionsData.defaultTarget) + 1 : 1;
        if (!defaultIndex || defaultIndex < 1) defaultIndex = 1;
        var listText = optionsData.targets.map(function(target, index) {
          return (index + 1) + '. ' + target;
        }).join('\\n');
        var answer = prompt(
          'Choose a Python target to run for "' + folder + '":\\n\\n' + listText + '\\n\\nEnter number (' + defaultIndex + ' default):',
          String(defaultIndex)
        );
        if (answer === null) return;

        var trimmed = answer.trim();
        var selectedNumber = Number.parseInt(trimmed === '' ? String(defaultIndex) : trimmed, 10);
        if (!Number.isInteger(selectedNumber) || selectedNumber < 1 || selectedNumber > optionsData.targets.length) {
          alert('Invalid selection.');
          return;
        }

        await runScript(folder, runBtn, optionsData.targets[selectedNumber - 1]);
      } catch (err) {
        alert('Failed to load run options: ' + err.message);
      }
    }

    async function runScript(folder, btn, target) {
      btn.classList.add('running');
      btn.textContent = 'Running…';
      try {
        var query = target ? ('?target=' + encodeURIComponent(target)) : '';
        var res = await fetch('/run/' + encodeURIComponent(folder) + '/' + query);
        var data = await res.json();
        document.getElementById('runTitle').textContent = (data.ok ? '✓ ' : '✗ ') + folder;
        document.getElementById('runOutput').textContent = data.output;
        document.getElementById('runOverlay').classList.add('active');
        document.body.style.overflow = 'hidden';
      } catch (err) {
        alert('Run failed: ' + err.message);
      } finally {
        btn.classList.remove('running');
        btn.textContent = 'Run';
      }
    }
    function closeRunOutput(e) {
      if (e && e.target !== e.currentTarget) return;
      document.getElementById('runOverlay').classList.remove('active');
      document.body.style.overflow = '';
    }

    document.addEventListener('keydown', function(e) {
      if (e.key === 'Escape') { closePreview(); closeRunOutput(); hideInfo(); }
    });

    // Initial render
    renderRows();
    updateSortHeaders();
  </script>
</body>
</html>`;
}

function formatPercent(value: number): string {
  if (!Number.isFinite(value)) {
    return "0.0%";
  }
  return `${value.toFixed(1)}%`;
}

function formatDuration(totalSeconds: number): string {
  if (!Number.isFinite(totalSeconds) || totalSeconds <= 0) {
    return "0s";
  }

  const seconds = Math.floor(totalSeconds);
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const remaining = seconds % 60;

  if (hours > 0) {
    return `${hours}h ${minutes}m ${remaining}s`;
  }
  if (minutes > 0) {
    return `${minutes}m ${remaining}s`;
  }
  return `${remaining}s`;
}

function formatCost(costUsd: number | null): string {
  if (costUsd === null || !Number.isFinite(costUsd)) {
    return "N/A";
  }
  return `$${costUsd.toFixed(4)}`;
}

function formatDate(input: string): string {
  const date = new Date(input);
  if (Number.isNaN(date.getTime())) {
    return input;
  }
  return date.toLocaleString();
}

function escapeHtml(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}
