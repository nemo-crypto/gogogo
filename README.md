# gogogo

一个使用 Go 标准库 + SQLite 构建的币圈量化研究原型。目前聚焦 OneBullEx 永续合约短线策略，覆盖本地行情存储、真实行情同步、K 线质量检查、回测数据快照、SMA/TPSL 策略回测、回测报告、账户/仓位快照、paper trading、本地风控检查、dry-run 下单审计、每日报告、SQLite 备份和人工急停开关。

系统默认不会向交易所提交真实订单。已实现 OneBullEx 公开行情、私有余额/仓位快照和受保护的订单接口；只有同时设置 `ONEBULLEX_LIVE_TRADING=true` 与 `SUBMIT_EXCHANGE=true` 才会进入真实下单路径。

## 已实现功能

- 本地 SQLite 数据库：`data.db`
- OneBullEx 公开行情同步：
  - 永续合约 K 线
  - 永续合约资金费率
  - 永续合约标记价格
  - 指数价格、最近成交和订单簿
  - 合约规格和杠杆档位
- 本地数据表：
  - `candles`
  - `trades`
  - `order_books`
  - `funding_rates`
  - `mark_prices`
  - `index_prices`
  - `candle_snapshots`
  - `backtest_runs`
  - `orders`
  - `paper_positions`
  - `risk_events`
  - `balances`
  - `positions`
  - `margin_snapshots`
  - `contract_specs`
  - `leverage_brackets`
  - `account_modes`
  - `strategy_runs`
  - `signals`
  - `performance_snapshots`
- K 线缺口检查和回测数据快照冻结
- Scalp TPSL 合约短线回测，SMA 均线交叉作为基线对比
- 单币对、多参数网格扫描
- 回测结果落库和排名报告
- 基础风控检查：单笔风险、单币种敞口、总敞口、杠杆、日亏损、连续亏损、强平距离、资金费率
- Dry-run 下单日志：计划订单、风控结论、风险事件落库，不触发真实交易
- Paper trading：基于真实行情驱动本地模拟开平仓，支持 `1m` 执行信号、`5m/15m/1h` 趋势过滤、日亏损/连续亏损熔断、保本止损和 ATR 追踪止损
- 可选真实下单：风控通过后提交 OneBullEx 订单，并记录交易所订单号；默认关闭
- 账户/仓位/保证金快照：本地写入 balances、positions、margin_snapshots；OneBullEx 私有 API 只读同步需显式开启
- 每日运行报告、SQLite 备份、人工 emergency halt/resume

## 目录结构

```text
cmd/
  quantdb/          # 初始化本地量化数据表
  marketsync/       # 同步 OneBullEx 永续合约公开行情
  datasnapshot/     # 检查 K 线缺口并冻结回测数据快照
  backtest/         # 运行当前 Scalp TPSL 回测并保存结果
  backtestreport/   # 查询和排序回测结果
  dashboard/        # 本地网页看板，读取 SQLite 量化数据
  riskcheck/        # 本地下单意图风控检查
  dryrunorder/      # 写入 dry-run 订单和风险事件
  accountsnapshot/  # 写入本地账户、仓位、保证金快照
  papertrade/       # 运行 paper trading；显式开启双重开关后可提交真实订单
  dailyreport/      # 汇总订单、风险事件、信号、快照等每日指标
  backupdb/         # 备份 SQLite data.db
  emergency/        # 本地人工急停开关
internal/
  exchange/         # 交易所抽象接口
  dashboard/        # Dashboard HTTP handler 与内嵌静态资源
  exchange/onebullex/ # OneBullEx 行情、签名、只读账户和受保护订单 client
  marketdata/       # 行情/资金费率/标记价格 SQLite 存取
  backtest/         # 回测、报告
  risk/             # 本地风控规则
  execution/        # dry-run 订单日志和风险事件
  portfolio/        # 账户、仓位、保证金快照
  strategy/         # 策略运行、信号和绩效快照
  storage/          # 统一 SQLite schema 初始化和迁移边界
  runtime/          # 本地 emergency halt guard
docs/
  quant-trading-strategy-todo.md
```

## 配置

参考 `.env.example` 设置本地变量。命令会自动读取当前目录下的 `.env.local` 和 `.env`；这两个文件已被 `.gitignore` 忽略。不要把真实 API key、secret、passphrase 或账户截图提交到仓库。

```bash
DATABASE_DSN=data.db
TRADING_MODE=dry_run
EXCHANGE_NAME=onebullex
ONEBULLEX_BASE_URL=https://futures-openapi.onebullex.com
ONEBULLEX_API_KEY=
ONEBULLEX_SECRET_KEY=
ONEBULLEX_LIVE_TRADING=false
MAX_LEVERAGE=3
BACKUP_PATH=./backups
```

## 一键启动本地交易链路

需要先安装 Go 1.22+（`go version` 可用）。完整排错与托管方式见 [`docs/local-startup.md`](docs/local-startup.md)。

默认使用 OneBullEx 线上公开行情驱动本地 paper 模拟仓，不会提交真实订单：

```bash
cd /Users/mac/Desktop/yoyo/gogogo
./scripts/stop-paper-local.sh

ONEBULLEX_LIVE_TRADING=false \
SUBMIT_EXCHANGE=false \
SYMBOL=BTCUSDT \
PROFILE=micro-trend-1m \
EQUITY=300 \
POSITION_MODEL=AGGREGATION \
./scripts/start-paper-local.sh
```

该脚本会初始化数据库，按本地最新 candle 增量同步执行周期和 profile 需要的趋势周期 K 线，持续同步标记价格和资金费率，并启动 `papertrade` 与 Dashboard。当前默认 `micro-trend-1m` 使用 `1m` 执行信号和 `5m EMA8/21` 趋势硬过滤。默认看板地址为 `http://localhost:8082`：

```bash
./scripts/status-paper-local.sh
tail -f .runtime/logs/papertrade.log
./scripts/stop-paper-local.sh
```

真实下单前先在 `.env.local` 配置 OneBullEx key。切换模式前先停掉旧进程。下面的命令会真实提交订单，并在启动时同步账户快照；`AGGREGATION` 表示单向持仓，双向持仓使用 `DISAGGREGATION`：

```bash
./scripts/stop-paper-local.sh

ONEBULLEX_LIVE_TRADING=true \
SUBMIT_EXCHANGE=true \
SYMBOL=BTCUSDT \
PROFILE=micro-trend-1m \
POSITION_MODEL=AGGREGATION \
./scripts/start-paper-local.sh
```

实盘提交模式会优先使用 `LIVE_ACCOUNT=live-main` 最新真实 USDT 快照做风控资金，不会把本地 `EQUITY=300` 模拟余额写到 `live-main`；真实下单时 `EQUITY` 不作为实盘资金来源。

如需用真实 key 只读同步真实账户、但策略仍 dry-run，不传 `SUBMIT_EXCHANGE=true` 即可；默认 paper 账户会使用 `paper-live-main`，真实账户快照使用 `LIVE_ACCOUNT=live-main`。

## 初始化数据库

```bash
DATABASE_DSN=data.db go run ./cmd/quantdb
```

## 同步行情

同步近 1 个月 BTCUSDT 永续合约 5m K 线：

```bash
go run ./cmd/marketsync \
  -dsn data.db \
  -dataset klines \
  -exchange onebullex \
  -market perpetual \
  -symbols BTCUSDT \
  -interval 5m \
  -start 2026-06-12T00:00:00Z \
  -end 2026-07-12T00:00:00Z \
  -limit 1500
```

同步半年 1h K 线时使用同一个命令，`marketsync` 已支持分页：

```bash
go run ./cmd/marketsync \
  -dsn data.db \
  -dataset klines \
  -exchange onebullex \
  -market perpetual \
  -symbols BTCUSDT \
  -interval 1h \
  -start 2026-01-12T00:00:00Z \
  -end 2026-07-12T00:00:00Z \
  -limit 1500
```

同步永续合约资金费率：

```bash
go run ./cmd/marketsync \
  -dsn data.db \
  -dataset funding \
  -exchange onebullex \
  -market perpetual \
  -symbols BTCUSDT \
  -start 2026-07-10T00:00:00Z \
  -end 2026-07-12T00:00:00Z
```

同步永续合约最新标记价格：

```bash
go run ./cmd/marketsync \
  -dsn data.db \
  -dataset mark-price \
  -exchange onebullex \
  -market perpetual \
  -symbols BTCUSDT
```

持续同步公开行情时开启 `-watch`。例如每 15 秒刷新 BTCUSDT 永续合约 5m K 线：

```bash
go run ./cmd/marketsync \
  -dsn data.db \
  -dataset klines \
  -exchange onebullex \
  -market perpetual \
  -symbols BTCUSDT \
  -interval 5m \
  -limit 20 \
  -watch \
  -poll-interval 15s
```

每 15 秒刷新 BTCUSDT 永续合约标记价格：

```bash
go run ./cmd/marketsync \
  -dsn data.db \
  -dataset mark-price \
  -exchange onebullex \
  -market perpetual \
  -symbols BTCUSDT \
  -watch \
  -poll-interval 15s
```

## 数据质量与快照

冻结永续合约近 1 个月 5m K 线快照。默认发现缺口会失败，不会写入快照：

```bash
go run ./cmd/datasnapshot \
  -dsn data.db \
  -exchange onebullex \
  -market perpetual \
  -symbols BTCUSDT \
  -interval 5m \
  -start 2026-06-12T00:00:00Z \
  -end 2026-07-12T00:00:00Z \
  -name onebullex-perpetual-btc-5m-20260612-20260712
```

冻结永续合约 1h K 线快照：

```bash
go run ./cmd/datasnapshot \
  -dsn data.db \
  -exchange onebullex \
  -market perpetual \
  -symbols BTCUSDT \
  -interval 1h \
  -start 2026-06-12T00:00:00Z \
  -end 2026-07-12T00:00:00Z \
  -name onebullex-perpetual-btc-1h-20260612-20260712
```

## 回测

运行当前 Scalp TPSL 单组参数回测，并写入 `backtest_runs`：

```bash
go run ./cmd/backtest \
  -dsn data.db \
  -exchange onebullex \
  -market perpetual \
  -symbol BTCUSDT \
  -interval 5m \
  -start 2026-06-12T00:00:00Z \
  -end 2026-07-12T00:00:00Z \
  -strategy-type scalp-tpsl \
  -fast 3 \
  -slow 9 \
  -dynamic-tpsl=true \
  -take-profit-atr-mult 1.6 \
  -stop-loss-atr-mult 1.0 \
  -min-take-profit-pct 0.55 \
  -max-take-profit-pct 1.40 \
  -min-stop-loss-pct 0.30 \
  -max-stop-loss-pct 0.75 \
  -fee-rate 0.0005
```

运行当前策略单币对、多参数扫描：

```bash
go run ./cmd/backtest \
  -dsn data.db \
  -exchange onebullex \
  -market perpetual \
  -symbol BTCUSDT \
  -interval 5m \
  -start 2026-06-12T00:00:00Z \
  -end 2026-07-12T00:00:00Z \
  -strategy-type scalp-tpsl \
  -fast 3,4,5 \
  -slow 9,12,15 \
  -dynamic-tpsl=true \
  -take-profit-atr-mult 1.6 \
  -stop-loss-atr-mult 1.0 \
  -fee-rate 0.0005
```

## 回测报告

按超额收益排序：

```bash
go run ./cmd/backtestreport \
  -dsn data.db \
  -sort excess \
  -limit 10
```

按绝对收益排序：

```bash
go run ./cmd/backtestreport \
  -dsn data.db \
  -sort total \
  -limit 10
```

## Dashboard 看板

启动本地网页看板：

```bash
HTTP_ADDR=:8081 DATABASE_DSN=data.db go run ./cmd/dashboard
```

浏览器访问：

```text
http://localhost:8081
```

看板当前读取 `data.db` 中的行情覆盖、价格曲线、回测结果、dry-run 订单、风险事件、策略信号、paper trading 绩效、资金费率、标记价格、余额和当前持仓。默认交易所筛选为 `onebullex`。

当前 `paper` 是本地模拟账户：开仓、持仓、止盈止损、盈亏由 `papertrade` 写入；标记价优先使用最新真实 OneBullEx 标记价格。`research`、`demo`、`test`、`manual` 等手工演示账户会被隐藏；历史里旧的 `paper` 手工假仓不会作为当前 paper 持仓展示。真实交易所账户余额和真实持仓只会在显式运行 `accountsnapshot -sync-live` 后写入。

接口为只读：

```text
GET /api/dashboard?exchange=onebullex&market=perpetual&symbol=BTCUSDT&interval=5m
```

## 风控检查

保守合约订单示例：

```bash
go run ./cmd/riskcheck \
  -market perpetual \
  -symbol BTCUSDT \
  -side buy \
  -price 60000 \
  -quantity 0.02 \
  -stop-price 58000 \
  -leverage 2 \
  -equity 10000 \
  -total-exposure 2000 \
  -symbol-exposure 1000
```

高杠杆合约订单示例，会被拒绝：

```bash
go run ./cmd/riskcheck \
  -market perpetual \
  -symbol SOLUSDT \
  -side buy \
  -price 150 \
  -quantity 10 \
  -stop-price 148 \
  -leverage 5 \
  -liquidation-price 140 \
  -funding-rate-pct 0.08 \
  -equity 10000
```

## Dry-run 下单日志

允许通过风控的 dry-run 订单会写入 `orders`。如需真正提交到 OneBullEx，必须同时传 `-submit-exchange=true` 且环境变量 `ONEBULLEX_LIVE_TRADING=true`；默认不会实盘下单：

```bash
go run ./cmd/dryrunorder \
  -dsn data.db \
  -account research \
  -strategy manual-dry-run \
  -client-order-id dryrun-btc-perp-allow-20260712 \
  -market perpetual \
  -symbol BTCUSDT \
  -side buy \
  -price 60000 \
  -quantity 0.02 \
  -stop-price 58000 \
  -leverage 2 \
  -equity 10000 \
  -total-exposure 2000 \
  -symbol-exposure 1000
```

被风控拒绝的 dry-run 订单会写入 `orders` 和 `risk_events`：

```bash
go run ./cmd/dryrunorder \
  -dsn data.db \
  -account research \
  -strategy manual-dry-run \
  -client-order-id dryrun-sol-perp-reject-20260712 \
  -market perpetual \
  -symbol SOLUSDT \
  -side buy \
  -price 150 \
  -quantity 10 \
  -stop-price 148 \
  -leverage 5 \
  -liquidation-price 140 \
  -funding-rate-pct 0.08 \
  -equity 10000
```

## 账户快照

写入本地 paper 账户余额、仓位和保证金快照：

```bash
go run ./cmd/accountsnapshot \
  -dsn data.db \
  -account paper \
  -exchange onebullex \
  -market perpetual \
  -asset USDT \
  -equity 10000 \
  -free 9000 \
  -locked 1000 \
  -symbol BTCUSDT \
  -quantity 0.01 \
  -entry-price 60000 \
  -mark-price 61000 \
  -liquidation-price 45000 \
  -leverage 1 \
  -margin-balance 10000
```

从 OneBullEx 私有 API 只读同步真实余额和真实合约持仓，需要先在环境变量里配置密钥。该命令只读取并入库，不会下单：

```bash
ONEBULLEX_API_KEY=... ONEBULLEX_SECRET_KEY=... \
go run ./cmd/accountsnapshot \
  -dsn data.db \
  -account live-readonly \
  -exchange onebullex \
  -market perpetual \
  -sync-live
```

## Paper Trading

基于已同步的 OneBullEx 本地 K 线运行 paper trading，并写入 `backtest_runs`、`strategy_runs`、`signals`、`performance_snapshots`。如果最新信号是开多或开空，会额外写入订单审计和 `paper_positions`；之后每轮用最新真实行情结算当前持仓、未实现盈亏、止盈和止损，并同步更新 `balances`、`positions`、`margin_snapshots`。

`-watch` 模式下会继续每轮结算持仓和止盈止损，但观望信号/账户快照默认按 `-persist-interval` 节流入库，完整回测记录默认按 `-backtest-interval` 节流入库，避免每 15 秒写一组重复 run/signal/backtest。

直接运行 `papertrade` 且不传 `-profile` 时，使用当前短线 `scalp-tpsl` 基础参数：周期为 `5m`，止盈止损按 ATR 动态计算，高周期趋势过滤要求 `15m EMA20/60` 与 `1h EMA20/60` 同向才允许新开仓。一键启动脚本默认传 `PROFILE=micro-trend-1m`，面向 300U 左右小资金的 1 分钟级别短线趋势；如需只跑基线对比，可显式传 `-strategy-type sma`。

```bash
go run ./cmd/papertrade \
  -dsn data.db \
  -account paper \
  -strategy perp-trend-scalp-v2-paper \
  -exchange onebullex \
  -market perpetual \
  -symbol BTCUSDT \
  -interval 5m \
  -start 2026-06-12T00:00:00Z \
  -end 2026-07-12T00:00:00Z \
  -fast 3 \
  -slow 9 \
  -dynamic-tpsl=true \
  -take-profit-atr-mult 1.6 \
  -stop-loss-atr-mult 1.0 \
  -min-take-profit-pct 0.55 \
  -max-take-profit-pct 1.40 \
  -min-stop-loss-pct 0.30 \
  -max-stop-loss-pct 0.75 \
  -signal-filter=true \
  -min-signal-score 0.55 \
  -trend-filter=true \
  -trend-interval 15m \
  -macro-trend-interval 1h \
  -equity 10000 \
  -quantity 0.01
```

启用趋势过滤前需同步对应的趋势周期 K 线；一键启动脚本会按 profile 自动完成同步。`micro-trend-1m` 会同步 `1m` 执行 K 线和 `5m` 趋势 K 线；`small-scalp-fast` / `small-scalp` 只需要 `15m`，基础/`aggressive` 档默认需要 `15m` 和 `1h`。单独调试且尚无高周期数据时，可临时传 `-trend-filter=false`。

短线 TP/SL 版本会用更短均线窗口提高交易频次，并把按 ATR 计算的止盈价、止损价写入 `orders`。开仓前还会计算 `signal_score`，低于 `-min-signal-score` 的候选开仓会被过滤成观望，并把候选动作、特征和过滤原因写入 `signals.raw_features_json`。当前 300U 左右小资金优先使用 `-profile micro-trend-1m`，该档在保留 `5m EMA8/21` 趋势硬过滤的前提下，使用 `1m` 执行信号、`fast=2/slow=5`、`min_signal_score=0.35`、`min_volume_ratio=0.50` 和更低 TP/SL 区间来提高短线触发频率：

```bash
go run ./cmd/papertrade \
  -dsn data.db \
  -account paper \
  -strategy-type scalp-tpsl \
  -exchange onebullex \
  -market perpetual \
  -symbol BTCUSDT \
  -profile micro-trend-1m \
  -start 2026-07-11T00:00:00Z \
  -end 2026-07-12T00:00:00Z \
  -equity 10000 \
  -quantity 0.01
```

持续运行短线 paper 策略时开启 `-watch`。默认不会向交易所提交真实订单，只会用真实行情驱动本地模拟下单和结算。只有显式增加 `-submit-exchange=true` 且 `ONEBULLEX_LIVE_TRADING=true` 时，风控通过的开仓/平仓订单才会调用 OneBullEx 下单 API，并把交易所订单号回写到 `orders.exchange_order_id`；开仓单会同步提交 `triggerProfitPrice` 和 `triggerStopPrice` 作为交易所原生保护字段。前期小资金可以用 `-profile micro-trend-1m` 切到更主动但单笔占用更受控的 1m 合约短线档，手动传入的参数会覆盖 profile 默认值：

```bash
go run ./cmd/papertrade \
  -dsn data.db \
  -account paper \
  -profile micro-trend-1m \
  -symbol BTCUSDT \
  -equity 1000 \
  -watch \
  -poll-interval 15s \
  -persist-interval 1m \
  -backtest-interval 5m
```

## 每日报告

统计指定时间之后的订单、风险事件、信号、策略运行、账户快照和仓位快照：

```bash
go run ./cmd/dailyreport \
  -dsn data.db \
  -since 2026-07-12T00:00:00Z
```

## 备份

把当前 SQLite 数据库复制到备份目录：

```bash
BACKUP_PATH=./backups \
DATABASE_DSN=data.db \
go run ./cmd/backupdb
```

## 急停开关

本地 emergency guard 使用 `.runtime/halt` 文件表示停机状态：

```bash
go run ./cmd/emergency -action status
go run ./cmd/emergency -action halt -reason manual_review
go run ./cmd/emergency -action resume
```

## 测试

```bash
go test ./...
```

## 注意事项

- 当前项目是量化研究原型，不是实盘交易系统。
- 默认不会转账，也不会向交易所提交真实订单；实盘提交必须显式设置 `-submit-exchange=true` 和 `ONEBULLEX_LIVE_TRADING=true`。
- `.env.example` 只提供变量名和默认边界，不包含真实密钥。
- `data.db` 已包含公开行情、回测结果、dry-run 订单、账户快照、paper trading 结果和风险事件，不建议提交到 git。
- `backups/` 是本地数据库备份目录，不建议提交到 git。
- 当前 Scalp TPSL paper/backtest 结果不构成上线建议；进入 live 前必须完成长区间回测、连续 paper trading、真实只读账户校验和小资金灰度审批。
