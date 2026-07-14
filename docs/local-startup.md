# 本地启动脚本速查

> 更新时间：2026-07-14  
> 默认链路：OneBullEx 永续合约 `BTCUSDT`，真实行情入库，本地 paper 模拟交易，Dashboard 看板。

## 你要的两条一键命令

### 1. OneBullEx 线上真实数据 + 本地模拟仓位 + 回测数据入库

这条只使用 OneBullEx 公开线上行情，不会提交真实订单；策略会写入本地 paper 仓位、订单、风控事件、回测结果和行情数据。

```bash
ONEBULLEX_LIVE_TRADING=false \
SUBMIT_EXCHANGE=false \
SYMBOL=BTCUSDT \
ACCOUNT=paper \
PROFILE=aggressive \
EQUITY=1000 \
POSITION_MODEL=AGGREGATION \
./scripts/start-paper-local.sh
```

启动后看：

```bash
tail -f .runtime/logs/papertrade.log
```

停止：

```bash
./scripts/stop-paper-local.sh
```

### 2. OneBullEx 线上真实数据 + 真实 key + 真实下单 + 回测数据入库

先在 `.env.local` 配置真实 key，不要把真实 key 写进命令行历史：

```bash
ONEBULLEX_BASE_URL=https://futures-openapi.onebullex.com
ONEBULLEX_API_KEY=你的真实_API_KEY
ONEBULLEX_SECRET_KEY=你的真实_SECRET_KEY
```

确认账户、交易对、仓位模式后，再启动真实下单模式：

```bash
ONEBULLEX_LIVE_TRADING=true \
SUBMIT_EXCHANGE=true \
SYMBOL=BTCUSDT \
ACCOUNT=live-main \
PROFILE=aggressive \
EQUITY=1000 \
POSITION_MODEL=AGGREGATION \
./scripts/start-paper-local.sh
```

这条命令会先同步一次真实账户快照和持仓配置，然后启动真实行情同步、策略、Dashboard；策略通过 `-submit-exchange` 向 OneBullEx 提交订单，同时仍会把回测结果、本地订单、风控事件、行情数据写入 `data.db`。

真实下单日志：

```bash
tail -f .runtime/logs/papertrade.log
tail -f .runtime/logs/accountsnapshot.log
```

停止：

```bash
./scripts/stop-paper-local.sh
```

> 风险提示：第二条命令会真实下单。`POSITION_MODEL=AGGREGATION` 表示 OneBullEx 单向持仓，下单会使用 `positionSide=BOTH`；双向持仓用 `POSITION_MODEL=DISAGGREGATION` 或留空，让策略按 LONG/SHORT 推断。

## 日常只用这些命令

| 命令 | 干啥 |
| --- | --- |
| `./scripts/start-paper-local.sh` | 一键后台启动整套本地 paper 服务。 |
| `./scripts/status-paper-local.sh` | 查看服务是否在跑、PID 和日志路径。 |
| `./scripts/stop-paper-local.sh` | 停止整套本地 paper 服务。 |
| `tail -f .runtime/logs/papertrade.log` | 查看 paper 策略运行日志。 |
| `tail -f .runtime/logs/marketsync-klines-3m.log` | 查看执行周期 K 线同步日志，默认 aggressive 是 `3m`。 |
| `tail -f .runtime/logs/marketsync-klines-15m.log` | 查看趋势周期 K 线同步日志。 |
| `tail -f .runtime/logs/marketsync-klines-1h.log` | 查看宏观趋势周期 K 线同步日志。 |
| `tail -f .runtime/logs/marketsync-mark-price.log` | 查看 mark price 同步日志。 |
| `tail -f .runtime/logs/marketsync-funding.log` | 查看资金费率同步日志。 |
| `tail -f .runtime/logs/accountsnapshot.log` | 查看真实账户快照同步日志，只在真实交易模式启动时会有内容。 |
| `tail -f .runtime/logs/dashboard.log` | 查看看板服务日志。 |

启动后看板地址：

```text
http://localhost:8082
```

## 一键启动会做什么

`./scripts/start-paper-local.sh` 会拉起 supervisor，supervisor 会自动管理：

| 子进程 | 干啥 |
| --- | --- |
| `quantdb` | 初始化/更新数据库表结构。 |
| `marketsync-klines-3m/5m` | 持续同步执行周期 K 线。 |
| `marketsync-klines-15m` | 持续同步趋势过滤 K 线。 |
| `marketsync-klines-1h` | 持续同步宏观趋势确认 K 线。 |
| `marketsync-mark-price` | 持续同步真实 mark price。 |
| `marketsync-funding` | 持续同步资金费率。 |
| `accountsnapshot` | 仅真实交易模式启动时执行一次，同步真实余额、持仓和仓位配置。 |
| `papertrade` | 持续运行本地 paper 策略。 |
| `dashboard` | 启动本地看板。 |

默认参数：

| 参数 | 默认值 |
| --- | --- |
| 交易对 | `BTCUSDT` |
| 策略档位 | `aggressive` |
| K 线周期 | `3m` |
| paper 账户 | `paper` |
| paper 权益 | `1000 USDT` |
| 看板端口 | `:8082` |
| 行情/策略轮询 | `15s` |
| 资金费率轮询 | `5m` |
| 趋势过滤 | 默认开启，`15m EMA20/60 + 1h EMA20/60` 同向才允许新开仓 |
| 保本止损 | 默认开启，浮盈达到 `1R` 后移动到保本价 |
| ATR 追踪止损 | 默认开启，浮盈达到 `1.5R` 后按 `1.2 * ATR` 追踪 |
| 实盘交易 | 默认关闭；必须同时设置 `ONEBULLEX_LIVE_TRADING=true` 和 `SUBMIT_EXCHANGE=true` |

## 常用变体

| 命令 | 干啥 |
| --- | --- |
| `SYMBOL=ETHUSDT ./scripts/start-paper-local.sh` | 改跑 `ETHUSDT`。 |
| `EQUITY=2000 ./scripts/start-paper-local.sh` | 改 paper 初始权益为 `2000 USDT`。 |
| `HTTP_ADDR=:8083 ./scripts/start-paper-local.sh` | 改看板端口为 `8083`。 |
| `PROFILE=manual INTERVAL=5m EQUITY=10000 ./scripts/start-paper-local.sh` | 使用更保守的 `5m` 手动档。 |
| `TREND_FILTER=false ./scripts/start-paper-local.sh` | 临时关闭高周期趋势过滤，只建议调试时使用。 |
| `TREND_INTERVAL=30m MACRO_TREND_INTERVAL=2h ./scripts/start-paper-local.sh` | 改趋势过滤周期，同时会同步对应 K 线。 |
| `BREAKEVEN_STOP=false TRAILING_STOP=false ./scripts/start-paper-local.sh` | 临时关闭保护止损，只建议对比回测/调试时使用。 |
| `TRAILING_ACTIVATION_R=2.0 TRAILING_ATR_MULT=1.5 ./scripts/start-paper-local.sh` | 调整 ATR 追踪止损触发和宽度。 |
| `POSITION_MODEL=AGGREGATION ./scripts/start-paper-local.sh` | 按 OneBullEx 单向持仓模式运行，模拟模式不会真实下单。 |
| `SYMBOL=ETHUSDT EQUITY=2000 HTTP_ADDR=:8083 ./scripts/start-paper-local.sh` | 同时改交易对、权益和端口。 |

## 调试和托管

| 命令 | 干啥 |
| --- | --- |
| `./scripts/run-paper-local-stack.sh` | 前台运行整套服务，适合调试，按 `Ctrl+C` 停止。 |
| `./scripts/start-paper-screen.sh` | 用 `screen` 启动整套服务，终端关了也能重新进入。 |
| `screen -r gogogo-paper` | 重新进入 `screen` 会话看运行输出。 |
| `./scripts/install-paper-launchd.sh` | 安装 macOS LaunchAgent，登录后自动常驻。 |
| `launchctl print gui/$(id -u)/com.gogogo.paper-local-stack` | 查看 LaunchAgent 状态。 |
| `./scripts/uninstall-paper-launchd.sh` | 卸载 LaunchAgent。 |

## 文件位置

| 路径 | 干啥 |
| --- | --- |
| `.runtime/logs/` | 所有本地服务日志。 |
| `.runtime/pids/` | 所有本地服务 PID。 |
| `.runtime/logs/supervisor.log` | supervisor 启动和异常日志。 |
| `.runtime/logs/launchd.out.log` | LaunchAgent 标准输出。 |
| `.runtime/logs/launchd.err.log` | LaunchAgent 错误输出。 |
| `data.db` | 本地 SQLite 数据库。 |

## 安全说明

这些本地启动脚本默认只跑 paper 模拟仓，不会提交真实订单。

真实订单必须同时满足：

```bash
ONEBULLEX_LIVE_TRADING=true
SUBMIT_EXCHANGE=true
```

只设置其中一个不会进入完整真实交易模式。日常调试优先使用第一条模拟命令。
