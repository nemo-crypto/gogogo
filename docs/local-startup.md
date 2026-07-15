# 本地启动脚本速查

> 更新时间：2026-07-14  
> 默认链路：OneBullEx 永续合约 `BTCUSDT`，真实行情入库，本地 paper 模拟交易，Dashboard 看板。

## 前置条件

1. 本机已安装 **Go 1.22+**，且 `go version` 可用：

```bash
go version
# 没有的话：
brew install go
```

国内网络若拉依赖超时，脚本会默认使用 `GOPROXY=https://goproxy.cn,direct`。也可手动：

```bash
export GOPROXY=https://goproxy.cn,direct
export GOSUMDB=off
```

2. 在项目根目录准备 `.env.local`（已被 gitignore，不要提交）：

```bash
cp .env.example .env.local
```

3. 每次切换 paper / 真实下单模式前，先停掉旧进程：

```bash
./scripts/stop-paper-local.sh
```

> 不要先单独敲 `bash` 再粘贴多行命令。应在项目根目录 **整段一次粘贴执行**，或写成单行。

## 你要的两条一键命令

### 1. OneBullEx 线上真实数据 + 本地模拟仓位 + 回测数据入库

这条只用 OneBullEx 公开线上行情，**不会提交真实订单**；策略会写入本地 paper 仓位、订单、风控事件、回测结果和行情数据。

```bash
cd /Users/mac/Desktop/yoyo/gogogo

ONEBULLEX_LIVE_TRADING=false \
SUBMIT_EXCHANGE=false \
SYMBOL=BTCUSDT \
PROFILE=aggressive \
EQUITY=1000 \
POSITION_MODEL=AGGREGATION \
./scripts/start-paper-local.sh
```

启动成功后再看日志：

```bash
./scripts/status-paper-local.sh
tail -f .runtime/logs/papertrade.log
```

停止：

```bash
./scripts/stop-paper-local.sh
```

### 2. OneBullEx 线上真实数据 + 真实 key + 真实下单 + 回测数据入库

真实 key 直接放在项目根目录 `.env.local`，启动脚本会自动读取；启动命令里不用复制、脱敏或重复配置 key。只要确认 `.env.local` 里已有 `ONEBULLEX_BASE_URL` / `ONEBULLEX_API_KEY` / `ONEBULLEX_SECRET_KEY`。

日常可把 `.env.local` 里的 `ONEBULLEX_LIVE_TRADING` 保持 `false`；真实下单只在启动时显式打开两个开关：

```bash
cd /Users/mac/Desktop/yoyo/gogogo
./scripts/stop-paper-local.sh

ONEBULLEX_LIVE_TRADING=true SUBMIT_EXCHANGE=true POSITION_MODEL=AGGREGATION ./scripts/start-paper-local.sh
```

默认跑 `BTCUSDT` / `aggressive` / `EQUITY=1000`；要改的话只在同一行追加覆盖，例如 `SYMBOL=ETHUSDT EQUITY=2000`。

这条会：

1. 校验 Go、校验 API key 是否已从 `.env.local` 加载  
2. 同步一次真实账户快照和持仓配置到 `data.db`  
3. 持续同步真实行情（K 线 / mark price / funding）入库；K 线默认按本地最新 candle 增量补齐
4. 启动策略并带 `-submit-exchange` 向 OneBullEx **真实下单**  
5. 同时把回测结果、本地订单、风控事件、行情写入 `data.db`  
6. 启动 Dashboard

真实下单日志：

```bash
./scripts/status-paper-local.sh
tail -f .runtime/logs/papertrade.log
tail -f .runtime/logs/accountsnapshot.log
```

停止：

```bash
./scripts/stop-paper-local.sh
```

> 风险提示：第二条会真实下单。`POSITION_MODEL=AGGREGATION` 表示 OneBullEx 单向持仓，下单用 `positionSide=BOTH`；双向持仓用 `POSITION_MODEL=DISAGGREGATION` 或留空，让策略按 LONG/SHORT 推断。

## 启动失败时优先看这些

| 现象 | 原因 | 处理 |
| --- | --- | --- |
| `go: command not found` | 本机没有 Go，或不在 PATH | `brew install go`，新开终端后再启 |
| `proxy.golang.org ... i/o timeout` | 默认 Go 代理不可达 | 脚本会自动用 `goproxy.cn`；也可手动 `export GOPROXY=https://goproxy.cn,direct` |
| `context deadline exceeded` 访问 OneBullEx | 访问交易所 API 超时 | 本机有 ClashX 时在 `.env.local` 加 `HTTP_PROXY=http://127.0.0.1:7890`、`HTTPS_PROXY=http://127.0.0.1:7890`、`NO_PROXY=localhost,127.0.0.1` 后重启 |
| `papertrade.log: No such file` | 栈其实没起来，只是旧文档/假成功 | 看 `.runtime/logs/supervisor.log` 和 `quantdb.log` |
| `already running` | 旧进程还在，新开关不会生效 | 先 `./scripts/stop-paper-local.sh` |
| `SUBMIT_EXCHANGE=true requires ONEBULLEX_LIVE_TRADING=true` | 只开了一个开关 | 两个开关必须都是 `true` |
| `requires ONEBULLEX_API_KEY ...` | `.env.local` 缺 key | 补齐 key 后重试 |
| Dashboard 打不开 | 默认端口是 `8082` | 打开 `http://localhost:8082`；或用 `HTTP_ADDR=:8083` 改端口 |

命令行传入的 `ONEBULLEX_LIVE_TRADING` / `SUBMIT_EXCHANGE` / `SYMBOL` 等会覆盖 `.env.local` 同名项；API key 仍只从 `.env.local` 读取。

## 日常只用这些命令

| 命令 | 干啥 |
| --- | --- |
| `./scripts/start-paper-local.sh` | 一键后台启动整套本地服务。 |
| `./scripts/status-paper-local.sh` | 查看服务是否在跑、PID 和日志路径。 |
| `./scripts/stop-paper-local.sh` | 停止整套本地服务。 |
| `tail -f .runtime/logs/papertrade.log` | 查看策略运行日志。 |
| `tail -f .runtime/logs/marketsync-klines-5m.log` | 查看执行周期 K 线同步日志，默认 aggressive 是 `5m`。 |
| `tail -f .runtime/logs/marketsync-klines-15m.log` | 查看趋势周期 K 线同步日志。 |
| `tail -f .runtime/logs/marketsync-klines-1h.log` | 查看宏观趋势周期 K 线同步日志。 |
| `tail -f .runtime/logs/marketsync-mark-price.log` | 查看 mark price 同步日志。 |
| `tail -f .runtime/logs/marketsync-funding.log` | 查看资金费率同步日志。 |
| `tail -f .runtime/logs/accountsnapshot.log` | 查看真实账户快照同步日志，只在真实交易模式启动时会有内容。 |
| `tail -f .runtime/logs/dashboard.log` | 查看看板服务日志。 |
| `tail -f .runtime/logs/supervisor.log` | 查看 supervisor 汇总日志（启动失败先看这个）。 |

启动后看板地址：

```text
http://localhost:8082
```

## 一键启动会做什么

`./scripts/start-paper-local.sh` 会拉起 supervisor，supervisor 会自动管理：

| 子进程 | 干啥 |
| --- | --- |
| `quantdb` | 初始化/更新数据库表结构。 |
| `marketsync-klines-5m` | 持续同步执行周期 K 线。 |
| `marketsync-klines-15m` | 持续同步趋势过滤 K 线。 |
| `marketsync-klines-1h` | 持续同步宏观趋势确认 K 线。 |
| `marketsync-mark-price` | 持续同步真实 mark price。 |
| `marketsync-funding` | 持续同步资金费率。 |
| `accountsnapshot` | `ONEBULLEX_LIVE_TRADING=true` 时持续只读同步真实余额、持仓和仓位配置。 |
| `papertrade` | 持续运行策略；默认按新 K 线/节流间隔入库，`SUBMIT_EXCHANGE=true` 时真实下单。 |
| `dashboard` | 启动本地看板。 |

默认参数：

| 参数 | 默认值 |
| --- | --- |
| 交易对 | `BTCUSDT` |
| 策略档位 | `aggressive` |
| K 线周期 | `5m` |
| paper 账户 | 普通 paper 为 `paper`；真实 key dry-run 为 `paper-live-main`；真实下单为 `live-main` |
| 真实账户快照 | `LIVE_ACCOUNT=live-main` |
| paper 权益 | `1000 USDT` |
| 看板端口 | `:8082` |
| 5m 行情/策略轮询 | `15s` |
| 15m / 1h K 线轮询 | `90s` / `5m` |
| paper hold 入库节流 | `PERSIST_INTERVAL=1m` |
| 回测入库节流 | `BACKTEST_INTERVAL=5m` |
| 资金费率轮询 | `30m` |
| mark price 保留 | `MARK_PRICE_RETENTION=168h` |
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
| `ONEBULLEX_LIVE_TRADING=true SUBMIT_EXCHANGE=false ./scripts/start-paper-local.sh` | 用真实 key 只读同步真实账户 + 真实行情，但策略仍 dry-run，不真实下单。 |
| `PROFILE=manual INTERVAL=5m EQUITY=10000 ./scripts/start-paper-local.sh` | 使用更保守的 `5m` 手动档。 |
| `TREND_FILTER=false ./scripts/start-paper-local.sh` | 临时关闭高周期趋势过滤，只建议调试时使用。 |
| `PERSIST_INTERVAL=30s BACKTEST_INTERVAL=10m ./scripts/start-paper-local.sh` | 调整 paper 入库和回测入库频率。 |
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
| `.runtime/bin/` | 本地编译出的可执行文件（启动脚本自动构建）。 |
| `.runtime/paper-local-stack.ready` | 启动就绪标记；没有这个文件说明栈未真正起来。 |
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

并且 `.env.local` 里已配置有效的 `ONEBULLEX_API_KEY` / `ONEBULLEX_SECRET_KEY`。

只设置其中一个不会进入完整真实交易模式。日常调试优先使用第一条模拟命令。
