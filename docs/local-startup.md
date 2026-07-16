# 本地启动脚本速查

> 更新时间：2026-07-15
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

## 当前推荐启动命令

> 现在默认策略档是 `micro-trend-1m`：`1m` 执行信号 + `5m EMA8/21` 趋势硬过滤。
> 只要想切换 paper / 真实下单 / profile，先停旧服务，否则旧进程还会继续用旧参数跑。

### A. 300U 小资金短线模拟：推荐先跑这个

这条使用 OneBullEx 线上真实行情，但只在本地模拟仓位，**不会提交真实订单**。适合先观察 `micro-trend-1m` 是否能提高 1 分钟级别短线成交频率。

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

### B. 真实下单：确认要实盘时再跑

这条会使用 `.env.local` 里的真实 key，并且会向 OneBullEx 提交真实订单。真实下单风控资金优先使用 `LIVE_ACCOUNT=live-main` 的真实 USDT 快照，`EQUITY` 不作为实盘资金来源。

```bash
cd /Users/mac/Desktop/yoyo/gogogo
./scripts/stop-paper-local.sh

ONEBULLEX_LIVE_TRADING=true \
SUBMIT_EXCHANGE=true \
SYMBOL=BTCUSDT \
PROFILE=micro-trend-1m \
POSITION_MODEL=AGGREGATION \
./scripts/start-paper-local.sh
```

启动后看状态和策略日志：

```bash
./scripts/status-paper-local.sh
tail -f .runtime/logs/papertrade.log
```

停止：

```bash
./scripts/stop-paper-local.sh
```

## 详细说明：两条一键命令

### 1. OneBullEx 线上真实数据 + 本地模拟仓位 + 回测数据入库

这条只用 OneBullEx 公开线上行情，**不会提交真实订单**；策略会写入本地 paper 仓位、订单、风控事件、回测结果和行情数据。

```bash
cd /Users/mac/Desktop/yoyo/gogogo

ONEBULLEX_LIVE_TRADING=false \
SUBMIT_EXCHANGE=false \
SYMBOL=BTCUSDT \
PROFILE=micro-trend-1m \
EQUITY=300 \
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

ONEBULLEX_LIVE_TRADING=true \
SUBMIT_EXCHANGE=true \
SYMBOL=BTCUSDT \
PROFILE=micro-trend-1m \
POSITION_MODEL=AGGREGATION \
./scripts/start-paper-local.sh
```

默认跑 `BTCUSDT` / `micro-trend-1m`。普通模拟建议用 `EQUITY=300` 对齐小资金场景；真实下单时风控会优先使用 `LIVE_ACCOUNT=live-main` 的真实 USDT 快照，不会把本地模拟余额混写到 `live-main`。要改参数就在同一行追加覆盖，例如 `SYMBOL=ETHUSDT EQUITY=300`。

## 小资金短线档位备忘

下次忘记时先看这里：

- 当前一键脚本默认已经是 `PROFILE=micro-trend-1m`，不需要再手动切 `aggressive`。
- 300U 左右账户优先用 `micro-trend-1m`；`small-scalp-fast` 是稍稳一点的 5m 快速档，`small-scalp` 更保守，`aggressive` 只作为 paper/backtest 研究档。
- 模拟跑 300U 效果时用 `PROFILE=micro-trend-1m EQUITY=300 ./scripts/start-paper-local.sh`；真实下单时 `EQUITY` 只影响本地兜底展示，风控资金以 `LIVE_ACCOUNT=live-main` 最新真实 USDT 余额快照为准。
- `micro-trend-1m` 核心参数：`1m` 执行、`5m EMA8/21` 趋势过滤、`3x`、`risk_pct=0.5`、`max_margin_pct=45`、`max_balance_use_pct=75`、`max_order_risk_pct=0.8`、`min_signal_score=0.35`、`min_volume_ratio=0.50`、`max_entry_extension_pct=0.50`、`max_candle_age=2m`。
- BTCUSDT 仍受交易所最小下单量约束：`min_order_quantity=0.001 BTC`。余额太低时，即使用 `micro-trend-1m`，也可能被最小数量、保证金占用、可用余额占用或强平距离风控拦住，这是正常保护。
- 切换 paper / 真实下单 / profile 前先执行 `./scripts/stop-paper-local.sh`，否则旧进程还在，新参数不会生效。

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
| `tail -f .runtime/logs/marketsync-klines-1m.log` | 查看执行周期 K 线同步日志，默认 `micro-trend-1m` 是 `1m`。 |
| `tail -f .runtime/logs/marketsync-klines-5m.log` | 查看趋势周期 K 线同步日志；`micro-trend-1m` 的趋势和宏观趋势都用 `5m`。 |
| `tail -f .runtime/logs/marketsync-klines-15m.log` | 查看 5m 快速档趋势 K 线日志；`small-scalp-fast` / `small-scalp` 才需要 `15m`。 |
| `tail -f .runtime/logs/marketsync-klines-1h.log` | 查看宏观趋势周期 K 线同步日志；仅 aggressive/手动指定 `MACRO_TREND_INTERVAL=1h` 时会启动。 |
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
| `marketsync-klines-1m` | 持续同步执行周期 K 线；默认 `micro-trend-1m` 会启动。 |
| `marketsync-klines-5m` | 持续同步趋势过滤 K 线；默认 `micro-trend-1m` 用 `5m EMA8/21`。 |
| `marketsync-klines-15m` | 持续同步 5m 快速档趋势 K 线；`small-scalp-fast` / `small-scalp` 才需要。 |
| `marketsync-klines-1h` | 持续同步宏观趋势确认 K 线；aggressive 或手动指定 `MACRO_TREND_INTERVAL=1h` 时启动。 |
| `marketsync-mark-price` | 持续同步真实 mark price。 |
| `marketsync-funding` | 持续同步资金费率。 |
| `accountsnapshot` | `ONEBULLEX_LIVE_TRADING=true` 时持续只读同步真实余额、持仓和仓位配置。 |
| `papertrade` | 持续运行策略；默认按新 K 线/节流间隔入库，`SUBMIT_EXCHANGE=true` 时真实下单。 |
| `dashboard` | 启动本地看板。 |

默认参数：

| 参数 | 默认值 |
| --- | --- |
| 交易对 | `BTCUSDT` |
| 策略档位 | `micro-trend-1m` |
| K 线周期 | `1m` 执行信号，`5m` 趋势过滤 |
| paper 账户 | 普通 paper 为 `paper`；真实 key dry-run 为 `paper-live-main`；真实下单为 `live-main` |
| 真实账户快照 | `LIVE_ACCOUNT=live-main` |
| paper 权益 | 模拟模式默认 `1000 USDT`；真实下单模式使用最新真实 USDT 余额做风控资金 |
| 看板端口 | `:8082` |
| 1m / 5m 行情轮询 | `15s` / `15s`；1m 轮询更快，但策略仍只使用已闭合 1m K 线 |
| 15m / 1h K 线轮询 | `90s` / `5m`；默认 `micro-trend-1m` 不单独启动 `15m` / `1h` |
| paper hold 入库节流 | `PERSIST_INTERVAL=1m` |
| 回测入库节流 | `BACKTEST_INTERVAL=5m` |
| 资金费率轮询 | `30m` |
| mark price 保留 | `MARK_PRICE_RETENTION=168h` |
| 趋势过滤 | 默认开启；`micro-trend-1m` 使用 `5m EMA8/21`，`small-scalp-fast` 使用 `15m EMA8/21`，aggressive/默认手动档使用 `15m EMA20/60 + 1h EMA20/60` |
| 保本止损 | 默认开启，浮盈达到 `1R` 后移动到保本价 |
| ATR 追踪止损 | 默认开启，浮盈达到 `1.5R` 后按 `1.2 * ATR` 追踪 |
| 实盘交易 | 默认关闭；必须同时设置 `ONEBULLEX_LIVE_TRADING=true` 和 `SUBMIT_EXCHANGE=true` |

## 常用变体

| 命令 | 干啥 |
| --- | --- |
| `SYMBOL=ETHUSDT ./scripts/start-paper-local.sh` | 改跑 `ETHUSDT`。 |
| `EQUITY=2000 ./scripts/start-paper-local.sh` | 改 paper 初始权益为 `2000 USDT`。 |
| `PROFILE=micro-trend-1m EQUITY=300 ./scripts/start-paper-local.sh` | 按 300U 左右小资金 1m 短线趋势档运行，成交频率最高。 |
| `PROFILE=small-scalp-fast EQUITY=300 ./scripts/start-paper-local.sh` | 切回 5m 快速档，成交频率低于 1m 档。 |
| `PROFILE=small-scalp EQUITY=300 ./scripts/start-paper-local.sh` | 用更稳的小资金短线档，成交频率低于 fast。 |
| `PROFILE=aggressive ./scripts/start-paper-local.sh` | 切回更激进的研究档，保证金和单笔风险更高。 |
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
