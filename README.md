# gogogo

一个使用 Go 标准库 + SQLite 构建的币圈量化研究原型。目前覆盖本地行情存储、Binance 公开行情同步、K 线质量检查、回测数据快照、SMA 趋势策略回测、自适应趋势轮动回测、参数扫描、回测报告、walk-forward 验证、账户/仓位快照、paper trading、本地风控检查、dry-run 下单审计、每日报告、SQLite 备份和人工急停开关。

当前系统不会向交易所提交真实订单。已实现命令以公开行情、本地回测、本地账户建模、paper trading 和 dry-run 订单审计为主；真实 API key、只读账户查询和 live 交易仍需要单独接入交易所私有 API 并完成安全审批。

## 已实现功能

- 本地 SQLite 数据库：`/Users/guilinzhou/Desktop/test-nemo/gogogo/data.db`
- Binance 公开行情同步：
  - 现货 / 永续合约 K 线
  - 永续合约资金费率
  - 永续合约标记价格
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
- SMA 均线交叉回测
- 多币种自适应趋势轮动：动量排名、趋势过滤、波动率控仓、移动止损、资金费率过滤
- 多币种、多参数网格扫描
- 回测结果落库和排名报告
- Walk-forward 样本外验证
- 研究函数：动量轮动、均值回归、现货 + 合约对冲比例
- 基础风控检查：单笔风险、单币种敞口、总敞口、杠杆、日亏损、连续亏损、强平距离、资金费率
- Dry-run 下单日志：计划订单、风控结论、风险事件落库，不触发真实交易
- Paper trading：基于本地历史 K 线生成策略运行、信号和绩效快照
- 账户/仓位/保证金快照：本地写入 balances、positions、margin_snapshots
- 每日运行报告、SQLite 备份、人工 emergency halt/resume
- 保留原有轻量 CRUD API 示例，主要用于 HTTP/API 结构参考

## 目录结构

```text
cmd/
  api/              # 原 CRUD API 示例
  quantdb/          # 初始化本地量化数据表
  marketsync/       # 同步 Binance 公开行情
  datasnapshot/     # 检查 K 线缺口并冻结回测数据快照
  backtest/         # 运行 SMA 回测并保存结果
  adaptivetrend/    # 运行多币种自适应趋势轮动回测并保存结果
  backtestreport/   # 查询和排序回测结果
  walkforward/      # 样本外 walk-forward 验证
  dashboard/        # 本地网页看板，读取 SQLite 量化数据
  riskcheck/        # 本地下单意图风控检查
  dryrunorder/      # 写入 dry-run 订单和风险事件
  accountsnapshot/  # 写入本地账户、仓位、保证金快照
  papertrade/       # 基于本地 K 线运行 paper trading
  dailyreport/      # 汇总订单、风险事件、信号、快照等每日指标
  backupdb/         # 备份 SQLite data.db
  emergency/        # 本地人工急停开关
internal/
  exchange/         # 交易所抽象接口
  dashboard/        # Dashboard HTTP handler 与内嵌静态资源
  exchange/binance/ # Binance 公开行情 client
  marketdata/       # 行情/资金费率/标记价格 SQLite 存取
  backtest/         # 回测、报告、walk-forward
  risk/             # 本地风控规则
  execution/        # dry-run 订单日志和风险事件
  portfolio/        # 账户、仓位、保证金快照
  strategy/         # 策略运行、信号、绩效快照和研究函数
  storage/          # 统一 SQLite schema 初始化和迁移边界
  runtime/          # 本地 emergency halt guard
  item/             # 原 CRUD 示例领域
  httpapi/          # 原 CRUD 示例 HTTP API
docs/
  quant-trading-strategy-todo.md
```

## 配置

参考 `.env.example` 设置本地变量。不要把真实 `.env`、API key、secret、passphrase 或账户截图提交到仓库。

```bash
DATABASE_DSN=/Users/guilinzhou/Desktop/test-nemo/gogogo/data.db
TRADING_MODE=dry_run
EXCHANGE_NAME=binance
MAX_LEVERAGE=3
BACKUP_PATH=/Users/guilinzhou/Desktop/test-nemo/gogogo/backups
```

## 初始化数据库

```bash
DATABASE_DSN=/Users/guilinzhou/Desktop/test-nemo/gogogo/data.db go run ./cmd/quantdb
```

也可以直接使用环境变量：

```bash
DATABASE_DSN=/Users/guilinzhou/Desktop/test-nemo/gogogo/data.db go run ./cmd/quantdb
```

## 同步行情

同步近 1 个月 BTC/ETH/BNB/SOL 现货 1h K 线：

```bash
go run ./cmd/marketsync \
  -dsn /Users/guilinzhou/Desktop/test-nemo/gogogo/data.db \
  -dataset klines \
  -market spot \
  -symbols BTCUSDT,ETHUSDT,BNBUSDT,SOLUSDT \
  -interval 1h \
  -start 2026-06-12T00:00:00Z \
  -end 2026-07-12T00:00:00Z \
  -limit 1000
```

同步半年 1h K 线时使用同一个命令，`marketsync` 已支持分页：

```bash
go run ./cmd/marketsync \
  -dsn /Users/guilinzhou/Desktop/test-nemo/gogogo/data.db \
  -dataset klines \
  -market spot \
  -symbols BTCUSDT,ETHUSDT,BNBUSDT,SOLUSDT \
  -interval 1h \
  -start 2026-01-12T00:00:00Z \
  -end 2026-07-12T00:00:00Z \
  -limit 1000
```

同步永续合约资金费率：

```bash
go run ./cmd/marketsync \
  -dsn /Users/guilinzhou/Desktop/test-nemo/gogogo/data.db \
  -dataset funding \
  -market perpetual \
  -symbols BTCUSDT \
  -start 2026-07-10T00:00:00Z \
  -end 2026-07-12T00:00:00Z
```

同步永续合约最新标记价格：

```bash
go run ./cmd/marketsync \
  -dsn /Users/guilinzhou/Desktop/test-nemo/gogogo/data.db \
  -dataset mark-price \
  -market perpetual \
  -symbols BTCUSDT
```

持续同步公开行情时开启 `-watch`。例如每 15 秒刷新 BTCUSDT 现货 1m K 线：

```bash
go run ./cmd/marketsync \
  -dsn /Users/guilinzhou/Desktop/test-nemo/gogogo/data.db \
  -dataset klines \
  -market spot \
  -symbols BTCUSDT \
  -interval 1m \
  -limit 20 \
  -watch \
  -poll-interval 15s
```

每 15 秒刷新 BTCUSDT 永续合约标记价格：

```bash
go run ./cmd/marketsync \
  -dsn /Users/guilinzhou/Desktop/test-nemo/gogogo/data.db \
  -dataset mark-price \
  -market perpetual \
  -symbols BTCUSDT \
  -watch \
  -poll-interval 15s
```

## 数据质量与快照

冻结现货近 1 个月 1h K 线快照。默认发现缺口会失败，不会写入快照：

```bash
go run ./cmd/datasnapshot \
  -dsn /Users/guilinzhou/Desktop/test-nemo/gogogo/data.db \
  -market spot \
  -symbols BTCUSDT,ETHUSDT,BNBUSDT,SOLUSDT \
  -interval 1h \
  -start 2026-06-12T00:00:00Z \
  -end 2026-07-12T00:00:00Z \
  -name spot-1h-20260612-20260712
```

冻结永续合约同区间快照：

```bash
go run ./cmd/datasnapshot \
  -dsn /Users/guilinzhou/Desktop/test-nemo/gogogo/data.db \
  -market perpetual \
  -symbols BTCUSDT,ETHUSDT,BNBUSDT,SOLUSDT \
  -interval 1h \
  -start 2026-06-12T00:00:00Z \
  -end 2026-07-12T00:00:00Z \
  -name perpetual-1h-20260612-20260712
```

当前本地 `data.db` 已冻结：

- `spot-1h-20260612-20260712`：BTC/ETH/BNB/SOL 都是 720/720，无缺口。
- `perpetual-1h-20260612-20260712`：BTC/ETH/BNB/SOL 都是 720/720，无缺口。

## 回测

运行单组参数回测：

```bash
go run ./cmd/backtest \
  -dsn /Users/guilinzhou/Desktop/test-nemo/gogogo/data.db \
  -market spot \
  -symbol BTCUSDT \
  -interval 1h \
  -start 2026-06-12T00:00:00Z \
  -end 2026-07-12T00:00:00Z \
  -fast 12 \
  -slow 48 \
  -fee-rate 0.001
```

运行多币种、多参数扫描：

```bash
go run ./cmd/backtest \
  -dsn /Users/guilinzhou/Desktop/test-nemo/gogogo/data.db \
  -market spot \
  -symbol BTCUSDT,ETHUSDT,BNBUSDT,SOLUSDT \
  -interval 1h \
  -start 2026-06-12T00:00:00Z \
  -end 2026-07-12T00:00:00Z \
  -fast 6,12,24 \
  -slow 24,48,96 \
  -fee-rate 0.001
```

运行多币种自适应趋势轮动策略。该策略会在多个币种中选择趋势和动量更强的标的，按波动率控制单币种仓位，触发移动止损时退出；如果同步了永续合约资金费率，也会过滤 funding 过高的追多交易：

```bash
go run ./cmd/adaptivetrend \
  -dsn /Users/guilinzhou/Desktop/test-nemo/gogogo/data.db \
  -market spot \
  -symbol BTCUSDT,ETHUSDT,BNBUSDT,SOLUSDT \
  -interval 1h \
  -start 2026-06-12T00:00:00Z \
  -end 2026-07-12T00:00:00Z \
  -momentum-window 24 \
  -trend-window 72 \
  -breakout-window 48 \
  -volatility-window 24 \
  -rebalance-window 6 \
  -top-n 2 \
  -target-volatility-pct 1 \
  -max-position-pct 30 \
  -trailing-stop-pct 6 \
  -max-funding-rate-pct 0.05 \
  -fee-rate 0.001 \
  -slippage-rate 0.0005
```

`adaptivetrend` 默认会把结果写入 `backtest_runs`，因此可以继续用 `backtestreport` 和 Dashboard 查看结果。当前实现仍是本地回测，不会向交易所提交真实订单。

## 回测报告

按超额收益排序：

```bash
go run ./cmd/backtestreport \
  -dsn /Users/guilinzhou/Desktop/test-nemo/gogogo/data.db \
  -sort excess \
  -limit 10
```

按绝对收益排序：

```bash
go run ./cmd/backtestreport \
  -dsn /Users/guilinzhou/Desktop/test-nemo/gogogo/data.db \
  -sort total \
  -limit 10
```

## Walk-forward 验证

```bash
go run ./cmd/walkforward \
  -dsn /Users/guilinzhou/Desktop/test-nemo/gogogo/data.db \
  -market spot \
  -symbol BTCUSDT,ETHUSDT,BNBUSDT,SOLUSDT \
  -interval 1h \
  -start 2026-06-12T00:00:00Z \
  -end 2026-07-12T00:00:00Z \
  -fast 6,12,24 \
  -slow 24,48,96 \
  -train-window 240 \
  -test-window 120 \
  -fee-rate 0.001
```

## Dashboard 看板

启动本地网页看板：

```bash
HTTP_ADDR=:8081 DATABASE_DSN=/Users/guilinzhou/Desktop/test-nemo/gogogo/data.db go run ./cmd/dashboard
```

浏览器访问：

```text
http://localhost:8081
```

看板当前读取 `data.db` 中的行情覆盖、价格曲线、回测结果、dry-run 订单、风险事件、策略信号、paper trading 绩效、资金费率、标记价格、余额和当前持仓。

当前 `paper` 是本地模拟账户：开仓、持仓、止盈止损、盈亏由 `papertrade` 写入；标记价优先使用最新真实 Binance 标记价格。`research`、`demo`、`test`、`manual` 等手工演示账户会被隐藏；历史里旧的 `paper` 手工假仓不会作为当前 paper 持仓展示。真实交易所账户余额和真实持仓仍需接入交易所私有只读账户 API 后写入。

接口为只读：

```text
GET /api/dashboard?market=spot&symbol=BTCUSDT&interval=1h
```

## 风控检查

保守现货订单示例：

```bash
go run ./cmd/riskcheck \
  -market spot \
  -symbol BTCUSDT \
  -side buy \
  -price 60000 \
  -quantity 0.02 \
  -stop-price 58000 \
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

允许通过风控的 dry-run 订单会写入 `orders`：

```bash
go run ./cmd/dryrunorder \
  -dsn /Users/guilinzhou/Desktop/test-nemo/gogogo/data.db \
  -account research \
  -strategy manual-dry-run \
  -client-order-id dryrun-btc-spot-allow-20260712 \
  -market spot \
  -symbol BTCUSDT \
  -side buy \
  -price 60000 \
  -quantity 0.02 \
  -stop-price 58000 \
  -equity 10000 \
  -total-exposure 2000 \
  -symbol-exposure 1000
```

被风控拒绝的 dry-run 订单会写入 `orders` 和 `risk_events`：

```bash
go run ./cmd/dryrunorder \
  -dsn /Users/guilinzhou/Desktop/test-nemo/gogogo/data.db \
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
  -dsn /Users/guilinzhou/Desktop/test-nemo/gogogo/data.db \
  -account paper \
  -exchange binance \
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

## Paper Trading

基于已同步的本地 K 线运行 paper trading，并写入 `backtest_runs`、`strategy_runs`、`signals`、`performance_snapshots`。如果最新信号是买入，会额外写入 dry-run 订单审计和 `paper_positions`；之后每轮用最新真实行情结算当前持仓、未实现盈亏、止盈和止损，并同步更新 `balances`、`positions`、`margin_snapshots`。

默认保留 SMA 基线，也可以切到短线 `scalp-tpsl`。

```bash
go run ./cmd/papertrade \
  -dsn /Users/guilinzhou/Desktop/test-nemo/gogogo/data.db \
  -account paper \
  -strategy sma-paper \
  -market spot \
  -symbol BTCUSDT \
  -interval 1h \
  -start 2026-06-12T00:00:00Z \
  -end 2026-07-12T00:00:00Z \
  -fast 12 \
  -slow 48 \
  -equity 10000 \
  -quantity 0.01
```

短线 TP/SL 版本会用更短均线窗口提高交易频次，并把止盈价、止损价写入 `orders`：

```bash
go run ./cmd/papertrade \
  -dsn /Users/guilinzhou/Desktop/test-nemo/gogogo/data.db \
  -account paper \
  -strategy-type scalp-tpsl \
  -market spot \
  -symbol BTCUSDT \
  -interval 1m \
  -start 2026-07-11T00:00:00Z \
  -end 2026-07-12T00:00:00Z \
  -fast 3 \
  -slow 9 \
  -take-profit-pct 0.8 \
  -stop-loss-pct 0.4 \
  -cooldown-bars 1 \
  -equity 10000 \
  -quantity 0.01
```

持续运行短线 paper 策略时开启 `-watch`。该模式不会向交易所提交真实订单，只会用真实行情驱动本地模拟下单和结算。前期小资金可以用 `-profile aggressive` 切到更主动的合约短线档，手动传入的参数会覆盖 profile 默认值：

```bash
go run ./cmd/papertrade \
  -dsn /Users/guilinzhou/Desktop/test-nemo/gogogo/data.db \
  -account paper \
  -profile aggressive \
  -symbol BTCUSDT \
  -equity 1000 \
  -watch \
  -poll-interval 15s
```

## 每日报告

统计指定时间之后的订单、风险事件、信号、策略运行、账户快照和仓位快照：

```bash
go run ./cmd/dailyreport \
  -dsn /Users/guilinzhou/Desktop/test-nemo/gogogo/data.db \
  -since 2026-07-12T00:00:00Z
```

## 备份

把当前 SQLite 数据库复制到备份目录：

```bash
BACKUP_PATH=/Users/guilinzhou/Desktop/test-nemo/gogogo/backups \
DATABASE_DSN=/Users/guilinzhou/Desktop/test-nemo/gogogo/data.db \
go run ./cmd/backupdb
```

## 急停开关

本地 emergency guard 使用 `.runtime/halt` 文件表示停机状态：

```bash
go run ./cmd/emergency -action status
go run ./cmd/emergency -action halt -reason manual_review
go run ./cmd/emergency -action resume
```

## 原 CRUD API 示例

原有 HTTP API 仍可运行：

```bash
API_TOKEN=dev-secret DATABASE_DSN=/Users/guilinzhou/Desktop/test-nemo/gogogo/data.db go run ./cmd/api
```

接口：

- `GET /health`
- `GET /items`
- `POST /items`
- `GET /items/{id}`
- `PUT /items/{id}`
- `DELETE /items/{id}`

写操作需要：

```http
Authorization: Bearer dev-secret
```

## 测试

```bash
go test ./...
```

## 注意事项

- 当前项目是量化研究原型，不是实盘交易系统。
- 当前不会转账，也不会向交易所提交真实订单。
- `.env.example` 只提供变量名和默认边界，不包含真实密钥。
- `data.db` 已包含公开行情、回测结果、dry-run 订单、账户快照、paper trading 结果和风险事件，不建议提交到 git。
- `backups/` 是本地数据库备份目录，不建议提交到 git。
- 当前 SMA paper/backtest 结果不构成上线建议；进入 live 前必须完成长区间回测、连续 paper trading、真实只读账户校验和小资金灰度审批。
