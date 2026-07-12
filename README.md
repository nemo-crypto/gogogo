# gogogo

一个使用 Go 标准库 + SQLite 构建的币圈量化研究原型。目前重点是本地行情存储、Binance 公开行情同步、SMA 趋势策略回测、参数扫描、回测报告和 walk-forward 验证。

当前系统不使用 API key、不下单、不做实盘交易。所有已实现命令都是只读公开行情或本地回测。

## 已实现功能

- 本地 SQLite 数据库：`/Users/guilinzhou/Desktop/test-nemo/gogogo/data.db`
- Binance 公开行情同步：
  - 现货 / 永续合约 K 线
  - 永续合约资金费率
  - 永续合约标记价格
- 本地数据表：
  - `candles`
  - `funding_rates`
  - `mark_prices`
  - `backtest_runs`
- SMA 均线交叉回测
- 多币种、多参数网格扫描
- 回测结果落库和排名报告
- Walk-forward 样本外验证
- 保留原有轻量 CRUD API 示例，主要用于 HTTP/API 结构参考

## 目录结构

```text
cmd/
  api/              # 原 CRUD API 示例
  quantdb/          # 初始化本地量化数据表
  marketsync/       # 同步 Binance 公开行情
  backtest/         # 运行 SMA 回测并保存结果
  backtestreport/   # 查询和排序回测结果
  walkforward/      # 样本外 walk-forward 验证
internal/
  exchange/binance/ # Binance 公开行情 client
  marketdata/       # 行情/资金费率/标记价格 SQLite 存取
  backtest/         # 回测、报告、walk-forward
  item/             # 原 CRUD 示例领域
  httpapi/          # 原 CRUD 示例 HTTP API
docs/
  quant-trading-strategy-todo.md
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
- 当前不会读取 API key，不会下单，不会转账。
- `data.db` 已包含真实公开行情和回测结果，不建议提交到 git。
- 回测结果不能代表未来收益，必须继续做更长历史区间、样本外验证、手续费/滑点建模和 paper trading。
