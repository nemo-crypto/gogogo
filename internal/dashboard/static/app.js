const state = {
  data: null,
};

const $ = (id) => document.getElementById(id);

document.addEventListener("DOMContentLoaded", () => {
  $("refresh").addEventListener("click", loadDashboard);
  ["market", "symbol", "interval"].forEach((id) => {
    $(id).addEventListener("change", loadDashboard);
  });
  loadDashboard();
});

async function loadDashboard() {
  const params = new URLSearchParams({
    market: $("market").value,
    symbol: $("symbol").value,
    interval: $("interval").value,
    limit: "240",
  });

  $("refresh").disabled = true;
  try {
    const response = await fetch(`/api/dashboard?${params.toString()}`, {
      headers: { Accept: "application/json" },
    });
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`);
    }
    state.data = await response.json();
    render(state.data);
  } catch (error) {
    renderError(error);
  } finally {
    $("refresh").disabled = false;
  }
}

function render(data) {
  $("subtitle").textContent = `${data.query.market_type} ${data.query.symbol} ${data.query.interval}`;
  $("generatedAt").textContent = `updated ${formatTime(data.generated_at)}`;
  $("runtimeBadge").textContent = data.runtime.halted ? "HALTED" : "RUNNING";
  $("runtimeBadge").className = data.runtime.halted ? "badge badge-danger" : "badge badge-ok";

  renderWarnings(data.warnings || []);
  renderMetrics(data);
  renderChart(data.price_series || []);
  renderCoverage(data.market_coverage || []);
  renderBacktests(data.backtests || []);
  renderOrders(data.orders || []);
  renderBalances(data.balances || []);
  renderPositions(data.positions || []);
  renderSignals(data.signals || []);
  renderPerformance(data.performance_snapshots || []);
  renderSnapshots(data.candle_snapshots || []);
  renderFunding(data.funding_rates || []);
  renderMarks(data.mark_prices || []);
}

function renderWarnings(warnings) {
  const box = $("warnings");
  if (!warnings.length) {
    box.hidden = true;
    box.textContent = "";
    return;
  }
  box.hidden = false;
  box.textContent = warnings.join(" | ");
}

function renderMetrics(data) {
  const series = data.price_series || [];
  const latest = series[series.length - 1];
  $("latestClose").textContent = latest ? money(latest.close) : "--";
  $("latestMeta").textContent = latest ? formatTime(latest.time) : "no candles";

  const backtests = data.backtests || [];
  const best = [...backtests].sort((a, b) => b.excess_return_pct - a.excess_return_pct)[0];
  $("runCount").textContent = number(data.counts.backtest_runs || 0);
  $("bestBacktest").textContent = best ? `${best.symbol} excess ${pct(best.excess_return_pct)}` : "no runs";

  const risks = data.risk_events || [];
  $("riskCount").textContent = number(data.counts.risk_events || 0);
  $("lastRisk").textContent = risks.length ? `${risks[0].symbol} ${risks[0].decision}` : "no risk event";

  const performance = data.performance_snapshots || [];
  const latestPerf = performance[0];
  $("paperEquity").textContent = latestPerf ? money(latestPerf.equity) : "--";
  $("paperPnL").textContent = latestPerf ? `PnL ${money(latestPerf.pnl)} / DD ${pct(latestPerf.drawdown_pct)}` : "no paper run";
}

function renderChart(series) {
  const svg = $("priceChart");
  svg.replaceChildren();
  $("seriesCount").textContent = `${series.length} points`;
  $("chartLabel").textContent = series.length
    ? `${formatTime(series[0].time)} to ${formatTime(series[series.length - 1].time)}`
    : "no candle data";

  const width = 900;
  const height = 320;
  const pad = { top: 20, right: 68, bottom: 42, left: 56 };
  const chartW = width - pad.left - pad.right;
  const chartH = height - pad.top - pad.bottom;

  if (!series.length) {
    svg.append(text(width / 2, height / 2, "No candle data", "chart-label", "middle"));
    return;
  }

  const closes = series.map((point) => point.close);
  const volumes = series.map((point) => point.volume);
  const minClose = Math.min(...closes);
  const maxClose = Math.max(...closes);
  const maxVolume = Math.max(...volumes, 1);
  const span = maxClose - minClose || 1;

  for (let i = 0; i <= 4; i += 1) {
    const y = pad.top + (chartH * i) / 4;
    svg.append(line(pad.left, y, width - pad.right, y, "grid"));
    const value = maxClose - (span * i) / 4;
    svg.append(text(width - pad.right + 8, y + 4, compact(value), "chart-label", "start"));
  }

  const xFor = (index) => pad.left + (chartW * index) / Math.max(series.length - 1, 1);
  const yFor = (price) => pad.top + ((maxClose - price) / span) * chartH;

  const volumeBase = height - pad.bottom;
  const volumeH = 54;
  series.forEach((point, index) => {
    const barW = Math.max(chartW / series.length - 1, 1);
    const barH = (point.volume / maxVolume) * volumeH;
    const rect = document.createElementNS("http://www.w3.org/2000/svg", "rect");
    rect.setAttribute("class", "volume-bar");
    rect.setAttribute("x", xFor(index) - barW / 2);
    rect.setAttribute("y", volumeBase - barH);
    rect.setAttribute("width", barW);
    rect.setAttribute("height", barH);
    svg.append(rect);
  });

  const path = series
    .map((point, index) => `${index === 0 ? "M" : "L"}${xFor(index).toFixed(2)},${yFor(point.close).toFixed(2)}`)
    .join(" ");
  const linePath = document.createElementNS("http://www.w3.org/2000/svg", "path");
  linePath.setAttribute("class", "price-line");
  linePath.setAttribute("d", path);
  svg.append(linePath);

  svg.append(line(pad.left, pad.top, pad.left, height - pad.bottom, "axis"));
  svg.append(line(pad.left, height - pad.bottom, width - pad.right, height - pad.bottom, "axis"));
  svg.append(text(pad.left, height - 14, formatShortDate(series[0].time), "chart-label", "start"));
  svg.append(text(width - pad.right, height - 14, formatShortDate(series[series.length - 1].time), "chart-label", "end"));
}

function renderCoverage(rows) {
  $("coverageRows").innerHTML = rows
    .map(
      (row) => `<tr>
        <td>${escapeHTML(row.market_type)}</td>
        <td>${escapeHTML(row.symbol)}</td>
        <td>${escapeHTML(row.interval)}</td>
        <td class="numeric">${number(row.candles)}</td>
        <td class="numeric">${money(row.last_close)}</td>
      </tr>`,
    )
    .join("") || emptyRow(5);
}

function renderBacktests(rows) {
  $("backtestRows").innerHTML = rows
    .map(
      (row) => `<tr>
        <td>${escapeHTML(row.strategy_name)}</td>
        <td>${escapeHTML(row.market_type)}</td>
        <td>${escapeHTML(row.symbol)}</td>
        <td class="numeric ${tone(row.total_return_pct)}">${pct(row.total_return_pct)}</td>
        <td class="numeric ${tone(row.excess_return_pct)}">${pct(row.excess_return_pct)}</td>
        <td class="numeric negative">${pct(row.max_drawdown_pct)}</td>
      </tr>`,
    )
    .join("") || emptyRow(6);
}

function renderOrders(rows) {
  $("orderRows").innerHTML = rows
    .map(
      (row) => `<tr>
        <td>${formatTime(row.created_at)}</td>
        <td>${escapeHTML(row.symbol)}</td>
        <td>${escapeHTML(row.side)}</td>
        <td>${escapeHTML(row.status)}</td>
        <td class="${row.risk_decision === "allow" ? "positive" : "negative"}">${escapeHTML(row.risk_decision)}</td>
      </tr>`,
    )
    .join("") || emptyRow(5);
}

function renderBalances(rows) {
  $("balanceRows").innerHTML = rows
    .map(
      (row) => `<div class="row-card">
        <div><strong>${escapeHTML(row.account_id)} / ${escapeHTML(row.asset)}</strong><span>${formatTime(row.snapshot_time)}</span></div>
        <div class="numeric"><strong>${money(row.total)}</strong><span>free ${money(row.free)}</span></div>
      </div>`,
    )
    .join("") || emptyBlock("No balances");
}

function renderPositions(rows) {
  $("positionRows").innerHTML = rows
    .map(
      (row) => `<div class="row-card">
        <div><strong>${escapeHTML(row.symbol)} ${escapeHTML(row.position_side)}</strong><span>${escapeHTML(row.market_type)} ${formatTime(row.snapshot_time)}</span></div>
        <div class="numeric"><strong>${money(row.notional)}</strong><span>liq distance ${pct(row.liquidation_distance_pct)}</span></div>
      </div>`,
    )
    .join("") || emptyBlock("No positions");
}

function renderSignals(rows) {
  $("signalRows").innerHTML = rows
    .map(
      (row) => `<div class="row-card">
        <div><strong>${escapeHTML(row.strategy_id)} / ${escapeHTML(row.symbol)}</strong><span>${escapeHTML(row.reason || "signal")} ${formatTime(row.signal_time)}</span></div>
        <div class="numeric"><strong>${escapeHTML(row.action)}</strong><span>${pct(row.confidence * 100)}</span></div>
      </div>`,
    )
    .join("") || emptyBlock("No signals");
}

function renderPerformance(rows) {
  $("performanceRows").innerHTML = rows
    .map(
      (row) => `<div class="row-card">
        <div><strong>${escapeHTML(row.strategy_id)} run ${row.run_id}</strong><span>${formatTime(row.snapshot_time)}</span></div>
        <div class="numeric"><strong>${money(row.equity)}</strong><span class="${tone(row.pnl)}">PnL ${money(row.pnl)}</span></div>
      </div>`,
    )
    .join("") || emptyBlock("No performance");
}

function renderSnapshots(rows) {
  $("snapshotRows").innerHTML = rows
    .map(
      (row) => `<tr>
        <td>${escapeHTML(row.name)}</td>
        <td>${escapeHTML(row.market_type)}</td>
        <td>${escapeHTML(row.symbol)}</td>
        <td class="numeric">${number(row.candle_count)} / ${number(row.expected_count)}</td>
        <td class="numeric ${row.gap_count > 0 ? "negative" : "positive"}">${number(row.gap_count)}</td>
      </tr>`,
    )
    .join("") || emptyRow(5);
}

function renderFunding(rows) {
  $("fundingRows").innerHTML = rows
    .map(
      (row) => `<div class="row-card">
        <div><strong>${escapeHTML(row.symbol)}</strong><span>${formatTime(row.funding_time)}</span></div>
        <div class="numeric"><strong class="${tone(row.funding_rate)}">${pct(row.funding_rate * 100)}</strong><span>${money(row.mark_price)}</span></div>
      </div>`,
    )
    .join("") || emptyBlock("No funding rates");
}

function renderMarks(rows) {
  $("markRows").innerHTML = rows
    .map(
      (row) => `<div class="row-card">
        <div><strong>${escapeHTML(row.symbol)}</strong><span>${formatTime(row.event_time)}</span></div>
        <div class="numeric"><strong>${money(row.mark_price)}</strong><span>index ${money(row.index_price)}</span></div>
      </div>`,
    )
    .join("") || emptyBlock("No mark prices");
}

function line(x1, y1, x2, y2, className) {
  const node = document.createElementNS("http://www.w3.org/2000/svg", "line");
  node.setAttribute("x1", x1);
  node.setAttribute("y1", y1);
  node.setAttribute("x2", x2);
  node.setAttribute("y2", y2);
  node.setAttribute("class", className);
  return node;
}

function text(x, y, value, className, anchor) {
  const node = document.createElementNS("http://www.w3.org/2000/svg", "text");
  node.setAttribute("x", x);
  node.setAttribute("y", y);
  node.setAttribute("class", className);
  node.setAttribute("text-anchor", anchor);
  node.textContent = value;
  return node;
}

function renderError(error) {
  $("warnings").hidden = false;
  $("warnings").textContent = `Dashboard load failed: ${error.message}`;
}

function money(value) {
  const numberValue = Number(value || 0);
  return new Intl.NumberFormat("en-US", {
    maximumFractionDigits: Math.abs(numberValue) >= 100 ? 2 : 6,
  }).format(numberValue);
}

function compact(value) {
  return new Intl.NumberFormat("en-US", {
    notation: "compact",
    maximumFractionDigits: 2,
  }).format(Number(value || 0));
}

function pct(value) {
  return `${Number(value || 0).toFixed(2)}%`;
}

function number(value) {
  return new Intl.NumberFormat("en-US").format(Number(value || 0));
}

function formatTime(value) {
  if (!value) return "--";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "--";
  return date.toLocaleString(undefined, {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function formatShortDate(value) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "--";
  return date.toLocaleDateString(undefined, { month: "2-digit", day: "2-digit" });
}

function tone(value) {
  const numberValue = Number(value || 0);
  if (numberValue > 0) return "positive";
  if (numberValue < 0) return "negative";
  return "";
}

function emptyRow(colspan) {
  return `<tr><td colspan="${colspan}" class="empty">No data</td></tr>`;
}

function emptyBlock(label) {
  return `<div class="empty">${label}</div>`;
}

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}
