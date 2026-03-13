import type { ReportData } from "./types.ts";

export function renderDashboard(report: ReportData): string {
  const models = [...new Set(report.rows.map((row) => row.model))].sort();
  const tracks = [...new Set(report.rows.map((row) => row.track))].sort();

  const rowsJson = JSON.stringify(report.rows).replaceAll("<", "\\u003c").replaceAll(">", "\\u003e");
  const modelOptions = models.map((model) => `<option value="${escapeHtml(model)}">${escapeHtml(model)}</option>`).join("");
  const trackOptions = tracks.map((track) => `<option value="${escapeHtml(track)}">${escapeHtml(track)}</option>`).join("");

  return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>High-Evals Dashboard</title>
  <link rel="preconnect" href="https://fonts.googleapis.com" />
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin />
  <link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Sans:wght@400;500;600&family=IBM+Plex+Mono:wght@400;500&display=swap" rel="stylesheet" />
  <style>
    :root {
      --bg: #f5f1e8;
      --panel: rgba(255, 252, 246, 0.9);
      --panel-strong: #fffaf2;
      --border: rgba(66, 56, 34, 0.16);
      --border-strong: rgba(66, 56, 34, 0.28);
      --text: #2c2417;
      --muted: #74664b;
      --accent: #9b5d24;
      --accent-soft: rgba(155, 93, 36, 0.12);
      --ok: #1f7a4a;
      --ok-soft: rgba(31, 122, 74, 0.12);
      --warn: #9b5d24;
      --warn-soft: rgba(155, 93, 36, 0.12);
      --bad: #b2412d;
      --bad-soft: rgba(178, 65, 45, 0.12);
      --shadow: 0 18px 60px rgba(78, 54, 17, 0.08);
      --radius: 18px;
      --sans: "IBM Plex Sans", system-ui, sans-serif;
      --mono: "IBM Plex Mono", ui-monospace, monospace;
    }

    * { box-sizing: border-box; }
    html, body { margin: 0; min-height: 100%; }

    body {
      font-family: var(--sans);
      color: var(--text);
      background:
        radial-gradient(circle at top left, rgba(155, 93, 36, 0.12), transparent 32%),
        radial-gradient(circle at top right, rgba(31, 122, 74, 0.08), transparent 26%),
        linear-gradient(180deg, #faf6ef 0%, var(--bg) 100%);
    }

    main {
      max-width: 1480px;
      margin: 0 auto;
      padding: 40px 24px 56px;
      display: grid;
      gap: 18px;
    }

    .hero, .metrics, .table-card {
      background: var(--panel);
      backdrop-filter: blur(16px);
      border: 1px solid var(--border);
      border-radius: var(--radius);
      box-shadow: var(--shadow);
    }

    .hero {
      padding: 26px 28px;
      display: flex;
      flex-wrap: wrap;
      justify-content: space-between;
      gap: 18px;
      align-items: end;
    }

    .eyebrow {
      font-size: 0.76rem;
      letter-spacing: 0.12em;
      text-transform: uppercase;
      color: var(--accent);
      font-weight: 600;
      margin-bottom: 8px;
    }

    h1 {
      margin: 0;
      font-size: clamp(1.7rem, 2vw, 2.4rem);
      line-height: 1.05;
      letter-spacing: -0.04em;
    }

    .hero-copy {
      max-width: 760px;
      color: var(--muted);
      font-size: 0.98rem;
      line-height: 1.5;
    }

    .hero-notes {
      display: flex;
      gap: 10px;
      flex-wrap: wrap;
      justify-content: flex-end;
      align-items: center;
    }

    .chip {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 6px 10px;
      border-radius: 999px;
      background: var(--panel-strong);
      border: 1px solid var(--border);
      color: var(--muted);
      font-size: 0.78rem;
      font-weight: 500;
    }

    .metrics {
      padding: 10px;
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
      gap: 10px;
    }

    .metric {
      background: var(--panel-strong);
      border: 1px solid var(--border);
      border-radius: 14px;
      padding: 14px 16px;
    }

    .metric-label {
      color: var(--muted);
      font-size: 0.72rem;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      font-weight: 600;
      margin-bottom: 8px;
    }

    .metric-value {
      font-family: var(--mono);
      font-size: 1.15rem;
      font-weight: 500;
    }

    .table-card {
      overflow: hidden;
    }

    .toolbar {
      display: flex;
      flex-wrap: wrap;
      gap: 12px;
      align-items: end;
      padding: 16px 18px;
      border-bottom: 1px solid var(--border);
      background: rgba(255, 250, 242, 0.72);
    }

    .filter {
      display: grid;
      gap: 6px;
    }

    .filter label {
      color: var(--muted);
      font-size: 0.72rem;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      font-weight: 600;
    }

    .filter select,
    .filter input,
    .toolbar button {
      min-height: 38px;
      border-radius: 10px;
      border: 1px solid var(--border);
      background: var(--panel-strong);
      color: var(--text);
      font: inherit;
      padding: 0 12px;
    }

    .filter input {
      min-width: 220px;
    }

    .toolbar button {
      cursor: pointer;
      font-weight: 600;
      transition: border-color 0.15s ease, transform 0.15s ease;
    }

    .toolbar button:hover,
    .action-btn:hover,
    .overlay-header button:hover {
      border-color: var(--border-strong);
      transform: translateY(-1px);
    }

    .toolbar-spacer {
      flex: 1 1 auto;
    }

    .table-wrap {
      overflow: auto;
    }

    table {
      width: 100%;
      min-width: 1220px;
      border-collapse: collapse;
    }

    thead {
      background: rgba(255, 250, 242, 0.86);
    }

    th, td {
      text-align: left;
      padding: 13px 16px;
      border-bottom: 1px solid var(--border);
      vertical-align: top;
    }

    th {
      color: var(--muted);
      font-size: 0.72rem;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      font-weight: 600;
      white-space: nowrap;
    }

    tbody tr:hover {
      background: rgba(155, 93, 36, 0.035);
    }

    .prompt-title {
      font-weight: 600;
      margin-bottom: 4px;
      line-height: 1.35;
    }

    .prompt-meta,
    .folder-text,
    .error-text {
      color: var(--muted);
      font-size: 0.82rem;
      line-height: 1.45;
    }

    .folder-text,
    .mono {
      font-family: var(--mono);
      font-size: 0.74rem;
    }

    .status-stack {
      display: flex;
      flex-wrap: wrap;
      gap: 6px;
      align-items: center;
    }

    .status {
      display: inline-flex;
      align-items: center;
      padding: 4px 8px;
      border-radius: 999px;
      font-family: var(--mono);
      font-size: 0.7rem;
      font-weight: 500;
      white-space: nowrap;
      border: 1px solid transparent;
    }

    .status-ok {
      color: var(--ok);
      background: var(--ok-soft);
      border-color: rgba(31, 122, 74, 0.18);
    }

    .status-bad {
      color: var(--bad);
      background: var(--bad-soft);
      border-color: rgba(178, 65, 45, 0.18);
    }

    .status-warn {
      color: var(--warn);
      background: var(--warn-soft);
      border-color: rgba(155, 93, 36, 0.18);
    }

    .status-dim {
      color: var(--muted);
      background: rgba(116, 102, 75, 0.1);
      border-color: rgba(116, 102, 75, 0.12);
    }

    .actions {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
    }

    .action-btn {
      min-height: 32px;
      padding: 0 12px;
      border-radius: 10px;
      border: 1px solid var(--border);
      background: var(--panel-strong);
      color: var(--text);
      font: inherit;
      font-size: 0.8rem;
      font-weight: 600;
      cursor: pointer;
      transition: border-color 0.15s ease, transform 0.15s ease;
    }

    .action-btn.primary {
      background: var(--accent-soft);
      color: var(--accent);
      border-color: rgba(155, 93, 36, 0.2);
    }

    .action-btn.success {
      background: var(--ok-soft);
      color: var(--ok);
      border-color: rgba(31, 122, 74, 0.2);
    }

    .action-btn:disabled {
      opacity: 0.45;
      cursor: not-allowed;
      transform: none;
    }

    .empty-state {
      padding: 46px 20px;
      text-align: center;
      color: var(--muted);
    }

    .overlay {
      position: fixed;
      inset: 0;
      display: none;
      align-items: center;
      justify-content: center;
      background: rgba(30, 21, 9, 0.56);
      backdrop-filter: blur(10px);
      padding: 20px;
      z-index: 20;
    }

    .overlay.active {
      display: flex;
    }

    .overlay-card {
      width: min(1380px, 94vw);
      height: min(88vh, 980px);
      background: var(--panel-strong);
      border: 1px solid var(--border);
      border-radius: 18px;
      overflow: hidden;
      display: flex;
      flex-direction: column;
      box-shadow: var(--shadow);
    }

    .overlay-card.run-card {
      width: min(980px, 94vw);
      height: min(72vh, 760px);
    }

    .overlay-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      padding: 14px 16px;
      border-bottom: 1px solid var(--border);
      background: rgba(255, 250, 242, 0.88);
    }

    .overlay-title {
      min-width: 0;
      font-weight: 600;
    }

    .overlay-actions {
      display: flex;
      align-items: center;
      gap: 10px;
    }

    .overlay-link {
      color: var(--accent);
      text-decoration: none;
      font-weight: 600;
      font-size: 0.84rem;
    }

    .overlay-link:hover {
      text-decoration: underline;
    }

    .overlay-header button {
      min-height: 32px;
      min-width: 32px;
      border-radius: 10px;
      border: 1px solid var(--border);
      background: var(--panel-strong);
      cursor: pointer;
    }

    iframe {
      flex: 1 1 auto;
      width: 100%;
      border: 0;
      background: #fff;
    }

    pre {
      margin: 0;
      flex: 1 1 auto;
      overflow: auto;
      padding: 18px;
      font-family: var(--mono);
      font-size: 0.78rem;
      line-height: 1.55;
      background: #211a0f;
      color: #f8f2e6;
      white-space: pre-wrap;
      word-break: break-word;
    }

    @media (max-width: 840px) {
      main {
        padding: 18px 14px 28px;
      }

      .hero, .metrics, .table-card {
        border-radius: 16px;
      }

      .filter input {
        min-width: 100%;
      }
    }
  </style>
</head>
<body>
  <main>
    <section class="hero">
      <div>
        <div class="eyebrow">High-Evals Dashboard</div>
        <h1>Runnable eval quality, not idle-state theater.</h1>
        <div class="hero-copy">
          Agent completion, validation success, preview mode, and run contracts are tracked separately. Headline rates exclude mobile and integration tracks until they have dedicated runtime harnesses.
        </div>
      </div>
      <div class="hero-notes">
        <span class="chip">Headline suite: ${report.headlineEvals} runs</span>
        <span class="chip">Full suite: ${report.totalEvals} runs</span>
      </div>
    </section>

    <section class="metrics">
      <article class="metric">
        <div class="metric-label">Headline Success</div>
        <div class="metric-value">${formatPercent(report.headlineSuccessRate)}</div>
      </article>
      <article class="metric">
        <div class="metric-label">Agent Success</div>
        <div class="metric-value">${report.agentSuccessfulEvals}/${report.totalEvals}</div>
      </article>
      <article class="metric">
        <div class="metric-label">Validation Pass</div>
        <div class="metric-value">${formatPercent(report.validationRate)}</div>
      </article>
      <article class="metric">
        <div class="metric-label">Full Success</div>
        <div class="metric-value">${formatPercent(report.successRate)}</div>
      </article>
      <article class="metric">
        <div class="metric-label">Total Runtime</div>
        <div class="metric-value">${formatDuration(report.totalDurationSeconds)}</div>
      </article>
      <article class="metric">
        <div class="metric-label">Known Cost</div>
        <div class="metric-value">${formatCost(report.totalKnownCostUsd)} · ${report.knownCostCount}/${report.totalEvals}</div>
      </article>
    </section>

    <section class="table-card">
      <div class="toolbar">
        <div class="filter">
          <label for="filter-model">Model</label>
          <select id="filter-model" onchange="applyFilters()">
            <option value="">All</option>
            ${modelOptions}
          </select>
        </div>
        <div class="filter">
          <label for="filter-track">Track</label>
          <select id="filter-track" onchange="applyFilters()">
            <option value="">All</option>
            ${trackOptions}
          </select>
        </div>
        <div class="filter">
          <label for="filter-status">Status</label>
          <select id="filter-status" onchange="applyFilters()">
            <option value="">All</option>
            <option value="success">Validated success</option>
            <option value="agent_failed">Agent failed</option>
            <option value="validation_failed">Validation failed</option>
            <option value="legacy">Legacy/partial</option>
          </select>
        </div>
        <div class="filter">
          <label for="filter-search">Search</label>
          <input id="filter-search" type="text" placeholder="prompt, folder, violation..." oninput="applyFilters()" />
        </div>
        <div class="toolbar-spacer"></div>
        <div class="chip" id="row-count">${report.rows.length} shown</div>
        <button type="button" onclick="resetFilters()">Reset</button>
      </div>

      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Prompt</th>
              <th>Model</th>
              <th>Track</th>
              <th>Agent</th>
              <th>Validation</th>
              <th>Preview</th>
              <th>Run</th>
              <th>Violations</th>
              <th>Completed</th>
              <th>Folder</th>
              <th>Actions</th>
            </tr>
          </thead>
          <tbody id="rows"></tbody>
        </table>
      </div>
    </section>
  </main>

  <div class="overlay" id="preview-overlay" onclick="closeOverlay(event, 'preview-overlay')">
    <div class="overlay-card">
      <div class="overlay-header">
        <div class="overlay-title" id="preview-title"></div>
        <div class="overlay-actions">
          <a id="preview-link" class="overlay-link" href="#" target="_blank" rel="noreferrer">Open in tab</a>
          <button type="button" onclick="closeOverlay(null, 'preview-overlay')">✕</button>
        </div>
      </div>
      <iframe id="preview-frame" sandbox="allow-scripts allow-same-origin allow-forms"></iframe>
    </div>
  </div>

  <div class="overlay" id="run-overlay" onclick="closeOverlay(event, 'run-overlay')">
    <div class="overlay-card run-card">
      <div class="overlay-header">
        <div class="overlay-title" id="run-title"></div>
        <div class="overlay-actions">
          <button type="button" onclick="closeOverlay(null, 'run-overlay')">✕</button>
        </div>
      </div>
      <pre id="run-output"></pre>
    </div>
  </div>

  <script>
    const rows = ${rowsJson};
    let filteredRows = [...rows];

    function escapeHtml(value) {
      return String(value)
        .replaceAll("&", "&amp;")
        .replaceAll("<", "&lt;")
        .replaceAll(">", "&gt;")
        .replaceAll('"', "&quot;")
        .replaceAll("'", "&#39;");
    }

    function badge(kind, label, title = "") {
      const safeTitle = title ? \` title="\${escapeHtml(title)}"\` : "";
      return \`<span class="status \${kind}"\${safeTitle}>\${escapeHtml(label)}</span>\`;
    }

    function formatDate(value) {
      if (!value) return "Unknown";
      const date = new Date(value);
      if (Number.isNaN(date.getTime())) return value;
      return date.toLocaleString();
    }

    function formatDuration(seconds) {
      const total = Math.max(0, Number(seconds || 0));
      const minutes = Math.floor(total / 60);
      const rem = total % 60;
      if (minutes > 0) return \`\${minutes}m \${rem}s\`;
      return \`\${rem}s\`;
    }

    function formatCost(value) {
      if (value === null || value === undefined || Number.isNaN(Number(value))) return "n/a";
      return \`$\${Number(value).toFixed(4)}\`;
    }

    function statusFilterMatches(row, selected) {
      if (!selected) return true;
      if (selected === "success") return row.success;
      if (selected === "agent_failed") return !row.agentSuccess;
      if (selected === "validation_failed") return row.agentSuccess && !row.validationSuccess;
      if (selected === "legacy") return row.legacy;
      return true;
    }

    function matchesSearch(row, query) {
      if (!query) return true;
      const haystack = [
        row.prompt,
        row.promptTitle,
        row.promptID,
        row.folder,
        row.model,
        row.track,
        row.error,
        ...(row.violations || []),
      ].filter(Boolean).join("\\n").toLowerCase();
      return haystack.includes(query);
    }

    function renderRows() {
      const tbody = document.getElementById("rows");
      const count = document.getElementById("row-count");
      count.textContent = \`\${filteredRows.length} shown\`;

      if (filteredRows.length === 0) {
        tbody.innerHTML = '<tr><td colspan="11"><div class="empty-state">No evals match the current filters.</div></td></tr>';
        return;
      }

      tbody.innerHTML = filteredRows.map((row) => {
        const promptLabel = row.promptNumber ? \`p\${row.promptNumber}\` : row.promptID || "prompt";
        const promptTitle = row.promptTitle || row.prompt;
        const promptPreview = row.prompt.length > 160 ? \`\${row.prompt.slice(0, 157)}...\` : row.prompt;
        const violations = Array.isArray(row.violations) ? row.violations : [];
        const violationTitle = violations.join("\\n");
        const validationLabel = row.validationSuccess ? "pass" : "fail";
        const previewLabel = row.previewMode === "project_server"
          ? "project server"
          : row.previewMode === "static"
            ? "static"
            : "none";
        const runLabel = row.runMode === "none" ? "none" : row.runMode;
        const previewButton = row.previewPath
          ? \`<button type="button" class="action-btn primary" onclick="openPreview('\${escapeHtml(row.folder)}')">Preview</button>\`
          : "";
        const pickButton = row.runMode === "uv" || row.runMode === "legacy"
          ? \`<button type="button" class="action-btn" onclick="pickRunTarget('\${escapeHtml(row.folder)}')">Pick</button>\`
          : "";
        const runButton = row.runMode !== "none"
          ? \`<button type="button" class="action-btn success" onclick="runEval('\${escapeHtml(row.folder)}')">Run</button>\`
          : "";

        return \`<tr>
          <td>
            <div class="prompt-title">\${escapeHtml(promptTitle)}</div>
            <div class="status-stack" style="margin-bottom:6px;">
              \${badge("status-dim", promptLabel)}
              \${row.legacy ? badge("status-warn", "legacy") : ""}
              \${row.headlineEligible ? badge("status-ok", "headline") : badge("status-dim", "non-headline")}
            </div>
            <div class="prompt-meta">\${escapeHtml(promptPreview)}</div>
            \${row.error ? \`<div class="error-text" style="margin-top:8px;">\${escapeHtml(row.error)}</div>\` : ""}
          </td>
          <td class="mono">\${escapeHtml(row.model)}</td>
          <td><div class="status-stack">\${badge("status-dim", row.track)}</div></td>
          <td><div class="status-stack">\${row.agentSuccess ? badge("status-ok", "ok") : badge("status-bad", "failed")}</div></td>
          <td><div class="status-stack">\${row.validationSuccess ? badge("status-ok", validationLabel) : badge("status-bad", validationLabel)}\${row.checks && Object.keys(row.checks).length ? badge("status-dim", \`\${Object.values(row.checks).filter(Boolean).length}/\${Object.keys(row.checks).length}\`) : ""}</div></td>
          <td><div class="status-stack">\${row.previewMode === "none" ? badge("status-dim", previewLabel) : badge(row.previewMode === "static" ? "status-ok" : "status-warn", previewLabel)}</div></td>
          <td><div class="status-stack">\${row.runMode === "none" ? badge("status-dim", runLabel) : badge(row.runMode === ".run" ? "status-ok" : "status-warn", runLabel)}</div></td>
          <td><div class="status-stack">\${violations.length === 0 ? badge("status-ok", "0") : badge("status-bad", String(violations.length), violationTitle)}</div></td>
          <td class="prompt-meta">\${escapeHtml(formatDate(row.completedAt))}<br><span class="mono">\${escapeHtml(formatDuration(row.durationSeconds))} · \${escapeHtml(formatCost(row.costUsd))}</span></td>
          <td class="folder-text">\${escapeHtml(row.folder)}</td>
          <td><div class="actions">\${previewButton}\${runButton}\${pickButton || ""}</div></td>
        </tr>\`;
      }).join("");
    }

    function applyFilters() {
      const model = document.getElementById("filter-model").value;
      const track = document.getElementById("filter-track").value;
      const status = document.getElementById("filter-status").value;
      const search = document.getElementById("filter-search").value.trim().toLowerCase();

      filteredRows = rows
        .filter((row) => !model || row.model === model)
        .filter((row) => !track || row.track === track)
        .filter((row) => statusFilterMatches(row, status))
        .filter((row) => matchesSearch(row, search))
        .sort((a, b) => (b.completedAtEpoch || 0) - (a.completedAtEpoch || 0));

      renderRows();
    }

    function resetFilters() {
      document.getElementById("filter-model").value = "";
      document.getElementById("filter-track").value = "";
      document.getElementById("filter-status").value = "";
      document.getElementById("filter-search").value = "";
      applyFilters();
    }

    function findRow(folder) {
      return rows.find((row) => row.folder === folder);
    }

    function openPreview(folder) {
      const row = findRow(folder);
      if (!row || !row.previewPath) return;
      document.getElementById("preview-title").textContent = row.promptTitle || row.prompt;
      document.getElementById("preview-link").href = row.previewPath;
      document.getElementById("preview-frame").src = row.previewPath;
      document.getElementById("preview-overlay").classList.add("active");
    }

    function closeOverlay(event, id) {
      if (event && event.target !== event.currentTarget) return;
      const overlay = document.getElementById(id);
      overlay.classList.remove("active");
      if (id === "preview-overlay") {
        document.getElementById("preview-frame").src = "about:blank";
      }
    }

    function showRunOutput(title, output) {
      document.getElementById("run-title").textContent = title;
      document.getElementById("run-output").textContent = output;
      document.getElementById("run-overlay").classList.add("active");
    }

    async function runEval(folder, target = null) {
      const buttonLabel = target ? \`\${folder} · \${target}\` : folder;
      showRunOutput(\`Running \${buttonLabel}\`, "Running...");
      const query = target ? \`?target=\${encodeURIComponent(target)}\` : "";
      const response = await fetch(\`/run/\${encodeURIComponent(folder)}\${query}\`);
      const payload = await response.json();
      showRunOutput(\`Run output · \${buttonLabel}\`, payload.output || "(no output)");
    }

    async function pickRunTarget(folder) {
      const response = await fetch(\`/api/run-options/\${encodeURIComponent(folder)}\`);
      const payload = await response.json();
      if (!payload.ok) {
        showRunOutput(\`Run options · \${folder}\`, "No runnable targets found.");
        return;
      }

      const targets = Array.isArray(payload.targets) ? payload.targets : [];
      if (targets.length <= 1) {
        await runEval(folder, targets[0] || null);
        return;
      }

      const choice = window.prompt(
        \`Choose a target for \${folder}:\\n\\n\${targets.map((target, index) => \`\${index + 1}. \${target}\`).join("\\n")}\`,
        payload.defaultTarget || targets[0],
      );
      if (!choice) return;
      await runEval(folder, choice.trim());
    }

    applyFilters();
  </script>
</body>
</html>`;
}

function escapeHtml(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function formatPercent(value: number): string {
  return `${value.toFixed(1)}%`;
}

function formatDuration(totalSeconds: number): string {
  const seconds = Math.max(0, Math.round(totalSeconds));
  const minutes = Math.floor(seconds / 60);
  const remainder = seconds % 60;
  if (minutes > 0) {
    return `${minutes}m ${remainder}s`;
  }
  return `${remainder}s`;
}

function formatCost(cost: number): string {
  return `$${cost.toFixed(4)}`;
}
