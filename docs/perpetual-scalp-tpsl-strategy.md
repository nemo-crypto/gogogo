# BTCUSDT 永续合约纸面策略说明

> 更新时间：2026-07-15
> 当前策略：`perp-trend-scalp-v2-paper`  
> 当前模式：paper trading，本地模拟成交，不是真实交易所实盘下单。
> v2 更新：已加入趋势强度、确认 K 线、ATR 波动率、成交量过滤，以及按账户权益、可用余额、保证金占用和强平距离计算 paper 下单数量。<br>
> v3 草案：前期小资金允许使用专门档位做更主动的合约趋势短线，但仍必须保留强平距离、单笔风险、保证金占用和交易频率约束。
> v4 实装：默认参数已统一为合约短线趋势策略；paper 执行价优先使用 OneBullEx mark price；回测 TP/SL 改为 high/low 盘中触发；开仓风控增加资金费率、最小数量、数量步长和价格 tick 检查。<br>
> v5 实装：默认信号周期从 `1m` 降为 `5m`，`aggressive` 档也使用 OneBullEx 稳定支持的 `5m`；止盈止损改为 ATR 动态区间，避免固定小区间被手续费和噪音吞掉。
> v6 实装：OneBullEx live 提交路径会把本地计算的止盈价和止损价作为交易所原生保护字段提交；实盘开关仍默认关闭。
> v7 实装：增加第一阶段信号质量过滤器，按特征给候选开仓打分，低分信号先观望并把特征写入 `signals.raw_features_json`。
> v8 第一阶段实装：新增 `15m/1h` 高周期趋势过滤、日亏损/连续亏损硬熔断接线、保本止损、ATR trailing stop、`paper_positions.close_reason` 平仓原因入库；完整增强路线见 `docs/perpetual-trend-risk-enhancement-plan.md`。
> v9 实装：新增 `small-scalp` 小资金短线档，面向约 300U 账户降低单笔风险和保证金占用，同时放宽 5m 入场阈值以提升短线触发频率。
> v10 实装：新增 `small-scalp-fast` 5m 快进快出档，继续保留 15m 趋势硬过滤，但降低成交量、趋势差、追价距离和评分门槛。
> v11 实装：新增当前默认 `micro-trend-1m` 1 分钟短线趋势档，执行信号切到 `1m fast=2/slow=5`，趋势硬过滤改为 `5m EMA8/21`，进一步降低入场门槛但保留小资金风控边界。

## 1. 当前实际参数

| 项目 | 当前值 | 说明 |
| --- | --- | --- |
| 市场 | `perpetual` | USDT 本位永续合约 |
| 交易对 | `BTCUSDT` | 当前只保留合约策略测试 |
| 周期 | 基础默认 `5m`；当前一键推荐 `micro-trend-1m` 为 `1m` | `small-scalp-fast` / `small-scalp` / `aggressive` 仍为 `5m`；仍可用 `-interval` 手动覆盖 |
| 策略类型 | `scalp-tpsl` | 短线均线 + ATR 动态止盈止损 |
| 快均线 | 基础 `3`；`micro-trend-1m` 为 `2` | 最近 N 根 K 线收盘均价 |
| 慢均线 | 基础 `9`；`micro-trend-1m` 为 `5` | 最近 N 根 K 线收盘均价 |
| 固定止盈兜底 | `0.80%` | `-dynamic-tpsl=false` 时使用 |
| 固定止损兜底 | `0.45%` | `-dynamic-tpsl=false` 时使用 |
| 动态止盈 | `ATR * 1.6`，区间 `0.55%~1.40%` | `micro-trend-1m` 为 `ATR*1.10`、区间 `0.25%~0.80%`；5m 小资金档为 `ATR*1.35`、区间 `0.45%~1.20%` |
| 动态止损 | `ATR * 1.0`，区间 `0.30%~0.75%` | `micro-trend-1m` 为 `ATR*0.75`、区间 `0.15%~0.45%`；5m 小资金档为 `ATR*0.90`、区间 `0.25%~0.65%` |
| 保本止损 | 默认开启，`breakeven_trigger_r=1.0` | 浮盈达到 1R 后，把止损移动到手续费调整后的保本价 |
| ATR 追踪止损 | 默认开启，`trailing_activation_r=1.5` / `trailing_atr_mult=1.2` | 浮盈达到 1.5R 后，只向盈利方向移动止损 |
| 冷却 | `1` 根 K 线 | 平仓后至少等 1 根 K 线 |
| 手续费 | `0.0005` | 回测/模拟成本参数 |
| 滑点 | `0.0005` | 回测/模拟成本参数 |
| 下单数量 | 动态优先，固定兜底 `0.001 BTC` | 启用 `risk_pct` 后不再固定数量 |
| 风险仓位 | 默认 `risk_pct=1` | 按单笔止损风险占账户权益比例反推数量；`micro-trend-1m` 为 `0.5`，5m 小资金档为 `0.8`，`aggressive` 为 `2` |
| 名义仓位上限 | `max_notional_pct` | 限制单个交易对最大名义敞口 |
| 保证金上限 | `max_margin_pct` | 限制账户总初始保证金占权益比例 |
| 可用余额占用 | `max_balance_use_pct` | 限制单笔订单最多吃掉多少可用余额 |
| 强平距离 | `min_liquidation_distance_pct` | 预估强平价太近时拒绝开仓 |
| 趋势强度过滤 | `min_trend_spread_pct=0.03` | 过滤快慢均线距离太小的噪音信号 |
| 高周期趋势过滤 | 默认开启，`15m + 1h` | `micro-trend-1m` 为 `5m EMA8/21`；5m 小资金档为 `15m EMA8/21`；基础/aggressive 为 `15m EMA20/60 + 1h EMA20/60` |
| 日亏损熔断 | `max_daily_loss_pct=2` | 当天已实现亏损达到阈值后拒绝新开仓 |
| 连续亏损熔断 | `max_consecutive_losses=3` | 连续亏损达到阈值后拒绝新开仓 |
| 确认 K 线 | `confirm_bars=1` | 要求连续 N 根 K 线同方向确认 |
| ATR 过滤 | `atr_window=14` / `min_atr_pct=0.08` / `max_atr_pct=1.6` | 波动太小不做，波动太大也不追 |
| 成交量过滤 | `volume_window=20` / `min_volume_ratio=1.10` | 当前成交量需要高于均量倍数 |
| 追价距离过滤 | `max_entry_extension_pct=0.18` | 价格离快均线太远时不追单 |
| 回踩确认 | `pullback_lookback=5` / `pullback_tolerance_pct=0.06` | 顺趋势开仓前，最近 N 根 K 线必须回踩或反抽到快均线附近 |
| 杠杆 | 默认 `1.0x`，`aggressive` / 小资金档 `3.0x` | 小资金档通过更低保证金/单笔风险上限控制小账户占用 |
| 执行价格 | OneBullEx `mark price` 优先 | K 线 close 只负责出信号；默认要求存在 mark price |
| K 线新鲜度 | 基础 `max_candle_age=7m`；`micro-trend-1m` 为 `2m` | 适配执行信号周期；旧参数 `max_market_data_age` 仍可覆盖 |
| 标记价新鲜度 | `max_mark_price_age=45s` | mark price 太旧会拒绝本轮运行 |
| 资金费率 | `max_abs_funding_rate_pct=0.05` | 开仓风控会读取最新 funding rate；超过阈值拒单 |
| 交易规则 | `min_order_quantity=0.001` / `quantity_step=0.001` / `price_tick_size=0.1` | 开仓前先做最小数量、数量步长、价格 tick 对齐和校验 |
| 原生保护单 | live 开仓时随单提交 | 使用 OneBullEx `triggerProfitPrice`、`triggerStopPrice`；paper 仍本地结算 |
| 信号评分过滤 | `signal_filter=true` / `min_signal_score=0.55` | `micro-trend-1m` 阈值为 `0.35`，`small-scalp-fast` 为 `0.40`，`small-scalp` 为 `0.45`，`aggressive` 为 `0.50` |
| 轮询频率 | 约 `15s` | 每 15 秒跑一次策略 |

当前参数来自 `strategy_runs.config_json` 和持仓快照，代码入口见 `cmd/papertrade/main.go`。

## 2. 策略目前有哪些功能

当前已经支持：

- 读取本地真实 OneBullEx 合约 K 线；当前默认 `micro-trend-1m` 使用 1m 执行 K 线，`aggressive`、`small-scalp` 和 `small-scalp-fast` 档使用 5m。
- paper 策略执行价优先读取 OneBullEx mark price；K 线 close 只负责信号判断。
- 对最新 K 线 close 和 mark price 分别做新鲜度检查，避免执行周期 K 线被 mark price 阈值误杀。
- 读取最新资金费率，并在合约开仓风控中检查 `max_abs_funding_rate_pct`。
- 允许合约开多。
- 允许合约开空。
- 每次开仓按当时 ATR 计算本笔持仓的止盈价和止损价，并写入持仓。
- 每轮策略运行前先检查已有持仓是否触发止盈/止损。
- 每轮策略运行前会检查趋势反转，多单遇到 `fast < slow`、空单遇到 `fast > slow` 会主动平仓。
- paper 未实现盈亏和已实现盈亏已按开仓、平仓双边手续费/滑点估算。
- 开仓前可以过滤弱趋势、低波动、异常高波动、低成交量信号。
- 开仓前可以过滤离快均线过远的追价信号，并要求最近有回踩/反抽确认。
- 开仓前会计算信号质量分 `signal_score`，低于阈值的候选开仓会被改成观望；特征、候选动作、最终动作和过滤原因会写入 `signals.raw_features_json`。
- 使用 `risk_pct` 按账户权益和止损距离动态计算数量，并用 `max_notional_pct`、`max_margin_pct`、`max_balance_use_pct` 同时压仓位。
- 开仓前会估算强平价，强平距离小于 `min_liquidation_distance_pct` 时拒单。
- 开仓前会把数量按 `quantity_step` 向下对齐，并校验 `min_order_quantity`；价格、止盈、止损会按 `price_tick_size` 对齐。
- live 开仓单会把止盈价、止损价同步传给 OneBullEx 下单接口，使用交易所原生保护字段。
- 回测 TP/SL 已使用 K 线 high/low 判断盘中触发；同一根 K 同时触发 TP 和 SL 时，按保守规则先算止损。
- paper 账户权益已累计历史已平仓净盈亏，并叠加当前持仓浮盈亏。
- 订单风控事件会记录订单风险、订单保证金、总保证金和可用余额。
- 同一策略同一交易对只允许一个打开的纸面持仓。
- 每轮都保存信号、订单、持仓、账户、回测和绩效快照。
- 看板默认展示合约 `BTCUSDT`，当前一键策略默认信号周期为 `1m`，并过滤掉现货交易噪音。

当前没有支持：

- 没有减仓。
- 没有分批止盈。
- 没有追踪止盈。
- 没有加仓。
- 没有反手开仓。例如多单止损后，不会在同一轮马上反手开空。
- 默认不开真实交易所下单；`-submit-exchange` 还必须配合 `ONEBULLEX_LIVE_TRADING=true` 才会提交。
- paper 模式不会提交交易所保护单；只有真实提交开关打开后，开仓单才带 OneBullEx 原生保护字段。
- 没有真实交易所账户保证金和资金费率结算；paper 只做本地估算和风控前置检查。
- 没有多人资金份额、申购赎回、跟单滑点分摊和业绩费结算。

## 3. 什么时候开多

当没有打开的纸面持仓时，满足以下条件才开多：

1. 最近 3 根 K 线收盘均价 > 最近 9 根 K 线收盘均价。
2. 最新收盘价 > 上一根收盘价。
3. 趋势强度、ATR、成交量过滤必须全部通过。
4. 追价距离过滤必须通过，最新收盘价不能离快均线太远。
5. 最近 N 根 K 线必须曾经回踩到快均线附近。
6. 当前不在冷却期，且 K 线 close 与 mark price 都没有过期。

开多后：

- 订单方向：`buy`
- 持仓方向：`long`
- 开仓价：OneBullEx 最新 mark price，按 `price_tick_size` 对齐。
- 数量：由账户权益、止损距离、可用余额和保证金上限动态计算；未启用 `risk_pct` 时才用 `quantity`
- 止盈价：开仓价按本轮 ATR 动态上移，默认 `ATR * 1.6`，且限制在 `0.55%~1.40%`
- 止损价：开仓价按本轮 ATR 动态下移，默认 `ATR * 1.0`，且限制在 `0.30%~0.75%`

示例：

如果开多价是 `63900`，本轮动态止盈为 `0.90%`，动态止损为 `0.55%`：

- 止盈价 = `63900 * 1.009 = 64475.1`
- 止损价 = `63900 * 0.9945 = 63548.55`

## 4. 什么时候开空

当没有打开的纸面持仓，并且市场是 `perpetual` 时，满足以下条件才开空：

1. 最近 3 根 K 线收盘均价 < 最近 9 根 K 线收盘均价。
2. 最新收盘价 < 上一根收盘价。
3. 趋势强度、ATR、成交量过滤必须全部通过。
4. 追价距离过滤必须通过，最新收盘价不能离快均线太远。
5. 最近 N 根 K 线必须曾经反抽到快均线附近。
6. 当前不在冷却期，且 K 线 close 与 mark price 都没有过期。

开空后：

- 订单方向：`sell`
- 持仓方向：`short`
- 开仓价：OneBullEx 最新 mark price，按 `price_tick_size` 对齐。
- 数量：由账户权益、止损距离、可用余额和保证金上限动态计算；未启用 `risk_pct` 时才用 `quantity`
- 止盈价：开仓价按本轮 ATR 动态下移，默认 `ATR * 1.6`，且限制在 `0.55%~1.40%`
- 止损价：开仓价按本轮 ATR 动态上移，默认 `ATR * 1.0`，且限制在 `0.30%~0.75%`

示例：

如果开空价是 `63748.1`，本轮动态止盈为 `0.90%`，动态止损为 `0.55%`：

- 止盈价 = `63748.1 * 0.991 = 63174.3671`
- 止损价 = `63748.1 * 1.0055 = 64098.71455`

## 5. 什么时候平仓

当前 paper trading 实时循环按以下顺序检查平仓：

1. 先用最新 mark price 检查本笔持仓已经写入的动态止盈/止损。
2. 如果没有触发止盈/止损，再检查趋势反转。
3. 触发后整仓关闭，暂不做分批减仓。

### 多单平仓

多单触发条件：

- 当前价格 >= 止盈价：触发 `take_profit`
- 当前价格 <= 止损价：触发 `stop_loss`
- 快均线 < 慢均线：触发 `trend_reversal`

多单盈亏计算：

```text
毛盈亏 = (当前价格 - 开仓价) * 数量
净盈亏 = 毛盈亏 - 开仓名义价值 * (手续费 + 滑点) - 当前名义价值 * (手续费 + 滑点)
```

### 空单平仓

空单触发条件：

- 当前价格 <= 止盈价：触发 `take_profit`
- 当前价格 >= 止损价：触发 `stop_loss`
- 快均线 > 慢均线：触发 `trend_reversal`

空单盈亏计算：

```text
毛盈亏 = (开仓价 - 当前价格) * 数量
净盈亏 = 毛盈亏 - 开仓名义价值 * (手续费 + 滑点) - 当前名义价值 * (手续费 + 滑点)
```

## 6. 回测和平仓逻辑对齐状态

回测函数和实时 paper 现在都包含 ATR 动态止盈止损和趋势反转平仓。实时 paper 使用最新 mark price 判断 TP/SL；回测使用每根 K 线的 high/low 判断盘中是否触发 TP/SL。

多单：

- K 线 high >= 止盈价：平仓。
- K 线 low <= 止损价：平仓。
- 快均线 < 慢均线：平仓。

空单：

- K 线 low <= 止盈价：平仓。
- K 线 high >= 止损价：平仓。
- 快均线 > 慢均线：平仓。

如果同一根 K 线同时触发止盈价和止损价，回测会按保守规则优先计算止损，避免高估短线策略收益。

注意：回测在历史 K 线里用下一根价格模拟趋势反转后的成交；实时 paper 无法知道下一根价格，所以用当前轮最新价格平仓。这一点仍会造成轻微口径差异，但退出规则已经一致。

## 7. 开仓额度怎么计算

当前 paper 合约策略先按单笔风险预算算出理论数量，再逐层压缩到风控允许范围内。

```text
账户权益 = 初始权益 + 历史已实现净盈亏 + 当前持仓未实现净盈亏
风险预算 = 账户权益 * risk_pct / 100
理论数量 = 风险预算 / abs(开仓价 - 止损价)

名义仓位上限数量 = 账户权益 * max_notional_pct / 100 / 开仓价
可用余额上限数量 = 可用余额 * max_balance_use_pct / 100 * 杠杆 / 开仓价
保证金上限数量 = (账户权益 * max_margin_pct / 100 - 当前初始保证金) * 杠杆 / 开仓价

最终数量 = min(理论数量, 名义仓位上限数量, 可用余额上限数量, 保证金上限数量)
```

如果最终数量小于等于 0，本轮不开仓并返回错误。即使数量先被压缩，订单入库前还会再经过 `risk.EvaluateOrder`，只要风控结论不是 `allow`，就只记录拒单，不会创建 paper 持仓。

强平价目前是 paper 估算：

```text
多单强平价 ~= 开仓价 * (1 - 1 / 杠杆 + 维护保证金率)
空单强平价 ~= 开仓价 * (1 + 1 / 杠杆 - 维护保证金率)
强平距离 = abs(开仓价 - 强平价) / 开仓价 * 100
```

接真实交易所 API 后，强平价必须优先使用交易所返回值，本地公式只能做兜底估算。

## 8. 什么时候减仓

目前没有减仓逻辑。

当前实现是：

1. 没仓时，满足信号后按动态仓位一次性开仓。
2. 有仓时，不再开新仓。
3. 到止盈或止损时，整仓关闭。

也就是说，现在仍是整进整出，但不再必须固定仓位。

## 9. 当前数据库里的最近状态示例

最近合约纸面持仓曾出现：

| 持仓方向 | 状态 | 数量 | 开仓价 | 止损价 | 止盈价 |
| --- | --- | --- | --- | --- | --- |
| `long` | 示例 | `0.001` | `63900.0` | `63548.55` | `64475.1` |
| `short` | 示例 | `0.001` | `63748.1` | `64098.71455` | `63174.3671` |

这些数据来自 `paper_positions` 和 `orders` 表。

## 10. 代码位置

| 逻辑 | 文件 |
| --- | --- |
| CLI 参数、杠杆、数量、周期、止盈止损和仓位风控参数 | `cmd/papertrade/main.go` |
| 实时循环，每 15 秒跑策略 | `cmd/papertrade/main.go` |
| 开仓信号：开多/开空 | `cmd/papertrade/main.go` |
| 止盈止损价格计算 | `cmd/papertrade/main.go` |
| 信号质量特征和评分过滤 | `cmd/papertrade/main.go` |
| 实时 paper 持仓止盈止损和趋势反转检查 | `cmd/papertrade/main.go` |
| 持仓净盈亏计算，含手续费和滑点 | `cmd/papertrade/main.go` |
| 回测里的趋势反转平仓 | `internal/backtest/model.go` |
| 开仓前风控：风险、敞口、保证金、可用余额、强平距离 | `internal/risk/model.go` |
| 持仓、订单、账户快照、风控事件入库 | `internal/portfolio/sqlite_repository.go`、`internal/execution/sqlite_repository.go` |
| live 下单原生保护字段映射 | `internal/execution/sqlite_repository.go`、`internal/exchange/onebullex/client.go` |
| 看板合约过滤和实时展示 | `internal/dashboard/server.go`、`internal/dashboard/static/app.js` |

关键代码入口：

- `cmd/papertrade/main.go:27`：paper 基础默认参数，包括 `5m` 周期、动态 TP/SL、风险仓位和行情新鲜度；profile 可切到 `micro-trend-1m`。
- `cmd/papertrade/main.go:108`：命令行参数，包括 `-dynamic-tpsl`、ATR 倍数、上下限、杠杆和仓位风控。
- `cmd/papertrade/main.go:493`：每轮策略执行入口。
- `cmd/papertrade/main.go:802`：开仓前计算本笔有效 TP/SL，并写入订单和持仓。
- `cmd/papertrade/main.go:1288`：已有持仓按数据库中的止盈/止损价检查退出。
- `cmd/papertrade/main.go:1329`：实时 paper 使用 ATR 动态计算开仓 TP/SL。
- `cmd/papertrade/main.go:1716`：K 线和 mark price 分开做新鲜度检查。
- `cmd/papertrade/main.go`：信号质量评分过滤器，输出 `signal_score`、`signal_allowed` 和 `signal_features`。
- `internal/execution/sqlite_repository.go:121`：提交交易所订单时映射原生止盈止损字段。
- `internal/exchange/onebullex/client.go:382`：OneBullEx 下单请求体构造。
- `internal/backtest/model.go:182`：回测版 `scalp-tpsl` 策略。
- `internal/backtest/model.go:364`：给实时 paper 复用的最新动态 TP/SL 百分比计算。

## 11. 当前策略风险和待完善点

当前策略偏简单，适合先做 paper trading 跑通链路，但不适合直接大资金实盘。

建议后续优先补：

- 减仓：例如盈利到 0.2% 先平一半，剩余仓位移动止损。
- 移动止损：盈利后把止损移动到开仓价附近，减少回撤。
- 反手规则：止损后是否允许反向开仓，需要独立判断，不能盲目反手。
- 波动率过滤：波动过小时不交易，波动过大时降低仓位。
- 成交量过滤：只在成交量放大时开仓。
- 资金费率过滤：资金费率异常时暂停开多或开空。
- 最大连续亏损熔断：底层风控已有字段，paper 策略还需要按真实交易历史持续更新连续亏损计数。
- 真正 reduce-only 平仓单：接真实交易所时必须防止平仓单变成反向开仓。

## 11.1 面向跟单资金的底线

如果后续有多人申请加入资金，这个系统不能只看“策略信号”。必须把它当成资金池/跟单系统来设计：

- 不承诺持续盈利，只展示真实历史收益、回撤、胜率、亏损周期和成本。
- 每个跟单账户必须按权益、可用余额、保证金占用和交易所最小下单规则独立算仓位，不能所有资金共用固定数量。
- 必须有最大日亏损、最大周亏损、最大连续亏损暂停。
- 必须有最大资金容量，资金太大后同一信号会产生更大滑点。
- 必须记录每笔交易的开仓原因、平仓原因、净盈亏、手续费、滑点和跟单账户分摊结果。

当前 v2 已完成单账户 paper 层面的动态仓位和保证金约束。资金份额、跟单队列、业绩报表、真实 API 账户同步和合规模块还没做。

## 11.2 小资金短线档位

前期资金量在 300U 左右且希望更快出现短线成交时，默认优先使用 `micro-trend-1m`。它不是放开所有风控，而是把执行信号切到 1 分钟级别，并用 `5m EMA8/21` 做趋势硬过滤；强平距离、单笔风险、保证金占用、可用余额占用和连续亏损暂停仍然保留。`small-scalp-fast` 是稍稳一点的 5m 快速档，`small-scalp` 更保守，`aggressive` 仍保留为 paper/backtest 研究档。

当前默认推荐的 `-profile micro-trend-1m` 含义是：

| 参数 | micro-trend-1m 值 | 设计目的 |
| --- | --- | --- |
| 市场 | `perpetual` | 只测合约，多空都允许 |
| 周期 | `1m` | 用 1 分钟已闭合 K 线做执行信号，目标是更高触发频率 |
| 均线 | `fast=2` / `slow=5` | 更快捕捉 1m 短线趋势 |
| 止盈/止损 | ATR 动态 | 止盈 `ATR*1.10`，区间 `0.25%~0.80%`；止损 `ATR*0.75`，区间 `0.15%~0.45%` |
| 杠杆 | `3x` | 小资金提高资金利用率，但受保证金上限约束 |
| 单笔风险 | `risk_pct=0.5%` | 1m 交易频率更高，单笔风险比 5m fast 档更低 |
| 单笔风险硬上限 | `max_order_risk_pct=0.8%` | 防止参数错误导致单笔亏损失控 |
| 名义仓位上限 | `max_notional_pct=150%` | 限制单交易对敞口，避免小账户一笔过重 |
| 保证金上限 | `max_margin_pct=45%` | 给补保证金、手续费和异常波动留空间 |
| 可用余额占用 | `max_balance_use_pct=75%` | 不把可用余额一次打满 |
| 强平距离 | `min_liquidation_distance_pct=15%` | 杠杆提高后必须留安全距离 |
| 执行趋势强度 | `min_trend_spread_pct=0.004%` | 适配 1m 较小均线差，减少长期观望 |
| 高周期趋势 | `5m EMA8/21`，`trend_min_spread_pct=0.005%` | 保留方向过滤，不放开逆 5m 趋势 |
| ATR 过滤 | `atr_window=14` / `0.02%-1.6%` | 放宽低波动门槛，波动异常大仍不追 |
| 成交量过滤 | `volume_window=20` / `min_volume_ratio=0.50` | 1m 成交量噪音更大，不要求高于均量，但极低流动性仍过滤 |
| 追价过滤 | `max_entry_extension_pct=0.50%` | 放宽快速突破/急跌后的短线入场，但仍避免极端追价 |
| 回踩确认 | `pullback_lookback=1` / `pullback_tolerance_pct=0.10%` | 1m 只要求最近一根有轻微回踩/反抽 |
| 信号评分 | `min_signal_score=0.35` | 更容易通过评分过滤，但低质信号仍会观望 |
| K 线新鲜度 | `max_candle_age=2m` | 1m 信号必须足够新，避免行情同步卡住后继续按旧 candle 决策 |

启动示例：

```bash
go run ./cmd/papertrade \
  -dsn data.db \
  -account paper \
  -profile micro-trend-1m \
  -symbol BTCUSDT \
  -equity 300 \
  -watch \
  -poll-interval 15s
```

手动传入的参数优先级高于 profile。例如 `-profile micro-trend-1m -leverage 2` 会保留 `2x`，不会被 profile 覆盖成 `3x`。兼容别名：`micro`、`micro-trend`、`1m`、`scalp-1m`、`minute-scalp` 都会归一化为 `micro-trend-1m`；`300u`、`small-fast`、`fast-scalp`、`micro-scalp-fast` 仍归一化为 5m `small-scalp-fast`；`small`、`small-scalp`、`small-capital`、`micro-scalp` 仍归一化为更稳的 `small-scalp`。

`small-scalp-fast` 的主要差异是回到 `5m fast=3/slow=9`、`15m EMA8/21` 趋势过滤、`risk_pct=0.8%`、`max_order_risk_pct=1.2%`、`min_signal_score=0.40`、`min_volume_ratio=0.80`、`max_entry_extension_pct=0.35%`，成交频率低于 1m 档但噪音更少。`small-scalp` 的主要差异是 `min_trend_spread_pct=0.015%`、`min_volume_ratio=1.00`、`max_entry_extension_pct=0.22%`、`pullback_lookback=3`、`min_signal_score=0.45`、`trend_min_spread_pct=0.02%`，成交频率会低于 fast 档。`aggressive` 研究档主要差异是 `risk_pct=2%`、`max_order_risk_pct=2.5%`、`max_notional_pct=220%`、`max_margin_pct=65%`、`max_balance_use_pct=90%`、`min_signal_score=0.50`，不建议 300U 起步直接作为真实默认档。

这个模式必须遵守以下降档条件：

1. 单日净亏损达到 `4%`，当天停止 micro-trend-1m/small-scalp-fast/small-scalp/aggressive 档。
2. 连续亏损 `3` 笔，暂停至少 30 分钟。
3. 最大回撤超过 `8%`，切回 `1x-2x` 和 `risk_pct <= 1%`。
4. 连续 paper trading 少于 2 周，不允许接真实跟单资金。
5. 一旦进入多人跟单，默认不能使用 micro-trend-1m/small-scalp-fast/small-scalp/aggressive 档，必须按每个跟单账户独立降杠杆、降风险。

## 12. 仔细分析后的核心问题

当前策略最大的问题不是“不能开多开空”，而是信号、成本、退出和风控都还比较粗。它已经能跑通合约 paper trading 链路，但离可以稳定评估收益还有距离。

### 12.1 已处理：固定小 TP/SL 被手续费和滑点吃掉

旧参数问题：

- 止盈：`0.35%`
- 止损：`0.2%`
- 手续费：`0.0005`
- 滑点：`0.0005`
- 回测单边成本：`0.1%`
- 一进一出成本：约 `0.2%`

表面看：

```text
毛止盈 / 毛止损 = 0.35 / 0.2 = 1.75
```

但扣掉一进一出成本后，大致变成：

```text
净止盈 ≈ 0.35% - 0.2% = 0.15%
净止损 ≈ 0.2% + 0.2% = 0.4%
净盈亏比 ≈ 0.15 / 0.4 = 0.375
```

这意味着旧策略必须有很高胜率才可能覆盖成本。对 1m 短线来说，这个要求偏苛刻。

当前处理：

- 基础默认周期仍为 `5m`，当前小资金高频档通过 `micro-trend-1m` 切到 `1m`，并用 `5m EMA8/21` 降低纯 1m 噪音。
- 止盈止损改为 ATR 动态区间；`micro-trend-1m` 为止盈 `0.25%~0.80%`、止损 `0.15%~0.45%`，基础默认仍为止盈 `0.55%~1.40%`、止损 `0.30%~0.75%`。
- 回测和 paper 都使用开仓时的 ATR 计算本笔持仓 TP/SL。
- paper 已支持保本止损和 ATR trailing stop。
- 下一步再做分批止盈。

### 12.2 已处理一部分：1m SMA 3/9 太容易被噪音触发

当前开仓条件只看：

- `3` 根均线和 `9` 根均线的大小关系；
- 最新收盘价是否比上一根高或低。

这个信号非常敏感，在横盘震荡里容易反复开仓、止损、再开仓。特别是在 BTC 1m K 线上，很多价格变化只是噪音，不代表真实趋势。

当前处理：

- 当前小资金默认信号周期已切到 `1m`（`micro-trend-1m`），`aggressive`、`small-scalp` 和 `small-scalp-fast` 仍保持 `5m`。
- 已有趋势强度过滤、成交量过滤、ATR 过滤、追价距离过滤和回踩确认。
- 已支持高周期趋势过滤，`micro-trend-1m` 用 `5m EMA8/21`，5m 小资金档用 `15m EMA8/21`，基础/aggressive 用 `15m + 1h`。

### 12.3 实时 paper 和回测退出逻辑已经基本对齐

当前回测和实时 paper 都有趋势反转平仓：

- 多单遇到 `fast < slow` 可以平仓；
- 空单遇到 `fast > slow` 可以平仓。

剩余差异在成交价口径：

- 回测趋势反转用下一根 K 线价格模拟成交；
- 实时 paper 用当前轮最新价格平仓；
- 这更接近实时系统能做到的行为，但和历史回测仍不会完全一模一样。

优化方向：

- 后续可以给实时 paper 增加更明确的成交模型，例如用 mark price、bid/ask 或下一轮确认价。
- 可以在 trade log 中记录 `exit_reason=trend_reversal` 和实际成交价，便于复盘。

### 12.4 看板盈亏已经计入手续费和滑点，但还没计入资金费率

当前 paper 持仓盈亏按净盈亏估算：

```text
净盈亏 = 毛盈亏 - 开仓成本 - 平仓成本
成本 = 名义价值 * (手续费 + 滑点)
```

现在 realized PnL 和 unrealized PnL 都会扣除开仓、平仓两边的手续费/滑点估算。剩余差距主要是：

- 还没有资金费率成本；
- 还没有真实 bid/ask spread；
- 还没有真实成交失败、部分成交、盘口冲击。

优化方向：

- 合约持仓增加资金费率成本字段。
- 行情同步增加 best bid/ask，用于更真实地估算成交和滑点。

### 12.5 仓位已经动态化，但还需要交易所规则校验

当前 paper 已按 `risk_pct`、止损距离、可用余额和保证金上限动态计算仓位，不再必须固定 `0.01 BTC`。

仍然缺的部分：

- 还没有读取交易所 `stepSize`、`minQty`、`minNotional` 来做数量取整；
- 还没有按真实账户仓位模式区分单向持仓、双向持仓；
- 还没有按真实 leverage bracket 计算维护保证金；
- 跟单账户还没有各自独立的订单拆分和滑点分摊。

优化方向：

```text
最终下单数量 = 本地风险数量 -> 交易所最小数量/步进取整 -> 再做一次交易所账户风控校验
```

接 API 后，任何真实下单前都必须重新拉取账户余额、持仓、强平价和交易规则，不能只信本地缓存。

### 12.6 已有强平距离估算，但还没有资金费率和盘口价差保护

当前是合约策略，但合约特有风险还没有真正进入策略判断：

- 资金费率过高时，做多或做空成本会变大；
- mark price 和 last price 可能短时偏离；
- 高杠杆时强平距离必须使用交易所返回值复核；
- 真实交易所平仓必须使用 reduce-only 防止反向开仓。

普通 paper 档仍建议保持 `1x` 到 `2x`。300U 左右小资金短线当前优先用 `micro-trend-1m` 测试 `3x`，如果噪音过多再降到 `small-scalp-fast`；`aggressive` 仅用于 paper/backtest 研究。所有小资金档都必须同时打开强平距离、保证金占用、单笔风险和连续亏损暂停。进入真实交易或多人跟单前，应优先降到 `1x-2x`，并使用交易所返回的 leverage bracket、账户模式、reduce-only 平仓和异常熔断。

## 13. 建议的优化优先级

### P0：先修正评估口径

这些属于必须先做，否则后续参数优化容易误判。

| 优化项 | 原因 | 建议 |
| --- | --- | --- |
| 统一回测和 paper 退出逻辑 | 已完成基础对齐 | 后续细化成交价模型 |
| paper 盈亏扣除手续费/滑点 | 已完成基础估算 | 后续加入资金费率和 bid/ask spread |
| 记录每笔平仓原因 | 后续分析必须知道是止盈、止损还是趋势反转 | `paper_positions` 增加 close_reason 或另建 trade log |
| 策略参数版本化 | 方便对比不同参数结果 | 在 run config 中记录完整参数和版本 |

### P1：提高信号质量

当前信号太简单，容易在震荡里频繁亏损。

| 优化项 | 推荐实现 |
| --- | --- |
| 均线差过滤 | 已支持：`min_trend_spread_pct` |
| 多周期趋势过滤 | 执行周期开仓方向必须和高周期趋势一致 |
| 成交量过滤 | 已支持：`volume_window` + `min_volume_ratio` |
| 波动率过滤 | 已支持：`atr_window` + `min_atr_pct` + `max_atr_pct` |
| 确认 K 线 | 已支持：`confirm_bars` |
| 追价过滤 | 已支持：`max_entry_extension_pct`，离快均线太远不追 |
| 回踩确认 | 已支持：`pullback_lookback` + `pullback_tolerance_pct` |
| 冷却加强 | 止损后冷却 3 到 5 根 K 线，止盈后冷却 1 到 2 根 |

### P2：优化退出策略

ATR 动态 TP/SL 已完成，下一步重点是把盈利单保护住，避免“到过浮盈又回撤止损”。

建议顺序：

1. 盈利达到 `0.6R` 到 `1R` 时平掉 `30%~50%`。
2. 剩余仓位止损移动到开仓价附近。
3. 盈利继续扩大时使用移动止损。
4. 均线反转时不等待止损，主动平仓。

可以先用以下规则：

```text
多单：
- +1R：减仓 50%，止损移动到开仓价
- +1.5R：启动 trailing stop，回撤 0.5R 平剩余
- -1R：整仓止损

空单：
- +1R 方向盈利：减仓 50%，止损移动到开仓价
- +1.5R 方向盈利：启动 trailing stop，反向回撤 0.5R 平剩余
- -1R：整仓止损
```

### P3：合约专用风控

在 1x paper 阶段可以先记录，真实交易前必须实现。

| 风控项 | 建议 |
| --- | --- |
| 最大日亏损 | 当日亏损达到 `1%` 到 `2%` 暂停策略 |
| 连续亏损 | 连续 `3` 笔亏损暂停 30 到 60 分钟 |
| 最大仓位 | 单策略名义仓位不超过权益的 `1x` 到 `2x` |
| 资金费率 | 资金费率绝对值过高时不开仓 |
| 强平距离 | 小于安全阈值时禁止加仓或开仓 |
| reduce-only | 所有平仓单必须 reduce-only |

## 14. 参数优化建议

当前参数不建议直接认定为有效参数，只能算跑通链路的初始参数。

### 14.1 可以优先测试的参数组合

| 组合 | 周期 | fast | slow | TP/SL | 适用假设 |
| --- | --- | --- | --- | --- | --- |
| 当前小资金默认 | 1m | 2 | 5 | ATR `1.10R/0.75R`，TP `0.25%~0.80%`，SL `0.15%~0.45%` | 推荐 `micro-trend-1m`，适合 300U 左右 paper/小额灰度的高频短线趋势 |
| 5m 小资金快进快出 | 5m | 3 | 9 | ATR `1.35R/0.90R`，TP `0.45%~1.20%`，SL `0.25%~0.65%` | `small-scalp-fast`，比 1m 档更稳但交易更少 |
| 基础默认 | 5m | 3 | 9 | ATR `1.6R/1.0R`，TP `0.55%~1.40%`，SL `0.30%~0.75%` | 降噪短线趋势 |
| 更稳版本 | 5m | 5 | 20 | ATR `1.5R/1.0R` | 减少噪音交易 |
| 趋势版本 | 15m | 5 | 20 | ATR `1.8R/1.0R` | 更少交易，持仓更久 |
| 快速试错 | 5m | 3 | 12 | ATR `1.45R/0.90R` | 保留短线频率，减少 1m 噪音 |

参数评估不能只看收益率，还要同时看：

- 交易次数是否足够；
- 胜率；
- 平均盈利；
- 平均亏损；
- 手续费占毛利润比例；
- 最大连续亏损；
- 最大回撤；
- 多空分别表现；
- 不同日期区间是否稳定。

### 14.2 不建议的优化方向

短期不建议做：

- 不建议直接加到高杠杆。
- 不建议马丁加仓。
- 不建议亏损后翻倍开仓。
- 不建议在没有 reduce-only 保护前接真实合约平仓。
- 不建议只根据最近几十根 K 线就判断策略有效。
- 不建议只优化某一天的数据，否则容易过拟合。

## 15. 长周期回测和 walk-forward

短线策略上线前不能只看最近 1 天或 1 周。建议至少覆盖 6 到 12 个月，并拆成不同市场环境：

| 区间类型 | 目的 | 建议 |
| --- | --- | --- |
| 单边上涨 | 看多头信号是否能跟上趋势 | 多单收益应明显优于震荡期 |
| 单边下跌 | 看空头信号是否能工作 | 空单收益和回撤必须单独统计 |
| 高波动震荡 | 检查 TP/SL 是否被反复扫损 | 重点看最大连续亏损和手续费占比 |
| 低波动横盘 | 检查过滤器是否能减少无效交易 | 交易次数应自然下降 |
| 资金费率异常期 | 检查 funding 风控是否有效 | 高费率方向不应频繁开仓 |

建议的验证流程：

1. 先同步至少 6 到 12 个月执行周期 K 线；测试 `micro-trend-1m` 时同步 `1m + 5m`，测试 5m 档时同步 `5m + 15m`，必要时再同步 `1h`。
2. 用当前参数跑完整区间回测，只作为基线。
3. 用前 70% 数据做参数搜索，后 30% 数据做 out-of-sample 验证。
4. 再做 walk-forward：每次用最近 N 天选参数，只在后续 M 天验证。
5. 最终只接受在多个时间段都稳定的参数，不接受只在某一小段暴利的参数。

可直接执行的长周期同步示例：

```bash
go run ./cmd/marketsync \
  -dsn data.db \
  -dataset klines \
  -exchange onebullex \
  -market perpetual \
  -symbols BTCUSDT \
  -interval 5m \
  -start 2026-01-01T00:00:00Z \
  -end 2026-07-13T00:00:00Z \
  -limit 1500
```

可直接执行的长周期回测示例：

```bash
go run ./cmd/backtest \
  -dsn data.db \
  -exchange onebullex \
  -market perpetual \
  -symbol BTCUSDT \
  -interval 5m \
  -start 2026-01-01T00:00:00Z \
  -end 2026-07-13T00:00:00Z \
  -strategy-type scalp-tpsl \
  -fast 3,5 \
  -slow 9,12,20 \
  -dynamic-tpsl=true \
  -take-profit-atr-mult 1.4,1.6,1.8 \
  -stop-loss-atr-mult 0.9,1.0,1.1 \
  -fee-rate 0.0005
```

注意：如果本地没有对应区间 K 线，回测不会自动联网拉数据，必须先 `marketsync` 入库。

## 16. 回测数据如何反哺策略

回测数据的价值不是“证明某组参数最赚钱”，而是持续发现什么条件下策略更容易亏钱。建议把回测结果拆成训练样本：

| 数据 | 用途 |
| --- | --- |
| 入场前特征 | 例如趋势强度、ATR、成交量倍率、追价距离、K 线方向、资金费率 |
| 交易结果标签 | 例如是否盈利、最大浮盈、最大浮亏、最终 R 倍数、退出原因 |
| 市场环境标签 | 趋势、震荡、高波动、低波动、资金费率异常 |
| 参数组合 | 记录 fast/slow、ATR TP/SL、过滤器阈值 |
| 交易成本 | 手续费、滑点、资金费率估算 |

第一阶段已落地为启发式信号质量过滤器，先不直接引入训练模型。当前做法是让策略先给出候选开多/开空，再由评分器判断信号质量：

1. 输出当前信号质量分 `signal_score`，范围 `0~1`。
2. 默认低于 `0.55` 的候选开仓跳过，`micro-trend-1m` 档低于 `0.35` 跳过，`small-scalp-fast` 档低于 `0.40` 跳过，`small-scalp` 档低于 `0.45` 跳过，`aggressive` 档低于 `0.50` 跳过。
3. 不做反向交易，只把低分信号改成观望。
4. 每轮把 `candidate_action`、`final_action`、`signal_score`、`signal_allowed`、`signal_filter_reason` 和 `signal_features` 写入 `signals.raw_features_json`。
5. 当前评分版本为 `paper_signal_filter_v1`，特征版本为 `paper_signal_features_v1`。

当前评分器使用的特征包括：

- 趋势强度：`trend_spread_pct`
- ATR 波动：`atr_pct`
- 成交量倍率：`volume_ratio`
- 追价距离：`entry_extension_pct`
- 资金费率绝对值：`funding_abs_pct`
- mark price 和 K 线 close 偏离：`mark_basis_pct`
- 近期回测表现：`excess_return_pct`、`win_rate_pct`、交易次数

后续真正引入机器学习模型时，不建议让模型直接自动下单。更稳的替换方式是让模型继续只做“过滤器”或“参数选择器”：

1. 模型输出 `0~1` 的信号质量分，替换当前启发式 score。
2. 低分信号继续跳过，不直接反向交易。
3. 模型只允许在预设参数集合中选一档，例如保守、默认、激进。
4. 每次模型版本、特征版本、训练区间和验证结果都入库。
5. 新模型必须先跑 out-of-sample 和 paper trading，不能直接替换线上策略。

Go 里可优先采用的库：

| 类型 | 建议 | 用法 |
| --- | --- | --- |
| 数值计算 | `gonum` | 矩阵、统计、优化、回归、基础特征分析 |
| 技术指标 | `go-talib` | 计算 RSI、MACD、ATR、布林带等指标 |
| 传统 ML | `golearn` | 随机森林、KNN、朴素贝叶斯等传统分类器 |
| 深度学习 | `gorgonia` | 神经网络和自动微分；复杂度高，暂不建议第一阶段使用 |
| 数据表处理 | `gota` | 类 dataframe 的特征清洗和离线分析 |

建议本项目第一阶段继续用当前内置评分器收集样本；等 `signals.raw_features_json` 和实际平仓结果足够多，再引入 `gonum + go-talib` 做特征统计、参数网格、walk-forward 和简单逻辑回归/树模型。等交易日志足够多后，再考虑更复杂模型。

## 17. 建议实施顺序

建议按下面顺序做，避免一次改太多导致无法判断效果。

1. 已完成：对齐回测和 paper 退出逻辑，paper 支持趋势反转平仓。
2. 已完成：paper PnL 加入手续费和滑点，避免看板盈亏过于乐观。
3. 已完成：增加均线差、确认 K 线、成交量、ATR 过滤。
4. 已完成：增加 `risk_pct`、`max_notional_pct`、`max_margin_pct`、`max_balance_use_pct`，支持按风险比例和保证金约束计算 paper 仓位。
5. 已完成：订单风控记录订单风险、保证金、可用余额和强平距离相关上下文。
6. 已完成：增加 `micro-trend-1m` / `small-scalp-fast` / `small-scalp` / `aggressive` 参数档、追价距离过滤和回踩确认。
7. 已完成第一阶段：`paper_positions.close_reason` 记录平仓原因；后续可继续扩展独立 trade log。
8. 已完成：改固定 TP/SL 为 ATR 动态 TP/SL。
9. 已完成：live 提交路径随开仓单提交 OneBullEx 原生止盈止损保护字段。
10. 已完成：保本止损和 ATR trailing stop。
11. 已完成：连续亏损暂停和日亏损暂停接入 paper 风控快照。
12. 做参数网格回测、out-of-sample 和 walk-forward 验证。
13. 已完成第一阶段：启发式信号质量过滤器入库特征，用于后续训练 ML 过滤器。
14. 增加分批止盈。
15. 后续建立模型版本表，用 ML 评分替换启发式评分，不直接自动替换策略。
16. 连续 paper trading 至少 2 到 4 周，再考虑小资金真实灰度。

## 18. 下一版建议目标

下一版策略可以命名为：

```text
scalp-tpsl-v2
```

建议目标：

- 保留当前合约短线趋势方向，1m 档必须叠加 5m 趋势过滤；
- 开仓增加均线差、成交量、波动率、追价距离和回踩确认过滤；
- 平仓统一支持止盈、止损、趋势反转；
- PnL 计入手续费和滑点；
- 支持保本止损和 ATR trailing stop；
- 增加部分止盈；
- 保留多周期趋势过滤；1m 档用 `5m EMA8/21`，基础/aggressive 用 `15m + 1h`；
- 接入交易所只读账户 API，用真实 available balance、position risk、leverage bracket 覆盖 paper 估算；
- 看板展示每笔交易的净盈亏和退出原因。

这版完成后，再评估是否需要继续提高交易频率或提高杠杆。当前阶段目标是让 1m 高频短线在保留风控边界的前提下运行，并持续复盘每笔交易的风险收益结构。
