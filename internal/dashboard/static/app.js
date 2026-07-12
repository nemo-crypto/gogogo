const state = {
  data: null,
  refreshTimer: null,
  loading: false,
};

const AUTO_REFRESH_MS = 15000;

const $ = (id) => document.getElementById(id);

document.addEventListener("DOMContentLoaded", () => {
  $("refresh").addEventListener("click", loadDashboard);
  ["market", "symbol", "interval"].forEach((id) => {
    $(id).addEventListener("change", loadDashboard);
  });
  document.addEventListener("visibilitychange", () => {
    if (!document.hidden) {
      loadDashboard();
    }
  });
  loadDashboard();
  startAutoRefresh();
});

async function loadDashboard() {
  if (state.loading) return;
  state.loading = true;

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
      cache: "no-store",
    });
    if (!response.ok) {
      throw new Error(`请求失败 ${response.status}`);
    }
    state.data = await response.json();
    render(state.data);
  } catch (error) {
    renderError(error);
  } finally {
    $("refresh").disabled = false;
    state.loading = false;
  }
}

function startAutoRefresh() {
  if (state.refreshTimer) {
    window.clearInterval(state.refreshTimer);
  }
  state.refreshTimer = window.setInterval(() => {
    if (!document.hidden) {
      loadDashboard();
    }
  }, AUTO_REFRESH_MS);
}

function render(data) {
  $("subtitle").textContent = `${marketLabel(data.query.market_type)} ${data.query.symbol} ${data.query.interval}`;
  $("generatedAt").textContent = `更新于 ${formatTime(data.generated_at)}`;
  $("runtimeBadge").textContent = data.runtime.halted ? "已停机" : "运行中";
  $("runtimeBadge").className = data.runtime.halted ? "badge badge-danger" : "badge badge-ok";

  renderWarnings(data.warnings || []);
  renderMetrics(data);
  renderStrategyOverview(data);
  renderChart(data.price_series || []);
  renderCoverage(data.market_coverage || []);
  renderBacktests(data.backtests || []);
  renderBacktestLogs(data.backtests || []);
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
  $("latestMeta").textContent = latest ? formatTime(latest.time) : "暂无 K 线";

  const backtests = data.backtests || [];
  const best = [...backtests].sort((a, b) => b.excess_return_pct - a.excess_return_pct)[0];
  $("runCount").textContent = number(data.counts.backtest_runs || 0);
  $("bestBacktest").textContent = best ? `${best.symbol} 超额 ${pct(best.excess_return_pct)}` : "暂无回测";

  const risks = data.risk_events || [];
  $("riskCount").textContent = number(data.counts.risk_events || 0);
  $("lastRisk").textContent = risks.length ? `${risks[0].symbol} ${decisionLabel(risks[0].decision)}` : "暂无风控事件";

  const performance = data.performance_snapshots || [];
  const latestPerf = performance[0];
  $("paperEquity").textContent = latestPerf ? money(latestPerf.equity) : "--";
  $("paperPnL").textContent = latestPerf
    ? `盈亏 ${money(latestPerf.pnl)} / 回撤 ${pct(latestPerf.drawdown_pct)}`
    : "暂无模拟运行";
}

function renderStrategyOverview(data) {
  const backtests = data.backtests || [];
  const selectedBacktests = backtests.filter(
    (row) =>
      row.market_type === data.query.market_type &&
      row.symbol === data.query.symbol &&
      row.interval === data.query.interval,
  );
  const latestBacktest = selectedBacktests[0] || backtests[0];
  const latestSignal = (data.signals || []).find(
    (row) =>
      row.market_type === data.query.market_type &&
      row.symbol === data.query.symbol,
  );

  if (!latestBacktest) {
    $("strategyName").textContent = "暂无策略回测";
    $("strategySubtitle").textContent = "先运行 backtest 或 papertrade 后会显示策略说明";
    $("strategyPlainText").textContent = "当前数据库里还没有可展示的策略参数。";
    $("buyRule").textContent = "--";
    $("sellRule").textContent = "--";
    $("currentSignal").textContent = latestSignal ? actionLabel(latestSignal.action) : "--";
    $("signalReason").textContent = latestSignal ? signalText(latestSignal) : "暂无最新信号";
    return;
  }

  const fast = latestBacktest.fast_window || parseStrategyWindow(latestBacktest.strategy_name, 0);
  const slow = latestBacktest.slow_window || parseStrategyWindow(latestBacktest.strategy_name, 1);
  const strategyName = strategyDisplayName(latestBacktest);
  $("strategyName").textContent = strategyName;
  $("strategySubtitle").textContent = `${marketLabel(data.query.market_type)} ${data.query.symbol} ${data.query.interval}`;
  if (isAdaptiveTrend(latestBacktest)) {
    $("strategyPlainText").textContent =
      `这是一个多币种趋势轮动策略：用 ${fast} 根 K 线看动量，用 ${slow} 根 K 线判断大趋势，并按波动率控制仓位。`;
    $("buyRule").textContent = "强趋势 + 高排名";
    $("sellRule").textContent = "排名跌出 / 移动止损";
    $("strategyLimit").textContent = `手续费 ${pct((latestBacktest.fee_rate || 0) * 100)}`;
    $("currentSignal").textContent = latestSignal ? actionLabel(latestSignal.action) : "看回测轮动";
    $("signalReason").textContent = "优先选择强势币，过滤过高 funding，避免单币种满仓追涨。";
    return;
  }
  $("strategyPlainText").textContent =
    `这是一个趋势跟随策略：用 ${fast} 根 K 线均价代表短期走势，用 ${slow} 根 K 线均价代表长期走势。`;
  $("buyRule").textContent = `${fast} 均线 > ${slow} 均线`;
  $("sellRule").textContent = `${fast} 均线 < ${slow} 均线`;
  $("strategyLimit").textContent = `手续费 ${pct((latestBacktest.fee_rate || 0) * 100)}`;
  $("currentSignal").textContent = latestSignal ? actionLabel(latestSignal.action) : "暂无信号";
  $("signalReason").textContent = latestSignal
    ? signalText(latestSignal)
    : "运行 papertrade 后，这里会显示最新买入或观望信号。";
}

function renderChart(series) {
  const svg = $("priceChart");
  svg.replaceChildren();
  $("seriesCount").textContent = `${series.length} 个点`;
  $("chartLabel").textContent = series.length
    ? `${formatTime(series[0].time)} 至 ${formatTime(series[series.length - 1].time)}`
    : "暂无 K 线数据";

  const width = 900;
  const height = 320;
  const pad = { top: 20, right: 68, bottom: 42, left: 56 };
  const chartW = width - pad.left - pad.right;
  const chartH = height - pad.top - pad.bottom;

  if (!series.length) {
    svg.append(text(width / 2, height / 2, "暂无 K 线数据", "chart-label", "middle"));
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
        <td>${escapeHTML(marketLabel(row.market_type))}</td>
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
        <td>${escapeHTML(strategyDisplayName(row))}</td>
        <td>${escapeHTML(marketLabel(row.market_type))}</td>
        <td>${escapeHTML(row.symbol)}</td>
        <td class="numeric ${tone(row.total_return_pct)}">${pct(row.total_return_pct)}</td>
        <td class="numeric ${tone(row.excess_return_pct)}">${pct(row.excess_return_pct)}</td>
        <td class="numeric negative">${pct(row.max_drawdown_pct)}</td>
      </tr>`,
    )
    .join("") || emptyRow(6);
}

function renderBacktestLogs(rows) {
  $("backtestLogRows").innerHTML = rows
    .map(
      (row) => `<tr>
        <td class="numeric">${number(row.id)}</td>
        <td>${formatTime(row.created_at)}</td>
        <td>${escapeHTML(strategyDisplayName(row))}</td>
        <td class="numeric ${tone(row.total_return_pct)}">${pct(row.total_return_pct)}</td>
        <td class="numeric ${tone(row.excess_return_pct)}">${pct(row.excess_return_pct)}</td>
        <td class="numeric negative">${pct(row.max_drawdown_pct)}</td>
        <td class="numeric">${number(row.trade_count)}</td>
      </tr>`,
    )
    .join("") || emptyRow(7);
}

function renderOrders(rows) {
  $("orderRows").innerHTML = rows
    .map(
      (row) => `<tr>
        <td>${formatTime(row.created_at)}</td>
        <td>${escapeHTML(row.symbol)}</td>
        <td>${escapeHTML(sideLabel(row.side))}</td>
        <td>${escapeHTML(statusLabel(row.status))}</td>
        <td class="${row.risk_decision === "allow" ? "positive" : "negative"}">${escapeHTML(decisionLabel(row.risk_decision))}</td>
      </tr>`,
    )
    .join("") || emptyRow(5);
}

function renderBalances(rows) {
  $("balanceRows").innerHTML = rows
    .map(
      (row) => `<div class="row-card">
        <div><strong>${escapeHTML(row.account_id)} / ${escapeHTML(row.asset)}</strong><span>${formatTime(row.snapshot_time)}</span></div>
        <div class="numeric"><strong>${money(row.total)}</strong><span>可用 ${money(row.free)}</span></div>
      </div>`,
    )
    .join("") || emptyBlock("暂无余额");
}

function renderPositions(rows) {
  $("positionRows").innerHTML = rows
    .map((row) => {
      const pnl = positionPnL(row);
      const pnlPct = positionPnLPct(row, pnl.value);
      return `<div class="position-card">
        <div class="position-head">
          <div>
            <strong>${escapeHTML(row.symbol)} ${escapeHTML(positionSideLabel(row.position_side))}</strong>
            <span>${escapeHTML(marketLabel(row.market_type))} / ${escapeHTML(row.margin_mode || "cross")} / ${formatTime(row.snapshot_time)}</span>
          </div>
          <div class="position-pnl numeric">
            <strong class="${tone(pnl.value)}">${signedMoney(pnl.value)}</strong>
            <span>${pnl.estimated ? "估算盈亏" : "未实现盈亏"} ${formatPctValue(pnlPct)}</span>
          </div>
        </div>
        <div class="position-detail">
          ${positionMetric("数量", quantity(row.quantity))}
          ${positionMetric("开仓价", moneyOrDash(row.entry_price))}
          ${positionMetric("标记价", moneyOrDash(row.mark_price))}
          ${positionMetric("强平价", moneyOrDash(row.liquidation_price))}
          ${positionMetric("杠杆", leverage(row.leverage))}
          ${positionMetric("名义价值", moneyOrDash(row.notional))}
          ${positionMetric("强平距离", pct(row.liquidation_distance_pct))}
          ${positionMetric("账户", row.account_id || "--")}
        </div>
      </div>`;
    })
    .join("") || emptyBlock("暂无持仓");
}

function renderSignals(rows) {
  $("signalRows").innerHTML = rows
    .map(
      (row) => `<div class="row-card">
        <div><strong>${escapeHTML(row.strategy_id)} / ${escapeHTML(row.symbol)}</strong><span>${escapeHTML(row.reason || "信号")} ${formatTime(row.signal_time)}</span></div>
        <div class="numeric"><strong>${escapeHTML(actionLabel(row.action))}</strong><span>${pct(row.confidence * 100)}</span></div>
      </div>`,
    )
    .join("") || emptyBlock("暂无信号");
}

function renderPerformance(rows) {
  $("performanceRows").innerHTML = rows
    .map(
      (row) => `<div class="row-card">
        <div><strong>${escapeHTML(row.strategy_id)} 运行 ${row.run_id}</strong><span>${formatTime(row.snapshot_time)}</span></div>
        <div class="numeric"><strong>${money(row.equity)}</strong><span class="${tone(row.pnl)}">盈亏 ${money(row.pnl)}</span></div>
      </div>`,
    )
    .join("") || emptyBlock("暂无绩效");
}

function renderSnapshots(rows) {
  $("snapshotRows").innerHTML = rows
    .map(
      (row) => `<tr>
        <td>${escapeHTML(row.name)}</td>
        <td>${escapeHTML(marketLabel(row.market_type))}</td>
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
    .join("") || emptyBlock("暂无资金费率");
}

function renderMarks(rows) {
  $("markRows").innerHTML = rows
    .map(
      (row) => `<div class="row-card">
        <div><strong>${escapeHTML(row.symbol)}</strong><span>${formatTime(row.event_time)}</span></div>
        <div class="numeric"><strong>${money(row.mark_price)}</strong><span>指数 ${money(row.index_price)}</span></div>
      </div>`,
    )
    .join("") || emptyBlock("暂无标记价格");
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
  $("warnings").textContent = `看板加载失败：${error.message}`;
}

function money(value) {
  const numberValue = Number(value || 0);
  return new Intl.NumberFormat("en-US", {
    maximumFractionDigits: Math.abs(numberValue) >= 100 ? 2 : 6,
  }).format(numberValue);
}

function moneyOrDash(value) {
  const numberValue = Number(value || 0);
  if (numberValue === 0) return "--";
  return money(numberValue);
}

function signedMoney(value) {
  const numberValue = Number(value || 0);
  const prefix = numberValue > 0 ? "+" : "";
  return `${prefix}${money(numberValue)}`;
}

function quantity(value) {
  return new Intl.NumberFormat("en-US", {
    maximumFractionDigits: 8,
  }).format(Number(value || 0));
}

function leverage(value) {
  return `${Number(value || 0).toFixed(1)}x`;
}

function positionPnL(row) {
  const provided = Number(row.unrealized_pnl || 0);
  const entry = Number(row.entry_price || 0);
  const mark = Number(row.mark_price || 0);
  const qty = Number(row.quantity || 0);
  if (provided !== 0 || entry <= 0 || mark <= 0 || qty === 0) {
    return { value: provided, estimated: false };
  }
  if (row.position_side === "short" && qty > 0) {
    return { value: (entry - mark) * Math.abs(qty), estimated: true };
  }
  return { value: (mark - entry) * qty, estimated: true };
}

function positionPnLPct(row, pnlValue) {
  const notional = Math.abs(Number(row.notional || 0));
  if (notional <= 0) return null;
  return (Number(pnlValue || 0) / notional) * 100;
}

function formatPctValue(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) {
    return "--";
  }
  const numberValue = Number(value);
  const prefix = numberValue > 0 ? "+" : "";
  return `${prefix}${numberValue.toFixed(2)}%`;
}

function positionMetric(label, value) {
  return `<span><em>${escapeHTML(label)}</em><strong>${escapeHTML(value)}</strong></span>`;
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
  return `<tr><td colspan="${colspan}" class="empty">暂无数据</td></tr>`;
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

function marketLabel(value) {
  const labels = {
    spot: "现货",
    perpetual: "永续合约",
  };
  return labels[value] || value || "--";
}

function decisionLabel(value) {
  const labels = {
    allow: "通过",
    reject: "拒绝",
    reduce: "降仓",
    halt: "停机",
  };
  return labels[value] || value || "--";
}

function sideLabel(value) {
  const labels = {
    buy: "买入",
    sell: "卖出",
    short: "做空",
    cover: "平空",
  };
  return labels[value] || value || "--";
}

function statusLabel(value) {
  const labels = {
    dry_run_accepted: "模拟通过",
    risk_rejected: "风控拒绝",
    planned: "已计划",
    submitted: "已提交",
    filled: "已成交",
    canceled: "已撤单",
    failed: "失败",
  };
  return labels[value] || value || "--";
}

function positionSideLabel(value) {
  const labels = {
    long: "多头",
    short: "空头",
    both: "双向",
    net: "净仓位",
  };
  return labels[value] || value || "--";
}

function actionLabel(value) {
  const labels = {
    buy: "买入",
    sell: "卖出",
    short: "做空",
    cover: "平空",
    hold: "观望",
  };
  return labels[value] || value || "--";
}

function strategyDisplayName(row) {
  if (isAdaptiveTrend(row)) {
    return "多币种自适应趋势轮动";
  }
  const fast = row.fast_window || parseStrategyWindow(row.strategy_name, 0);
  const slow = row.slow_window || parseStrategyWindow(row.strategy_name, 1);
  if (fast && slow) {
    return `SMA ${fast}/${slow} 均线交叉`;
  }
  return row.strategy_name || "--";
}

function isAdaptiveTrend(row) {
  return String(row.strategy_name || "").startsWith("adaptive_trend_rotation_");
}

function parseStrategyWindow(name, index) {
  const match = String(name || "").match(/sma_crossover_(\d+)_(\d+)/);
  if (!match) return 0;
  return Number(match[index + 1] || 0);
}

function signalText(signal) {
  const confidence = pct(Number(signal.confidence || 0) * 100);
  const reason = signal.reason ? `，原因 ${signal.reason}` : "";
  return `${formatTime(signal.signal_time)}，置信度 ${confidence}${reason}`;
}
