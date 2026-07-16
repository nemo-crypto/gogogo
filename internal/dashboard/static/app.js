const state = {
  data: null,
  refreshTimer: null,
  loading: false,
  lastLoadedAt: 0,
};

const AUTO_REFRESH_MS = 30000;
const STALE_REFRESH_MS = 10000;

const $ = (id) => document.getElementById(id);

document.addEventListener("DOMContentLoaded", () => {
  $("refresh").addEventListener("click", loadDashboard);
  $("loadTable").addEventListener("click", loadTablePreview);
  $("dbTable").addEventListener("change", loadTablePreview);
  ["exchange", "market", "symbol", "interval"].forEach((id) => {
    $(id).addEventListener("change", loadDashboard);
  });
  document.addEventListener("visibilitychange", () => {
    if (!document.hidden && Date.now() - state.lastLoadedAt > STALE_REFRESH_MS) {
      loadDashboard();
    }
  });
  loadDashboard();
  loadTablePreview();
  startAutoRefresh();
});

async function loadDashboard() {
  if (state.loading) return;
  state.loading = true;

  const params = new URLSearchParams({
    exchange: $("exchange").value,
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
    state.lastLoadedAt = Date.now();
    render(state.data);
  } catch (error) {
    renderError(error);
  } finally {
    $("refresh").disabled = false;
    state.loading = false;
  }
}

async function loadTablePreview() {
  const params = new URLSearchParams({
    name: $("dbTable").value,
    limit: "50",
  });

  $("loadTable").disabled = true;
  try {
    const response = await fetch(`/api/table?${params.toString()}`, {
      headers: { Accept: "application/json" },
      cache: "no-store",
    });
    if (!response.ok) {
      throw new Error(`请求失败 ${response.status}`);
    }
    renderTablePreview(await response.json());
  } catch (error) {
    $("dbTableMeta").textContent = `加载失败：${error.message}`;
    $("dbTableHead").innerHTML = "";
    $("dbTableRows").innerHTML = "";
  } finally {
    $("loadTable").disabled = false;
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
  $("subtitle").textContent = "OneBullEx 永续合约 / 持仓 / 盈亏 / 回测入库";
  $("generatedAt").textContent = `更新于 ${formatTime(data.generated_at)}`;
  const runtime = data.runtime || {};
  const runtimeAccount = runtime.account_id ? ` / ${accountLabel(runtime.account_id)}` : "";
  $("runtimeBadge").textContent = runtime.halted ? "已停机" : `运行中${runtimeAccount}`;
  $("runtimeBadge").className = runtime.halted ? "badge badge-danger" : "badge badge-ok";

  renderWarnings(data.warnings || []);
  renderMetrics(data);
  renderStrategyOverview(data);
  renderBacktests(data.backtests || []);
  renderBacktestLogs(data.backtests || []);
  renderTradeActions(data);
  renderPositions(data.positions || []);
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
  const latestBalance = selectPrimaryBalance(data);
  const positions = data.positions || [];
  const positionPnl = positions.reduce((total, row) => total + positionPnL(row).value, 0);
  const activePositions = positions.filter((row) => Number(row.quantity || 0) !== 0);
  const positionSymbols = activePositions.map((row) => row.symbol).slice(0, 3).join(", ");
  const backtests = data.backtests || [];
  const latestBacktest = backtests[0];
  const executionStats = data.execution_stats || {};
  const paperPositions = Number(executionStats.paper_positions || 0);
  const acceptedOrders = Number(executionStats.accepted_orders || 0);
  const blockedOrders = Number(executionStats.risk_halted_orders || 0) + Number(executionStats.risk_rejected_orders || 0);
  const backtestTrades = latestBacktest ? Number(latestBacktest.trade_count || 0) : 0;

  $("accountEquity").textContent = latestBalance ? money(latestBalance.total || latestBalance.usd_value) : "--";
  $("accountMeta").textContent = latestBalance
    ? `${accountLabel(latestBalance.account_id)} / ${latestBalance.asset} / 可用 ${money(latestBalance.free)}`
    : "暂无余额快照";

  $("positionPnL").textContent = signedMoney(positionPnl);
  $("positionPnL").className = tone(positionPnl);
  $("positionPnLMeta").textContent = activePositions.length ? `${activePositions.length} 个持仓合计` : "暂无持仓";

  $("openPositionCount").textContent = number(activePositions.length);
  $("positionSummary").textContent = positionSymbols || "暂无持仓";

  $("latestBacktestReturn").textContent = latestBacktest ? pct(latestBacktest.total_return_pct) : "--";
  $("latestBacktestReturn").className = latestBacktest ? tone(latestBacktest.total_return_pct) : "";
  $("latestBacktestMeta").textContent = latestBacktest
    ? `Run ${latestBacktest.id} / ${strategyDisplayName(latestBacktest)}`
    : "暂无回测";

  $("executionStats").textContent = `${number(paperPositions)} 轮`;
  $("executionStatsMeta").textContent =
    `paper开平仓通过 ${number(acceptedOrders)} 单 / 风控拦截 ${number(blockedOrders)} 单 / 回测交易 ${number(backtestTrades)} 笔`;
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
  $("buyRule").previousElementSibling.textContent = "开多";
  $("sellRule").previousElementSibling.textContent = "开空";
  $("strategyPlainText").textContent =
    `这是一个合约短线 TPSL 趋势策略：用 ${fast} 根 K 线均价判断短线方向，用 ${slow} 根 K 线均价过滤趋势，允许开多或开空，并通过止盈止损退出。`;
  $("buyRule").textContent = `开多：${fast} 均线 > ${slow} 均线且价格上行`;
  $("sellRule").textContent = `开空：${fast} 均线 < ${slow} 均线且价格下行`;
  $("strategyLimit").textContent = `手续费 ${pct((latestBacktest.fee_rate || 0) * 100)}`;
  $("currentSignal").textContent = latestSignal ? actionLabel(latestSignal.action) : "暂无信号";
  $("signalReason").textContent = latestSignal
    ? signalText(latestSignal)
    : "运行 papertrade 后，这里会显示最新开多、开空或观望信号。";
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
        <td class="numeric">${number(row.id)}</td>
        <td>${escapeHTML(strategyDisplayName(row))}</td>
        <td>${escapeHTML(marketLabel(row.market_type))}</td>
        <td>${escapeHTML(row.symbol)}</td>
        <td class="numeric ${tone(row.total_return_pct)}">${pct(row.total_return_pct)}</td>
        <td class="numeric ${tone(row.buy_hold_return_pct)}">${pct(row.buy_hold_return_pct)}</td>
        <td class="numeric ${tone(row.excess_return_pct)}">${pct(row.excess_return_pct)}</td>
        <td class="numeric negative">${pct(row.max_drawdown_pct)}</td>
        <td class="numeric">${number(row.trade_count)}</td>
      </tr>`,
    )
    .join("") || emptyRow(9);
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

function renderTablePreview(table) {
  const columns = (table.columns || []).slice(0, 12);
  $("dbTableMeta").textContent =
    `${table.name} / 共 ${number(table.total_rows)} 行 / 最近 ${number((table.rows || []).length)} 行 / 按 ${table.sort_column} 倒序`;
  $("dbTableHead").innerHTML = `<tr>${columns.map((column) => `<th>${escapeHTML(column)}</th>`).join("")}</tr>`;
  $("dbTableRows").innerHTML = (table.rows || [])
    .map(
      (row) => `<tr>
        ${columns.map((column) => `<td>${escapeHTML(compactCell(row[column]))}</td>`).join("")}
      </tr>`,
    )
    .join("") || emptyRow(Math.max(columns.length, 1));
}

function renderTradeActions(data) {
  const orders = (data.orders || []).map(orderActionCard);
  const signals = (data.signals || []).map((row) => ({
    time: row.signal_time,
    title: `${row.symbol} ${actionLabel(row.action)}`,
    meta: `${row.strategy_id} / 置信度 ${pct(row.confidence * 100)}`,
    value: row.reason || "策略信号",
    tone: row.action === "buy" || row.action === "cover" ? "positive" : row.action === "sell" || row.action === "short" ? "negative" : "",
  }));
  const actions = [...orders, ...signals]
    .sort((a, b) => new Date(b.time).getTime() - new Date(a.time).getTime())
    .slice(0, 8);

  $("tradeActionRows").innerHTML = actions
    .map(
      (row) => `<div class="action-card">
        <div>
          <strong>${escapeHTML(row.title)}</strong>
          <span>${escapeHTML(row.meta)} / ${formatTime(row.time)}</span>
        </div>
        <div class="numeric">
          <strong class="${row.tone}">${escapeHTML(row.value)}</strong>
        </div>
      </div>`,
    )
    .join("") || emptyBlock("暂无买卖动作");
}

function orderActionCard(row) {
  const blocked = row.status === "risk_halted" || row.status === "risk_rejected";
  if (blocked) {
    return {
      time: row.created_at,
      title: `${row.symbol} ${statusLabel(row.status)}`,
      meta: `${marketLabel(row.market_type)} / ${decisionLabel(row.risk_decision)} / 不是成交`,
      value: row.risk_reason || "风控拦截，未提交订单",
      tone: "negative",
    };
  }
  return {
    time: row.created_at,
    title: `${row.symbol} ${sideLabel(row.side)}`,
    meta: `${marketLabel(row.market_type)} / ${statusLabel(row.status)}${row.exchange_order_id ? ` / OneBullEx ${row.exchange_order_id}` : ""}`,
    value: `${money(row.quantity)} @ ${money(row.price)} / TP ${moneyOrDash(row.take_profit_price)} SL ${moneyOrDash(row.stop_price)}${row.exchange_status ? ` / ${row.exchange_status}` : ""}`,
    tone: row.risk_decision === "allow" ? "positive" : "negative",
  };
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
        <div><strong>${escapeHTML(accountLabel(row.account_id))} / ${escapeHTML(row.asset)}</strong><span>${formatTime(row.snapshot_time)}</span></div>
        <div class="numeric"><strong>${money(row.total)}</strong><span>可用 ${money(row.free)}</span></div>
      </div>`,
    )
    .join("") || emptyBlock("暂无余额");
}

function renderPositions(rows) {
  $("positionUpdatedAt").textContent = rows.length ? `更新 ${formatTime(rows[0].snapshot_time)}` : "暂无持仓";
  $("positionRows").innerHTML = positionGroup("合约持仓", rows);
}

function selectPrimaryBalance(data) {
  const balances = data.balances || [];
  if (!balances.length) return null;
  const preferredAccounts = [];
  pushUnique(preferredAccounts, data.runtime && data.runtime.account_id);
  for (const row of data.positions || []) {
    pushUnique(preferredAccounts, row.account_id);
  }
  for (const row of data.orders || []) {
    pushUnique(preferredAccounts, row.account_id);
  }
  ["paper", "paper-live-main", "live-main"].forEach((accountID) => pushUnique(preferredAccounts, accountID));
  for (const accountID of preferredAccounts) {
    const match = balances.find((row) => row.account_id === accountID);
    if (match) return match;
  }
  return balances[0];
}

function pushUnique(values, value) {
  if (value && !values.includes(value)) {
    values.push(value);
  }
}

function positionGroup(title, rows) {
  return `<section class="position-group">
    <div class="position-group-head">
      <strong>${escapeHTML(title)}</strong>
      <span>${rows.length ? `${number(rows.length)} 个持仓` : "暂无"}</span>
    </div>
    ${rows.length ? rows.map(positionCard).join("") : emptyBlock("暂无合约持仓")}
  </section>`;
}

function positionCard(row) {
  const pnl = positionPnL(row);
  const pnlPct = positionPnLPct(row, pnl.value);
  const markSource =
    row.mark_price_source === "latest_mark_price"
      ? "合约标记价"
      : "快照价";
  const snapshotLabel = row.snapshot_stale ? "账户快照已过期" : "账户快照";
  return `<div class="position-card">
        <div class="position-head">
          <div>
            <strong>${escapeHTML(row.symbol)} ${escapeHTML(positionSideLabel(row.position_side))}</strong>
            <span>${escapeHTML(marketLabel(row.market_type))} / ${escapeHTML(row.margin_mode || "cross")} / ${snapshotLabel} ${formatTime(row.snapshot_time)}</span>
          </div>
          <div class="position-pnl numeric">
            <strong class="${tone(pnl.value)}">${signedMoney(pnl.value)}</strong>
            <span>${pnl.estimated ? "估算盈亏" : "未实现盈亏"} ${formatPctValue(pnlPct)}</span>
          </div>
        </div>
        <div class="position-detail">
          ${positionMetric("数量", quantity(row.quantity))}
          ${positionMetric("开仓价", `${moneyOrDash(row.entry_price)} / 账户快照`)}
          ${positionMetric("标记价", `${moneyOrDash(row.mark_price)} / ${markSource}`)}
          ${positionMetric("行情时间", formatTime(row.mark_price_time || row.snapshot_time))}
          ${positionMetric("强平价", moneyOrDash(row.liquidation_price))}
          ${positionMetric("杠杆", leverage(row.leverage))}
          ${positionMetric("名义价值", moneyOrDash(row.notional))}
          ${positionMetric("强平距离", pct(row.liquidation_distance_pct))}
          ${positionMetric("账户", accountLabel(row.account_id))}
        </div>
      </div>`;
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

function compactCell(value) {
  const text = String(value ?? "");
  if (text.length <= 80) {
    return text;
  }
  return `${text.slice(0, 77)}...`;
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
    risk_halted: "风控熔断",
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

function accountLabel(value) {
  if (!value) return "--";
  if (value === "paper") return "paper 模拟账户";
  if (value === "paper-live-main") return "真实行情 dry-run";
  if (value === "live-main") return "真实账户";
  return value;
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
  const fast = row.fast_window || parseStrategyWindow(row.strategy_name, 0);
  const slow = row.slow_window || parseStrategyWindow(row.strategy_name, 1);
  if (fast && slow) {
    return `SMA ${fast}/${slow} 均线交叉`;
  }
  return row.strategy_name || "--";
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
