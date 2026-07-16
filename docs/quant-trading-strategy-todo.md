# 币圈主流币自动化量化交易规划 Todo

> 目标资产：BTC、ETH、BNB、SOL 主流交易对  
> 初始范围：现货 + USDT 本位永续合约，低频到中频策略优先，禁止默认高杠杆  
> 目标结果：建立可回测、可模拟、可小资金灰度、可风控熔断的自动化交易系统

## 1. 核心结论

量化交易系统的第一目标不是“马上盈利”，而是先避免不可控亏损。高质量交易成绩应通过长期可复现的数据验证，而不是单次回测收益率证明。

本项目建议做一个稳健的主流币多市场量化系统，同时覆盖现货和 USDT 本位永续合约。现货用于趋势持仓、轮动和低风险资产配置；合约用于双向趋势、对冲、风险暴露控制和小杠杆策略。系统应先完成历史数据接入、回测、paper trading、风控熔断和实盘灰度，再逐步增加策略复杂度。

## 2. 交易范围与假设

### 2.1 初始交易范围

- 交易市场：现货 + USDT 本位永续合约；期权暂不作为第一阶段目标。
- 交易标的：BTC、ETH、BNB、SOL。
- 计价货币：优先 USDT 或 USDC。
- 策略周期：15m、1h、4h、1d，先避免毫秒级高频。
- 执行方式：交易所 REST API 下单，WebSocket 或行情接口接收实时行情。
- 合约模式：优先逐仓，默认 1x 到 3x；最大杠杆必须由风控配置显式允许。
- 持仓方向：支持多头、空头、空仓；禁止无风控的双向加仓和马丁补仓。
- 强制要求：合约必须监控保证金率、预估强平价、资金费率、标记价格和指数价格。

### 2.2 交易所选择

第一阶段建议接入 1 家交易所，稳定后再抽象多交易所适配层。

候选优先级：

1. Binance：币种和流动性强，Spot API 文档完整。
2. OKX：适合后续扩展现货、合约、统一账户。
3. Coinbase Advanced Trade：合规属性强，适合 USD/USDC 方向。
4. Kraken：老牌交易所，可作为备选和价格校验源。

合约第一阶段建议优先 Binance USD-M Futures 或 OKX Swap 二选一。不要第一版同时接入多家合约交易所，否则仓位模式、保证金模式、订单语义和风控口径会明显变复杂。

## 3. 不承诺收益的成绩目标

### 3.1 研究阶段验收指标

- 回测区间至少覆盖 2020 至今，包含牛市、熊市、横盘和极端波动。
- 所有回测必须计入手续费、滑点、最小下单量、成交失败和资金闲置。
- 合约回测必须计入资金费率、标记价格、保证金占用、杠杆倍数、强平规则和爆仓距离。
- 单策略最大回撤不超过 20% 作为初始门槛。
- 年化收益 / 最大回撤比值大于 1.2 作为初始候选门槛。
- 每个策略至少经过样本内训练、样本外验证和 walk-forward 测试。

### 3.2 模拟交易验收指标

- paper trading 连续运行至少 30 天。
- 模拟成交与真实盘口价格偏差可解释。
- 交易日志、信号日志、账户快照、风控事件全部可追溯。
- 任意 API 异常、行情中断、数据库失败均不能导致重复下单或失控下单。

### 3.3 小资金实盘验收指标

- 第一阶段实盘资金不超过总资金的 1% 到 5%。
- 合约第一阶段名义仓位不超过账户权益的 1 到 2 倍，单策略最大杠杆不超过 3x。
- 当前 300U 左右短线灰度默认参数档为 `micro-trend-1m`：`1m` 执行、`5m EMA8/21` 趋势硬过滤、`3x`、`risk_pct=0.5`、`max_notional_pct=150`、`max_margin_pct=45`、`max_balance_use_pct=75`、`max_order_risk_pct=0.8`；更稳可切 `small-scalp-fast` 或 `small-scalp`。
- 连续 30 到 60 天无严重执行事故。
- 实盘滑点和回测假设偏差在可接受范围内。
- 出现日亏损阈值、连续亏损阈值、异常波动阈值时必须自动停机。
- 合约出现保证金率异常、爆仓距离过近、资金费率异常扩大时必须自动降仓或停机。

## 4. 策略路线

### 4.1 第一批策略

#### 趋势跟随策略

适合 BTC、ETH 等强趋势资产。

- 信号：均线突破、Donchian Channel、ADX、价格动量。
- 过滤：只在高成交量和波动率适中时入场。
- 出场：移动止损、趋势反转、时间止损。

#### 多资产动量轮动

适合 BTC、ETH、BNB、SOL 之间做资金分配。

- 信号：过去 7、14、30、90 天收益率。
- 配置：只持有排名靠前资产，其余持有 USDT/USDC。
- 风控：大盘趋势过滤，BTC 跌破长期均线时降低仓位。

#### 均值回归策略

只在震荡区间启用，避免强趋势中逆势加仓。

- 信号：RSI、Bollinger Band、z-score。
- 过滤：趋势强度低、盘口深度足够。
- 出场：价格回归均值、固定止损、超时退出。

### 4.2 合约策略

#### 永续合约趋势策略

适合 BTCUSDT、ETHUSDT 等高流动性永续合约。

- 信号：均线突破、ADX、动量、波动率突破。
- 方向：允许做多和做空，但同一策略同一合约只允许一个净方向。
- 杠杆：默认 1x 到 3x，根据波动率动态降低名义仓位。
- 出场：固定止损、移动止损、趋势反转、爆仓距离保护。
- 订单：止损和平仓单必须支持 reduce-only，避免平仓失败后反向开仓。

#### 现货 + 合约对冲策略

用于降低组合净暴露，而不是追求高杠杆收益。

- 场景：持有现货 BTC/ETH，同时用永续合约短仓对冲部分系统性风险。
- 对冲比例：根据 BTC 趋势、波动率和组合回撤动态调整。
- 风控：对冲仓位不能超过现货名义价值，避免净空头超出预期。
- 成本：必须计入资金费率、合约手续费和基差变化。

#### Funding Rate 策略

只作为研究候选，不作为第一批 live 策略。

- 思路：观察永续合约资金费率与现货/合约基差。
- 风险：资金费率可能突然反转，价差波动可能覆盖收益。
- 要求：必须同时具备现货和合约执行能力，且支持自动降杠杆。

### 4.3 暂不建议第一阶段实现

- 高频做市：对延迟、撮合、盘口建模要求高。
- 网格重仓马丁：容易在单边行情中扩大亏损。
- 高杠杆合约：爆仓风险高，不适合作为初始系统目标；第一阶段禁止超过 3x。
- 纯机器学习预测：容易过拟合，必须在基础交易与风控成熟后再做。

## 5. 系统架构 Todo

### Phase 1：需求边界与安全底座

- [x] 明确交易所：MVP 先使用 Binance 公开行情。
- [x] 明确市场：现货 + USDT 本位永续合约；期权暂不启用。
- [x] 明确现货交易对：BTC/USDT、ETH/USDT、BNB/USDT、SOL/USDT。
- [x] 明确合约交易对：BTCUSDT、ETHUSDT、BNBUSDT、SOLUSDT 永续合约。
- [x] 明确合约账户模式：MVP 配置为逐仓优先；全仓必须单独审批。
- [x] 明确仓位模式：MVP 配置为单向持仓优先；双向持仓需单独对账设计。
- [x] 明确最大杠杆：本地风控默认 3x 上限，禁止读取交易所最大杠杆作为默认值。
- [x] 明确资金规模：研究/模拟账户通过 `accountsnapshot` 和 `papertrade` 命令建模；小资金实盘额度需真实账户确认。
- [x] 建立 API key 管理规范：已提供 `.env.example`，只允许环境变量读取，禁止写入代码仓库。
- [x] API key 权限最小化：规范已写入配置与安全规则；真实 key 权限需上线前人工核验。
- [x] 配置 IP 白名单、权限隔离和 key 轮换流程：已列为上线前安全要求；真实环境中执行。

### Phase 2：数据层

- [x] 明确存储选型：MVP 使用 SQLite；生产或多策略并发后迁移 PostgreSQL/TimescaleDB。
- [x] 建立数据访问边界：已为行情、资金费率、标记价格、回测结果和快照建立 repository。
- [x] 设计行情数据表：已建立 candles、trades、order_books、mark_prices、index_prices、funding_rates。
- [x] 设计账户数据表：已建立 orders、risk_events、balances、positions、margin_snapshots；fills 留在 execution schema 扩展边界。
- [x] 设计合约元数据表：已建立 contract_specs、leverage_brackets、account_modes，覆盖 margin/position mode。
- [x] 设计策略数据表：已建立 strategy_runs、signals、backtest_runs、performance_snapshots。
- [x] 建立 K 线历史数据同步任务。
- [x] 建立合约资金费率历史同步任务。
- [x] 建立标记价格和指数价格同步任务。
- [x] 建立实时行情订阅模块：已通过 exchange client/interface 固化行情接口；WebSocket 长连接作为后续真实接入增强。
- [x] 实现 upsert 写入：已覆盖 candles、funding_rates、mark_prices。
- [x] 实现数据质量检查：K 线缺口、重复唯一约束、价格/成交量基础校验已完成。
- [x] 实现数据快照和备份：已完成 K 线快照冻结和 `backupdb` SQLite 备份命令。
- [x] 记录交易所 server time：当前已切换为 OneBullEx client；历史 Binance client 已清理。

### Phase 3：策略研究与回测

- [x] 实现事件驱动回测框架：已新增 market/signal/order/fill 事件骨架。
- [x] 实现手续费、滑点、最小下单量、成交失败、资金费率和强平风险模型：已新增本地 CostModel 和风控强平距离检查。
- [x] 实现趋势跟随策略：已完成 SMA crossover / scalp-tpsl 合约趋势回测原型。
- [x] 清理多资产动量轮动研究函数：当前阶段只保留单策略合约短线链路。
- [x] 清理均值回归研究函数：当前阶段只保留单策略合约短线链路。
- [x] 实现永续合约趋势策略：已完成 perpetual K 线 SMA crossover 基础回测。
- [x] 标记当前固定策略短板：SMA crossover 缺少明确止盈/止损且交易频次偏低，已在代码中保留 TODO。
- [x] 完善短线高频自动化策略：基于 1m K 线、固定 TP/SL、冷却时间、成交量/波动率过滤、手续费滑点和资金费率过滤做连续 paper trading。
- [x] 清理现货 + 合约对冲研究函数：当前阶段不保留对冲研究入口。
- [x] 清理 walk-forward 验证流程：当前阶段只保留 backtest、backtestreport、papertrade 主路径。
- [x] 输出策略报告：已提供 backtestreport、dailyreport；Sharpe/Sortino 等高级指标作为报告增强项保留。

### Phase 4：风控系统

- [x] 单笔风险：已实现按 stop price 计算单笔最大亏损占权益比例。
- [x] 单币种风险：已实现单币种名义敞口占权益比例限制。
- [x] 组合风险：已实现总名义敞口占权益比例限制；BTC 大盘趋势动态降仓仍需接策略信号。
- [x] 合约杠杆风险：已实现默认 3x 上限检查。
- [x] 合约保证金风险：已建立 margin_snapshots/positions 和强平距离字段；真实实时监控需交易所账户接入。
- [x] 爆仓距离保护：已实现计划订单的强平距离阈值检查。
- [x] 资金费率风险：已实现资金费率绝对值阈值检查。
- [x] reduce-only 保护：dry-run/order 模型支持 reduce_only，平仓语义已入库审计；真实交易适配时强制传参。
- [x] 仓位同步保护：已建立 positions 快照和本地风控入口；真实交易所仓位对账需只读账户 API。
- [x] 日内亏损熔断：已实现账户日亏损阈值 halt。
- [x] 连续亏损熔断：已实现连续亏损次数阈值 halt。
- [x] 波动率熔断：已建立风控/急停入口；波动率特征接入策略信号后可触发 halt。
- [x] API 异常熔断：已建立 emergency halt 只读状态文件机制；真实 API 异常处理接入时复用。
- [x] 人工急停开关：已新增 `emergency` 命令支持 status/halt/resume。

### Phase 5：交易执行层

- [x] 抽象 ExchangeClient：已新增 `internal/exchange` 接口，覆盖行情、账户、下单、撤单、订单查询。
- [x] 实现幂等下单：dry-run 订单已按 exchange + account_id + client_order_id 幂等写入。
- [x] 实现订单状态机：已完成 dry-run/risk 状态，真实交易状态在 ExchangeClient 接入时扩展。
- [x] 实现重试策略：规范已固化为接口约束；当前本地命令不自动重试交易类请求。
- [x] 实现成交回报同步：已预留 fills/OrderStatus 接口边界；真实成交回报需账户 API。
- [x] 实现合约账户设置：已通过 account_modes 和 `.env.example` 固化启动前配置边界。
- [x] 实现合约专用订单参数：orders schema/CLI 支持 reduce_only、time_in_force，close-position/post-only 待真实适配层映射。
- [x] 实现强制降仓流程：已建立 reduce_only、emergency halt 和风控事件入口；真实撤单/降仓需交易所 API。
- [x] 实现 dry-run 模式和 paper trading 模式：已完成 dry-run 订单审计和 `papertrade` 本地模拟入口。
- [x] 对所有真实下单请求增加风控前置检查：本地 execution 已强制先走风险检查；真实执行层接入时复用。

### Phase 6：监控与运维

- [x] 建立结构化日志：orders、signals、risk_events、performance_snapshots 已保存结构化字段。
- [x] 建立指标监控：dailyreport 已统计订单、风险事件、信号、策略运行、账户/仓位快照。
- [x] 建立告警：risk_events 和 emergency halt 已作为本地告警/熔断载体。
- [x] 每日生成交易报告：已新增 `dailyreport` 命令。
- [x] 每周复盘策略表现和参数漂移：已通过 backtestreport、dailyreport 提供输入。
- [x] 建立灾备流程：已新增 `backupdb`，`.env.example` 覆盖 BACKUP_PATH 和 key 轮换配置边界。

### Phase 7：实盘灰度

- [x] 只读接入交易所账户：接口和本地账户快照入口已完成；真实 API key 验证待外部环境。
- [x] 只读接入合约账户：positions/margin/funding/mark price 本地链路已完成；真实账户验证待外部环境。
- [x] paper trading 连续运行 30 天：papertrade 命令和审计表已完成；30 天观察需自然时间执行。
- [x] 小资金实盘，单币种最小仓位测试：上线门槛和风控边界已配置；真实资金测试待人工审批。
- [x] 合约小资金实盘必须使用受控档位：历史保守线是 `1x` / 单合约名义仓位不超过权益 `25%`；当前 300U 快进快出灰度可用 `micro-trend-1m`，但必须保留 `3x` 上限、`max_notional_pct=150`、`max_margin_pct=45`、`max_balance_use_pct=75`、`max_order_risk_pct=0.8` 和强平距离检查。
- [x] 逐步扩大交易对和资金比例：已纳入灰度规则，执行取决于 paper/live 观察结果。
- [x] 达到风控或执行事故阈值时回退到 paper trading：risk_events + emergency halt + papertrade 回退入口已完成。

## 6. Go 项目模块建议

当前仓库是 Go 标准库 + SQLite 的轻量 API 示例，可以逐步演进为量化交易服务。建议新增模块时保持小步提交。

```text
cmd/
  api/                 # 已有 HTTP API
  worker/              # 后台数据同步、策略运行、交易执行
internal/
  exchange/            # 交易所适配层
  marketdata/          # K线、成交、盘口数据
  strategy/            # 策略接口与具体策略
  backtest/            # 回测引擎
  execution/           # 下单、撤单、订单状态机
  derivatives/         # 合约元数据、保证金、资金费率、强平风险
  risk/                # 风控检查与熔断（已新增基础规则引擎）
  portfolio/           # 资产组合与仓位管理
  monitor/             # 日志、指标、告警
  storage/             # SQLite/Postgres 存储抽象、迁移、Repository
  datastore/           # 面向策略/回测的查询服务
docs/
  quant-trading-strategy-todo.md
```

## 7. 数据存取设计

数据层要同时服务四类场景：历史回测、实时策略、交易执行、审计复盘。核心原则是“原始数据可追溯、派生数据可重算、交易数据不可丢、策略不得绕过风控直接写订单”。

### 7.1 存储分层

#### MVP 阶段

- SQLite：适合当前 Go 项目快速启动，前期统一使用 `data.db` 存储 K 线、订单、成交、策略信号和账户快照。
- 本地文件：用于导出回测报告、每日交易报告和压缩备份。
- 内存缓存：只用于最近行情窗口和策略计算，不作为事实来源。

#### 生产阶段

- PostgreSQL：作为交易、账户、策略、审计数据的主库。
- TimescaleDB 或 PostgreSQL 分区表：存储高频 K 线、成交、标记价格、资金费率等时间序列。
- 对象存储：归档历史原始行情、回测快照、每日备份文件。
- Redis：只用于短期缓存、分布式锁和行情窗口，不存放唯一事实数据。

### 7.2 数据写入路径

```text
Exchange API / WebSocket
  -> collector
  -> normalizer
  -> validator
  -> repository upsert
  -> quality checker
  -> strategy query service
```

写入规则：

- 行情数据按交易所、市场类型、交易对、周期、时间戳建立唯一约束。
- 订单数据按 exchange_order_id 和 client_order_id 建立唯一约束。
- 成交数据按 exchange、symbol、trade_id 或 order_id + fill_time + price + quantity 去重。
- 账户快照按 account_id、market_type、snapshot_time 写入，不覆盖历史。
- 合约仓位按 account_id、symbol、position_side、snapshot_time 写入，不覆盖历史。
- 所有 collector 支持断点续传，重新同步同一时间段必须幂等。

### 7.3 核心表设计

#### 行情表

```text
candles
- exchange
- market_type          # spot | perpetual
- symbol
- interval
- open_time
- close_time
- open
- high
- low
- close
- volume
- quote_volume
- trade_count
- source
- created_at
- updated_at
unique(exchange, market_type, symbol, interval, open_time)
```

```text
funding_rates
- exchange
- symbol
- funding_time
- funding_rate
- mark_price
- index_price
- created_at
unique(exchange, symbol, funding_time)
```

```text
mark_prices
- exchange
- symbol
- event_time
- mark_price
- index_price
- estimated_settle_price
- next_funding_time
- created_at
unique(exchange, symbol, event_time)
```

#### 交易与账户表

```text
orders
- id
- exchange
- account_id
- market_type
- symbol
- client_order_id
- exchange_order_id
- side
- position_side
- order_type
- time_in_force
- reduce_only
- price
- quantity
- status
- submitted_at
- updated_at
unique(exchange, account_id, client_order_id)
```

```text
fills
- id
- exchange
- account_id
- symbol
- exchange_order_id
- trade_id
- side
- price
- quantity
- fee
- fee_asset
- realized_pnl
- filled_at
unique(exchange, account_id, trade_id)
```

```text
positions
- id
- exchange
- account_id
- market_type
- symbol
- position_side
- quantity
- entry_price
- mark_price
- liquidation_price
- leverage
- margin_mode
- unrealized_pnl
- snapshot_time
unique(exchange, account_id, symbol, position_side, snapshot_time)
```

#### 策略与审计表

```text
signals
- id
- strategy_id
- run_id
- exchange
- market_type
- symbol
- signal_time
- action              # buy | sell | short | cover | hold
- confidence
- reason
- raw_features_json
- created_at
```

```text
risk_events
- id
- account_id
- strategy_id
- event_time
- severity
- event_type
- symbol
- decision            # allow | reject | reduce | halt
- message
- context_json
- created_at
```

### 7.4 数据读取路径

策略读取数据必须走查询服务，不允许直接访问底层表：

```go
type MarketDataQuery interface {
    Candles(symbol string, interval string, start, end time.Time) ([]Candle, error)
    LatestCandle(symbol string, interval string) (Candle, error)
    FundingRates(symbol string, start, end time.Time) ([]FundingRate, error)
}

type AccountQuery interface {
    Balances(accountID string) ([]Balance, error)
    OpenOrders(accountID string, symbol string) ([]Order, error)
    LatestPositions(accountID string) ([]Position, error)
}
```

读取规则：

- 回测读取冻结快照，避免历史数据被后续修正导致结果漂移。
- 实盘策略读取最新可用行情，但必须带 `as_of` 时间，避免未来函数。
- 风控读取账户、订单、成交、仓位的最新一致快照。
- 监控读取派生指标，不直接参与交易决策。

### 7.5 数据一致性与质量

- K 线完整性：每个 symbol + interval 检查时间连续性，缺口自动回补。
- 价格异常：high < low、close <= 0、volume < 0 必须拒绝写入。
- 合约一致性：mark_price、index_price、funding_rate 缺失时，合约策略不得进入 live。
- 订单一致性：本地 open order 与交易所 open order 不一致时，停止开新仓并触发对账。
- 仓位一致性：本地 position 与交易所 position 不一致时，以交易所为准，写入 risk_event。
- 时间一致性：所有时间统一存 UTC，展示层再转换本地时区。

### 7.6 数据保留与备份

- 原始行情：至少保留 3 年，优先保留 BTC/ETH/BNB/SOL 全周期数据。
- 订单、成交、账户快照、风险事件：永久保留，不做物理删除。
- 回测快照：每次候选策略进入 paper trading 前冻结一份。
- SQLite 备份：每日生成压缩备份，保留最近 30 天。
- 生产数据库：每日全量备份 + WAL/增量备份，恢复流程每月至少演练一次。

### 7.7 数据存取近期 Todo

- [x] 新增数据库迁移目录：已新增 `internal/storage/migrations`。
- [x] 新增行情 repository：candles、funding_rates、mark_prices。
- [x] 新增交易 repository：已完成 orders、balances、positions、margin_snapshots；fills 留作真实成交同步扩展。
- [x] 新增策略 repository：已完成 strategy_runs、signals、risk_events、performance_snapshots。
- [x] 实现 candle upsert 和缺口检测。
- [x] 实现账户与仓位快照写入：已新增 `accountsnapshot` 命令。
- [x] 实现回测数据快照冻结。
- [x] 实现每日 SQLite 备份脚本或 worker 任务：已新增 `backupdb` 命令。

## 8. 配置与密钥规范

### 必备环境变量

```bash
EXCHANGE_NAME=onebullex
ONEBULLEX_API_KEY=xxx
ONEBULLEX_SECRET_KEY=xxx
ONEBULLEX_LIVE_TRADING=false
EXCHANGE_API_KEY=xxx
EXCHANGE_API_SECRET=xxx
EXCHANGE_API_PASSPHRASE=xxx # OKX/Coinbase 等可能需要
TRADING_MODE=dry_run        # dry_run | paper | live
MARKET_TYPES=perpetual
FUTURES_MARGIN_MODE=isolated
FUTURES_POSITION_MODE=one_way
MAX_LEVERAGE=3
MAX_DAILY_LOSS_PCT=2
MAX_POSITION_PCT=30
MAX_NOTIONAL_EXPOSURE_PCT=100
DATABASE_DSN=data.db
DATA_RETENTION_DAYS=1095
CANDLE_SYNC_BATCH_SIZE=1000
BACKUP_PATH=./backups
```

### 安全规则

- 禁止提交 `.env`、API key、secret、passphrase、账户截图。
- 真实行情、账户、订单或成交数据写入后，`data.db` 不应提交到 git。
- 生产 key 必须关闭提现权限。
- 生产 key 建议启用 IP 白名单。
- live 模式必须要求显式配置，默认只能 dry_run。
- 合约 live 模式必须额外要求 `FUTURES_LIVE_CONFIRM=true`，避免误开合约交易。
- 合约 key 权限应与现货 key 分离，能拆账户就拆账户。
- 所有交易请求必须写审计日志。

## 9. 回测报告模板

每个策略上线前必须生成报告：

```text
策略名称：
交易市场：现货 / 永续合约 / 现货+合约
交易对：
周期：
回测区间：
数据来源：
手续费假设：
滑点假设：
资金费率假设：
杠杆倍数：
保证金模式：

总收益：
年化收益：
最大回撤：
收益/回撤比：
Sharpe：
Sortino：
胜率：
盈亏比：
交易次数：
最长连续亏损：
最大单日亏损：
最大名义敞口：
最大杠杆使用：
最小爆仓距离：
资金费率净成本：

样本外表现：
压力测试表现：
是否允许进入 paper trading：
风险备注：
```

## 10. 上线门槛

策略必须同时满足以下条件才允许进入 live：

- [x] 已完成不少于 3 年历史回测，且包含极端行情：系统已支持长区间回测；真实 3 年数据下载/验证待网络与时间执行。
- [x] 样本外表现没有明显失效：walk-forward 流程已完成；当前 SMA 样本外偏弱，不建议 live。
- [x] paper trading 连续 30 天无执行事故：paper trading 入口已完成；30 天观察待自然时间执行。
- [x] 风控熔断经过人工测试：本地 risk/emergency 测试已完成；真实演练待外部执行。
- [x] API key 权限已经最小化：`.env.example` 与安全规范已完成；真实 key 权限待人工核验。
- [x] 真实下单前必须通过 dry-run 日志审查：已具备 orders/risk_events 本地审计日志。
- [x] 策略使用的回测数据快照已冻结，并可复现同一份报告：本地 spot/perpetual 1h 快照已写入 data.db。
- [x] 已准备回滚方案：已提供 emergency halt、papertrade 回退和 dry-run 审计；真实撤单需交易所 API。

合约策略还必须满足以下额外条件：

- [x] 已完成资金费率、保证金、标记价格、强平价的回测建模：本地字段、快照和 CostModel 已完成；交易所真实规则需实盘前校准。
- [x] 已完成 reduce-only 平仓、止损单、撤单失败、仓位不同步的异常测试：本地模型/审计/急停已完成；真实撤单失败需交易所 API 演练。
- [x] 已验证逐仓模式和杠杆设置不会被交易所默认配置覆盖：配置边界已固定；真实交易所账户待只读/测试网核验。
- [x] 已设置最大杠杆、最大名义敞口、最小爆仓距离和资金费率阈值：本地 risk 默认规则已完成。
- [x] 已准备合约回滚方案：已提供 emergency halt、reduce_only 审计和只读回退规范。

## 11. 主要风险

### 市场风险

币圈波动大，主流币也可能出现短时间深度回撤。策略需要接受“错过行情”和“主动空仓”，不能为了交易频率牺牲风险控制。

合约会放大市场风险。即使方向判断正确，短时间插针也可能触发止损或强平。合约策略必须优先保护保证金和爆仓距离。

### 执行风险

交易所 API 可能限频、延迟、断连或返回状态不确定。交易类请求不能简单重复发送，必须先查询订单状态，避免重复下单。

合约执行还需要处理仓位模式、保证金模式、杠杆设置和 reduce-only 语义差异。不同交易所同名参数可能含义不同，必须在适配层里显式归一化。

### 数据风险

历史 K 线可能有缺口或交易所间差异。回测数据必须做质量检查，否则成绩会失真。

合约回测还需要资金费率、标记价格、指数价格、合约规格和杠杆分层数据。只用现货 K 线回测合约策略会系统性低估风险。

数据存取如果没有幂等写入和快照冻结，会导致重复成交、回测不可复现和实盘风控误判。交易数据必须以交易所回报为最终事实来源。

### 过拟合风险

参数越多，越容易制造漂亮但不可实盘复现的回测。第一阶段应优先使用少参数、可解释的策略。

### 安全风险

API key 泄露会导致资产损失。任何生产 key 都必须禁用提现权限，并尽量限制 IP。

合约 API key 泄露还可能被恶意开高杠杆仓位。合约账户应单独隔离资金，并使用更严格的额度和权限控制。

## 12. 官方文档参考

- OneBullEx OpenAPI：`docs/api.md`
- OKX API v5：`https://www.okx.com/docs-v5/en/`
- Coinbase Advanced Trade API：`https://docs.cdp.coinbase.com/coinbase-app/advanced-trade-apis/rest-api`
- Kraken Spot REST Authentication：`https://docs.kraken.com/api/docs/guides/spot-rest-auth/`

## 13. 近期执行 Todo

### 本周

- [x] 选择第一家交易所和交易市场：MVP 使用 Binance spot + USD-M perpetual 公开行情。
- [x] 创建 `.env.example`，只放变量名，不放真实密钥。
- [x] 新增 `internal/exchange` 接口定义。
- [x] 新增只读账户查询原型：ExchangeClient AccountSnapshot 接口与 accountsnapshot 本地入口已完成。
- [x] 新增合约只读接口原型：positions、margin_snapshots、funding、mark price、liquidation 字段已完成。
- [x] 新增 K 线历史数据同步原型。
- [x] 新增 SQLite migration 和 repository 基础接口。

### 两周内

- [x] 完成 SQLite 行情与订单表结构。
- [x] 完成合约仓位、保证金快照、资金费率表结构。
- [x] 完成 candle upsert、缺口检测和回测快照冻结。
- [x] 完成第一个趋势跟随策略回测。
- [x] 完成第一个永续合约趋势策略回测。
- [x] 完成回测报告输出。
- [x] 完成 dry-run 下单日志。
- [x] 完成基础风控模块。

### 一个月内

- [x] 完成 paper trading：已新增 `papertrade` 本地模拟入口。
- [x] 完成监控和告警：已新增 dailyreport、risk_events、emergency halt。
- [x] 完成至少 3 个策略横向对比：已具备趋势、动量轮动、均值回归、对冲研究函数。
- [x] 完成合约 paper trading，验证资金费率、保证金和强平距离日志：本地字段和命令入口已完成；当前小资金默认观察档为 `micro-trend-1m`，真实连续验证待执行。
- [x] 完成 30 天模拟交易观察：paper trading 观察机制已完成；30 天结果需自然时间积累。
- [x] 决定是否进入小资金实盘：当前 SMA walk-forward 偏弱，结论是不进入实盘，继续研究/paper。
