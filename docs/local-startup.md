# 本地启动脚本速查

> 更新时间：2026-07-14  
> 工作目录：`/Users/guilinzhou/Desktop/test-nemo/gogogo`  
> 默认链路：OneBullEx 永续合约 `BTCUSDT`，真实行情入库，本地 paper 模拟交易，Dashboard 看板。

## 先进入项目

```bash
cd /Users/guilinzhou/Desktop/test-nemo/gogogo
```

## 日常只用这些命令

| 命令 | 干啥 |
| --- | --- |
| `./scripts/start-paper-local.sh` | 一键后台启动整套本地 paper 服务。 |
| `./scripts/status-paper-local.sh` | 查看服务是否在跑、PID 和日志路径。 |
| `./scripts/stop-paper-local.sh` | 停止整套本地 paper 服务。 |
| `tail -f .runtime/logs/papertrade.log` | 查看 paper 策略运行日志。 |
| `tail -f .runtime/logs/marketsync-klines.log` | 查看 K 线同步日志。 |
| `tail -f .runtime/logs/marketsync-mark-price.log` | 查看 mark price 同步日志。 |
| `tail -f .runtime/logs/marketsync-funding.log` | 查看资金费率同步日志。 |
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
| `marketsync-klines` | 持续同步 OneBullEx 合约 K 线。 |
| `marketsync-mark-price` | 持续同步真实 mark price。 |
| `marketsync-funding` | 持续同步资金费率。 |
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
| 实盘交易 | 强制关闭 |

## 常用变体

| 命令 | 干啥 |
| --- | --- |
| `SYMBOL=ETHUSDT ./scripts/start-paper-local.sh` | 改跑 `ETHUSDT`。 |
| `EQUITY=2000 ./scripts/start-paper-local.sh` | 改 paper 初始权益为 `2000 USDT`。 |
| `HTTP_ADDR=:8083 ./scripts/start-paper-local.sh` | 改看板端口为 `8083`。 |
| `PROFILE=manual INTERVAL=5m EQUITY=10000 ./scripts/start-paper-local.sh` | 使用更保守的 `5m` 手动档。 |
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

脚本内部会强制：

```bash
ONEBULLEX_LIVE_TRADING=false
```

所以日常启动不要手动加 `-submit-exchange=true`，也不要把 `ONEBULLEX_LIVE_TRADING` 改成 `true`。
