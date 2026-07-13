OneBullEx Open API 文档
版本: 2.1.1
日期: 2026-04-20
团队: OneBullEx Team
官网: https://www.onebullex.com
支持邮箱: it@onebullex.com

目录
简介
API 概述
认证方式
公开接口 (V2)
私有接口 (V2)
WebSocket API
错误码
常见问题
支持
1. 简介
1.1 概述
OneBullEx Open API 为量化机构和第三方开发者提供专业的交易接口访问。我们的 API 支持：

行情数据: 实时合约行情数据
合约交易: V2 版本提供完整的合约交易功能
账户管理: 查询持仓、资产和订单历史
1.2 API 版本说明
V2 API: 合约交易接口
测试环境: https://futures-openapi.1bullex.com
生产环境: https://futures-openapi.onebullex.com
1.3 API 分类
公开接口: 无需认证，用于获取公共行情数据
私有接口: 需要认证，用于交易操作和账户查询
1.4 请求格式
Content-Type: application/json
字符编码: UTF-8
时间格式: Unix 时间戳（毫秒）
2. API 概述
2.1 V2 公开接口（无需认证）
API	方法	描述
/v2/public/time	GET	获取服务器时间
/v2/public/symbol/list	GET	获取所有交易对配置
/v2/public/symbol/detail	GET	获取单个交易对配置
/v2/public/symbol/coins	GET	获取交易对币种列表
/v2/public/symbol/all	GET	获取所有交易对名称
/v2/public/q/ticker	GET	获取指定交易对行情
/v2/public/q/tickers	GET	获取全交易对行情
/v2/public/q/agg-ticker	GET	获取指定交易对聚合行情
/v2/public/q/agg-tickers	GET	获取全交易对聚合行情
/v2/public/q/depth	GET	获取深度数据
/v2/public/q/deal	GET	获取最新成交记录
/v2/public/q/kline	GET	获取 K 线数据
/v2/public/q/mark-price	GET	获取标记价格
/v2/public/q/symbol-mark-price	GET	获取单个交易对标记价格
/v2/public/q/index-price	GET	获取指数价格
/v2/public/q/funding-rate	GET	获取资金费率
/v2/public/q/funding-rate-record	GET	获取资金费率历史记录
/v2/public/contract/risk-balance	GET	获取风险基金余额记录
/v2/public/contract/open-interest	GET	获取交易对持仓头寸
/v2/public/leverage/bracket/list	GET	查询所有交易对杠杆分层
/v2/public/leverage/bracket/detail	GET	查询单个交易对杠杆分层
2.2 V2 私有接口（需要认证）
API	方法	描述
/v2/user/listen-key	GET	获取 ListenKey
/v2/balance/list	GET	获取用户全部资金
/v2/balance/detail	GET	获取单币种资金
/v2/balance/bills	GET	获取账务流水
/v2/balance/funding	GET	获取资金账户(USDT)余额
/v2/balance/transfer	POST	资金划转
/v2/order/create	POST	下单
/v2/order/create-batch	POST	批量下单
/v2/order/listUnfinished	GET	查询当前未完成订单（单交易对）
/v2/order/all/listUnfinished	GET	查询当前未完成订单（多交易对）
/v2/order/list	GET	查询订单列表（分页）
/v2/order/list-by-ids	POST	根据 ID 列表查询订单
/v2/order/list-history	GET	查询历史订单
/v2/order/detail	GET	根据 ID 查询订单详情
/v2/order/cancel	POST	撤销订单
/v2/order/cancel-batch	POST	批量撤单
/v2/order/cancel-all	POST	撤销所有订单
/v2/order/trade-list	GET	查询成交明细
/v2/entrust/create-plan	POST	创建计划委托
/v2/entrust/create-profit	POST	创建止盈止损
/v2/entrust/plan-list	GET	查询当前计划委托
/v2/entrust/plan-list-history	GET	查询历史计划委托
/v2/entrust/plan-detail	GET	根据 ID 查询计划委托
/v2/entrust/profit-list	GET	查询当前止盈止损
/v2/entrust/profit-detail	GET	根据 ID 查询止盈止损
/v2/entrust/cancel-plan	POST	撤销计划委托
/v2/entrust/cancel-profit-stop	POST	撤销止盈止损
/v2/entrust/update-profit-stop	POST	修改止盈止损
/v2/entrust/cancel-all-plan	POST	撤销所有计划委托
/v2/entrust/cancel-all-profit-stop	POST	撤销所有止盈止损
/v2/order-entrust/list	GET	查询全部委托
/v2/order-entrust/cancel	POST	撤销委托
/v2/order-entrust/cancel-all	POST	撤销所有委托
/v2/position/list	GET	获取持仓信息
/v2/position/adjust-leverage	POST	调整杠杆倍数
/v2/position/margin	POST	修改逐仓保证金
/v2/position/auto-margin	POST	修改自动追加保证金
/v2/position/close-all	POST	一键平仓
/v2/position/merge	POST	合并仓位
/v2/position/change-type	POST	修改持仓模式
/v2/public/q/symbol-index-price	GET	获取单个交易对指数价格（需要认证）
3. 认证方式
3.1 申请 API Key
请联系 OneBullEx 官方团队申请 API 凭证：

邮箱: it@onebullex.com
官网: https://www.onebullex.com
3.2 V2 认证方式
所有私有接口（路径不以 /v2/public/ 开头）均需在请求头中携带以下字段：

Header 名称	是否必填	说明
X-API-KEY	是	平台分配的 API Access Key
X-Signature	是	请求签名，见签名算法
X-Nonce	是	随机字符串，每次请求唯一，防重放
X-Timestamp	是	当前时间戳（秒）
公开接口（路径以 /v2/public/ 开头）无需认证。

3.3 V2 签名算法
Step 1 — 构造参数集合

始终加入：nonce（来自 Header的X-Nonce）、timestamp（来自 Header的X-Timestamp）
GET 请求：将所有 Query 参数加入集合
POST 请求（JSON Body）：解析 JSON Body，将顶层基本类型字段（字符串、数字）加入集合，忽略 null 值和嵌套对象/数组
Step 2 — 按 Key 字典序排序

将所有参数按 Key 的 ASCII 字典序升序排列。

Step 3 — 拼接签名原文

key1=value1&key2=value2&key3=value3
Copy to clipboardErrorCopied
数字类型：整数保持整数形式，小数保持原始精度（不使用科学计数法）。

Step 4 — HMAC-SHA256 签名

使用平台分配的 Secret Key 对签名原文进行 HMAC-SHA256 计算，结果以十六进制字符串表示，填入 X-Signature Header。

GET 请求签名示例：

请求：GET /v2/public/q/ticker?symbol=btc_usdt&timeRangeType=UTC8

Headers:
  X-API-KEY: your_access_key
  X-Nonce: abc123xyz
  X-Timestamp: 1711900000
  X-Signature: <HMAC-SHA256 结果>

签名原文（排序后）：
  nonce=abc123xyz&symbol=btc_usdt&timeRangeType=UTC8&timestamp=1711900000
Copy to clipboardErrorCopied
POST 请求签名示例：

请求：POST /v2/order/create
Body: {"symbol":"btc_usdt","orderSide":"BUY","price":"50000","origQty":"1","orderType":"LIMIT","positionSide":"LONG","timeInForce":"GTC"}

Headers:
  X-API-KEY: your_access_key
  X-Nonce: def456uvw
  X-Timestamp: 1711900000
  X-Signature: <HMAC-SHA256 结果>

签名原文（排序后）：
  nonce=def456uvw&orderSide=BUY&orderType=LIMIT&origQty=1&positionSide=LONG&price=50000&symbol=btc_usdt&timeInForce=GTC&timestamp=1711900000
Copy to clipboardErrorCopied
3.4 V2 统一响应格式
{
  "returnCode": 0,
  "msgInfo": "success",
  "data": {}
}
Copy to clipboardErrorCopied
字段	类型	说明
returnCode	int	0 表示成功，非 0 表示失败
msgInfo	string	成功时为 success，失败时为错误码
data	any	业务数据，类型因接口而异
常见错误码：

错误码	说明
sign-error	签名验证失败（缺少 Header 或签名不匹配）
system-error	系统内部错误
invalid_symbol	无效交易对
3.5 V2 游标分页说明
部分接口使用游标翻页，响应 data 结构如下：

字段	类型	说明
items	array	数据列表
hasMore	boolean	是否有更多数据
翻页请求参数：

参数	说明
id	游标 ID，首次不传，后续传上一页最后一条记录的 id
direction	NEXT（下一页）/ PREV（上一页），默认 NEXT
limit	每页条数
4. 公开接口 (V2)
公开接口无需认证，用于获取公共行情数据和合约信息。

Base URL:

测试环境: https://futures-openapi.1bullex.com
生产环境: https://futures-openapi.onebullex.com
4.1 服务器时间
GET /v2/public/time — 获取服务器时间
请求参数： 无

响应示例：

{
  "returnCode": 0,
  "msgInfo": "success",
  "data": 1711900000
}
Copy to clipboardErrorCopied
字段	类型	说明
data	long	服务器当前时间戳（毫秒）
4.2 交易对信息
GET /v2/public/symbol/list — 获取所有交易对配置
请求参数： 无

响应 data： SymbolCacheDTO 数组

SymbolCacheDTO 字段定义：

字段	类型	说明
symbol	string	交易对，如 btc_usdt
contractType	string	合约类型（永续、交割）
underlyingType	string	标的类型（币本位、U本位）
contractSize	string	合约乘数（面值）
tradeSwitch	boolean	交易对开关
state	int	状态
initLeverage	int	初始杠杆倍数
initPositionType	string	初始仓位类型
baseCoin	string	标的资产
quoteCoin	string	报价资产
baseCoinPrecision	int	标的币种精度
baseCoinDisplayPrecision	int	标的币种显示精度
quoteCoinPrecision	int	报价币种精度
quoteCoinDisplayPrecision	int	报价币种显示精度
quantityPrecision	int	数量精度
pricePrecision	int	价格精度
supportOrderType	string	支持的订单类型
supportTimeInForce	string	支持的有效方式
supportEntrustType	string	支持的计划委托类型
supportPositionType	string	支持的仓位类型
minPrice	string	最小价格
minQty	string	最小数量
minNotional	string	最小名义价值
maxNotional	string	最大名义价值
multiplierDown	string	限价卖单价格下限百分比
multiplierUp	string	限价买单价格上限百分比
maxOpenOrders	int	最多挂单数
maxEntrusts	int	最多条件单数
makerFee	string	Maker 手续费率
takerFee	string	Taker 手续费率
liquidationFee	string	强平手续费率
marketTakeBound	string	市价单最大价格偏离
depthPrecisionMerge	int	盘口精度合并
labels	array<string>	标签
onboardDate	long	上线时间戳（毫秒）
enName	string	合约英文名称
cnName	string	合约中文名称
minStepPrice	string	最小价格变动单位
baseCoinName	string	标的币种名称
quoteCoinName	string	报价币种名称
GET /v2/public/symbol/detail — 获取单个交易对配置
请求参数：

参数	位置	类型	必填	说明
symbol	query	string	是	交易对，如 btc_usdt
响应 data： SymbolCacheDTO，字段定义同上。

GET /v2/public/symbol/coins — 获取交易对币种列表
请求参数： 无

响应示例：

{
  "returnCode": 0,
  "msgInfo": "success",
  "data": ["btc", "eth", "sql"]
}
Copy to clipboardErrorCopied
字段	类型	说明
data	array<string>	可用币种列表
GET /v2/public/symbol/all — 获取所有交易对名称
请求参数： 无

响应示例：

{
  "returnCode": 0,
  "msgInfo": "success",
  "data": ["btc_usdt", "eth_usdt", "sol_usdt"]
}
Copy to clipboardErrorCopied
字段	类型	说明
data	array<string>	所有交易对名称列表
4.3 行情接口
GET /v2/public/q/ticker — 获取指定交易对行情
请求参数：

参数	位置	类型	必填	说明
symbol	query	string	是	交易对
timeRangeType	query	string	是	时区类型，如 H24
TimeRangeType：

类型	释义
H24	24小时
UTC_P_12	utc+12
UTC_P_11	utc+11
UTC_P_10	
UTC_P_9	
UTC_P_8	
UTC_P_7	
UTC_P_6	
UTC_P_5	
UTC_P_4	
UTC_P_3	
UTC_P_2	
UTC_P_1	utc+1
UTC_P_0	utc+0
UTC_N_1	utc-1
UTC_N_2	
UTC_N_3	
UTC_N_4	
UTC_N_5	
UTC_N_6	
UTC_N_7	
UTC_N_8	
UTC_N_9	
UTC_N_10	
UTC_N_11	utc-11
UTC_N_12	utc-12
响应 data： TickerVO

字段	类型	说明
t	long	时间戳（毫秒）
s	string	交易对
c	string	最新价
h	string	24小时最高价
l	string	24小时最低价
a	string	24小时成交量
v	string	24小时成交额
o	string	24小时前第一笔成交价
r	string	24小时涨跌幅
GET /v2/public/q/tickers — 获取全交易对行情
请求参数：

参数	位置	类型	必填	说明
timeRangeType	query	string	是	时区类型
响应 data： TickerVO 数组，字段定义同上。

GET /v2/public/q/agg-ticker — 获取指定交易对聚合行情
请求参数：

参数	位置	类型	必填	说明
symbol	query	string	是	交易对
timeRangeType	query	string	是	时区类型
响应 data： AggTickerVO

字段	类型	说明
t	long	时间戳（毫秒）
s	string	交易对
c	string	最新价
h	string	24小时最高价
l	string	24小时最低价
a	string	24小时成交量
v	string	24小时成交额
o	string	24小时前第一笔成交价
r	string	24小时涨跌幅
i	string	指数价格
m	string	标记价格
bp	string	买一价格
ap	string	卖一价格
GET /v2/public/q/agg-tickers — 获取全交易对聚合行情
请求参数：

参数	位置	类型	必填	说明
timeRangeType	query	string	是	时区类型
响应 data： AggTickerVO 数组，字段定义同上。

GET /v2/public/q/depth — 获取深度数据
请求参数：

参数	位置	类型	必填	说明
symbol	query	string	是	交易对
level	query	int	是	档位，范围 1~50
响应 data： DepthVO

字段	类型	说明
t	long	时间戳（毫秒）
s	string	交易对
u	long	updateId
b	array<string[]>	买单列表，每项为 [价格, 数量]
a	array<string[]>	卖单列表，每项为 [价格, 数量]
GET /v2/public/q/deal — 获取最新成交记录
请求参数：

参数	位置	类型	必填	说明
symbol	query	string	是	交易对
num	query	long	否	返回条数，默认 50，最小 1
响应 data： DealVO 数组

字段	类型	说明
t	long	成交时间戳（毫秒）
s	string	交易对
p	string	成交价
a	string	成交量
m	string	买卖方向（BUY / SELL）
GET /v2/public/q/kline — 获取 K 线数据
请求参数：

参数	位置	类型	必填	说明
symbol	query	string	是	交易对
interval	query	string	是	时间间隔，如 1m、5m、1h、1d
startTime	query	long	否	起始时间戳（毫秒）
endTime	query	long	否	结束时间戳（毫秒）
limit	query	int	否	条数，默认 500，范围 1~1500
响应 data： KlineVO 数组

字段	类型	说明
s	string	交易对
t	long	时间戳（毫秒）
o	string	开盘价
c	string	收盘价
h	string	最高价
l	string	最低价
a	string	成交量
v	string	成交额
GET /v2/public/q/mark-price — 获取标记价格
请求参数：

参数	位置	类型	必填	说明
symbol	query	string	否	交易对，不传则返回所有交易对
响应 data： PriceVO 数组

字段	类型	说明
s	string	交易对
p	string	标记价格
t	long	时间戳（毫秒）
GET /v2/public/q/symbol-mark-price — 获取单个交易对标记价格
请求参数：

参数	位置	类型	必填	说明
symbol	query	string	是	交易对
响应 data： PriceVO，字段定义同上。

GET /v2/public/q/index-price — 获取指数价格
请求参数：

参数	位置	类型	必填	说明
symbol	query	string	否	交易对，不传则返回所有交易对
响应 data： PriceVO 数组，字段定义同上。

GET /v2/public/q/symbol-index-price — 获取单个交易对指数价格
需要认证（实测服务端要求携带签名头）

请求参数：

参数	位置	类型	必填	说明
symbol	query	string	是	交易对
响应 data： PriceVO，字段定义同上。

GET /v2/public/q/funding-rate — 获取资金费率
请求参数：

参数	位置	类型	必填	说明
symbol	query	string	是	交易对
响应 data： FundRateVO

字段	类型	说明
symbol	string	交易对
fundingRate	string	当前资金费率
nextCollectionTime	long	下次收取时间戳（毫秒）
GET /v2/public/q/funding-rate-record — 获取资金费率历史记录
请求参数：

参数	位置	类型	必填	说明
symbol	query	string	是	交易对
id	query	long	否	游标 ID
direction	query	string	否	PREV / NEXT，默认 NEXT
limit	query	int	否	每页条数，默认 10
响应 data： 游标分页，items 为 FundRateRecordVO 数组

字段	类型	说明
id	string	记录 ID
symbol	string	交易对
fundingRate	string	资金费率
createdTime	long	时间戳（毫秒）
collectionInternal	long	收取时间间隔（秒）
4.4 合约信息
GET /v2/public/contract/risk-balance — 获取风险基金余额记录
请求参数：

参数	位置	类型	必填	说明
symbol	query	string	是	交易对
id	query	long	否	游标 ID
direction	query	string	否	PREV / NEXT，默认 NEXT
limit	query	int	否	每页条数，默认 10
响应 data： 游标分页，items 为 RiskBalanceVO 数组

字段	类型	说明
id	string	记录 ID
coin	string	币种
amount	string	风险基金余额
createdTime	long	时间戳（毫秒）
GET /v2/public/contract/open-interest — 获取交易对持仓头寸
请求参数：

参数	位置	类型	必填	说明
symbol	query	string	是	交易对
响应 data： OpenInterestVO

字段	类型	说明
symbol	string	交易对
openInterest	string	持仓头寸（张）
openInterestUsd	string	持仓头寸价值（USD）
time	long	时间戳（毫秒）
4.5 杠杆分层
GET /v2/public/leverage/bracket/list — 查询所有交易对杠杆分层
请求参数： 无

响应 data： SymbolBracketVO 数组

字段	类型	说明
symbol	string	交易对
leverageBrackets	array	分层列表，见 LeverageBracketVO
LeverageBracketVO 字段定义：

字段	类型	说明
symbol	string	交易对
bracket	int	档位序号
maxNominalValue	string	该层最大名义价值
maintMarginRate	string	维持保证金率
startMarginRate	string	起始保证金率
maxStartMarginRate	string	最大起始保证金率
maxLeverage	string	最大杠杆倍数
minLeverage	string	最小杠杆倍数
GET /v2/public/leverage/bracket/detail — 查询单个交易对杠杆分层
请求参数：

参数	位置	类型	必填	说明
symbol	query	string	是	交易对
响应 data： SymbolBracketVO，字段定义同上。

5. 私有接口 (V2)
私有接口需要认证，用于合约交易、资金管理和持仓操作。

Base URL:

测试环境: https://futures-openapi.1bullex.com
生产环境: https://futures-openapi.onebullex.com
认证方式: 见 3.3 V2 认证方式 和 3.4 V2 签名算法

所有私有接口请求头必须携带：X-API-KEY、X-Signature、X-Nonce、X-Timestamp

5.1 WebSocket 订阅
GET /v2/user/listen-key — 获取 ListenKey
请求参数： 无

响应示例：

{
  "returnCode": 0,
  "msgInfo": "success",
  "data": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
}
Copy to clipboardErrorCopied
字段	类型	说明
data	string	ListenKey，用于建立 WebSocket 私有频道连接
5.2 资金账户
GET /v2/balance/list — 获取用户全部资金
请求参数： 无

响应 data： BalanceVO 数组

字段	类型	说明
coin	string	币种
walletBalance	string	钱包余额
openOrderMarginFrozen	string	订单冻结保证金
isolatedMargin	string	逐仓保证金冻结
crossedMargin	string	全仓起始保证金
availableBalance	string	可用余额
bonus	string	体验金余额
可用余额 = 钱包余额 - 逐仓保证金 - 全仓保证金 - 订单冻结保证金

GET /v2/balance/detail — 获取单币种资金
请求参数：

参数	位置	类型	必填	说明
coin	query	string	是	币种，如 USDT
响应 data： BalanceVO，字段定义同上。

GET /v2/balance/bills — 获取账务流水
请求参数：

参数	位置	类型	必填	说明
coin	query	string	否	币种筛选
type	query	string	否	流水类型筛选
startTime	query	long	否	开始时间戳（毫秒）
endTime	query	long	否	结束时间戳（毫秒）
page	query	int	否	页码，默认 1
size	query	int	否	每页条数
响应 data： 分页结果，items 为 BalanceBillVO 数组

字段	类型	说明
id	string	流水 ID
coin	string	币种
symbol	string	交易对（若有）
type	string	流水类型：EXCHANGE（划转）/ CLOSE_POSITION（平仓盈亏）/ TAKE_OVER（仓位接管）/ QIANG_PING_MANAGER（强平管理费）/ FUND（资金费用）/ FEE（手续费）/ ADL（自动减仓）/ MERGE（仓位合并）
amount	string	变动数量
side	string	变动方向：ADD（划入）/ SUB（转出）
afterAmount	string	变动后余额
createdTime	long	时间戳（毫秒）
GET /v2/balance/funding — 获取资金账户(USDT)余额
请求参数： 无

响应 data： FundingAccountVO

字段	类型	说明
balance	string	余额
frozen	string	冻结
available	string	可用
POST /v2/balance/transfer — 资金划转
请求体（JSON）：

字段	类型	必填	说明
billNo	string	是	幂等单号，每次请求唯一
from	string	是	转出账户类型：FUNDING（资金账户）/ CONTRACT（合约账户）
to	string	是	转入账户类型：FUNDING（资金账户）/ CONTRACT（合约账户）
amount	decimal	是	划转金额，必须大于 0
remark	string	否	备注
from 和 to 不能相同。

响应 data： 划转记录编号（long，以字符串形式返回）

GET /v2/bot/redeeming-info — 获取赎回队列汇总
请求参数： 无

响应 data： BotRedeemingInfo

字段	类型	说明
estimatedAmount	string	当前排队赎回金额
pendingCount	long	当前排队赎回条数
5.3 订单管理
POST /v2/order/create — 下单
注意：在通过本接口完成下单，获取到订单ID后，需要通过 根据ID查询订单详情 接口获取到订单详情信息来确定订单是否执行成功，因我平台为异步系统，特殊情况下存在订单信息获取延迟的情况，遇到这种情况“不能”简单认为下单失败，应多次进行订单详情查询以确认订单执行状态，如问题频繁或一直存在，请及时与我平台客服联系。

请求体（JSON）：

字段	类型	必填	说明
symbol	string	是	交易对
orderType	string	是	订单类型：LIMIT（限价）/ MARKET（市价）
orderSide	string	是	买卖方向：BUY / SELL
positionSide	string	是	仓位方向：LONG / SHORT / BOTH
origQty	decimal	是	委托数量（张），必须大于 0
price	decimal	否	委托价格，限价单必填
timeInForce	string	否	有效方式：GTC / IOC / FOK / GTX
reduceOnly	boolean	否	是否只减仓，默认 false
clientOrderId	string	否	自定义订单 ID，长度 1~32，支持字母数字及 _.-
positionId	long	否	平仓时需传对应持仓 ID
leverage	int	否	杠杆倍数
triggerProfitPrice	decimal	否	止盈触发价
triggerStopPrice	decimal	否	止损触发价
profitOrderType	string	否	止盈订单类型：MARKET / LIMIT
stopOrderType	string	否	止损订单类型：MARKET / LIMIT
profitOrderPrice	decimal	否	止盈委托价（profitOrderType=LIMIT 时必填）
stopOrderPrice	decimal	否	止损委托价（stopOrderType=LIMIT 时必填）
marketOrderLevel	int	否	市价最优档：1（对手价）/ 5 / 10 / 15
响应 data： 订单 ID（long，以字符串形式返回）

POST /v2/order/create-batch — 批量下单
请求体（JSON）：

字段	类型	必填	说明
list	string	是	订单数组的 JSON 字符串，每个元素字段同下单接口
响应 data： boolean，true 表示成功

GET /v2/order/listUnfinished — 查询当前未完成订单（单交易对）
请求参数：

参数	位置	类型	必填	说明
symbol	query	string	是	交易对
direction	query	string	是	方向：BUY / SELL
响应 data： OrderVO 数组，字段定义见下方。

GET /v2/order/all/listUnfinished — 查询当前未完成订单（多交易对）
请求参数：

参数	位置	类型	必填	说明
list	query	string	是	交易对列表，逗号分隔，如 btc_usdt,eth_usdt
响应 data： OrderVO 数组，字段定义见下方。

GET /v2/order/list — 查询订单列表（分页）
请求参数：

参数	位置	类型	必填	说明
symbol	query	string	否	交易对
status	query	string	否	订单状态筛选
page	query	int	否	页码，默认 1
size	query	int	否	每页条数
响应 data： 分页结果，items 为 OrderVO 数组，字段定义见下方。

POST /v2/order/list-by-ids — 根据 ID 列表查询订单
请求体（JSON）：

字段	类型	必填	说明
ids	array<long>	是	订单 ID 列表
响应 data： OrderVO 数组，字段定义见下方。

GET /v2/order/list-history — 查询历史订单
请求参数：

参数	位置	类型	必填	说明
symbol	query	string	否	交易对
startTime	query	long	否	开始时间戳（毫秒）
endTime	query	long	否	结束时间戳（毫秒）
id	query	long	否	游标 ID
direction	query	string	否	PREV / NEXT，默认 NEXT
limit	query	int	否	每页条数
forceClose	query	boolean	否	是否查询强平订单，默认 false
响应 data： 游标分页，items 为 OrderVO 数组，字段定义见下方。

GET /v2/order/detail — 根据 ID 查询订单详情
请求参数：

参数	位置	类型	必填	说明
orderId	query	long	是	订单 ID
响应 data： OrderVO，字段定义见下方。

OrderVO 字段定义：

字段	类型	说明
orderId	string	订单 ID
positionId	long	仓位 ID
clientOrderId	string	自定义订单 ID
symbol	string	交易对
orderType	string	订单类型：LIMIT / MARKET
orderSide	string	买卖方向：BUY / SELL
positionSide	string	仓位方向：LONG / SHORT
timeInForce	string	有效方式
closePosition	boolean	是否条件全平仓
price	string	委托价格
origQty	string	委托数量（张）
avgPrice	string	成交均价
executedQty	string	已成交数量（张）
marginFrozen	string	占用保证金
triggerProfitPrice	string	止盈触发价
triggerStopPrice	string	止损触发价
sourceId	long	条件触发 ID
forceClose	boolean	是否强平订单
closeProfit	string	平仓盈亏
state	string	订单状态：NEW（未成交）/ PARTIALLY_FILLED（部分成交）/ PARTIALLY_CANCELED（部分撤销）/ FILLED（全部成交）/ CANCELED（已撤销）/ REJECTED（下单失败）/ EXPIRED（已过期）
createdTime	long	创建时间戳（毫秒）
POST /v2/order/cancel — 撤销订单
请求体（JSON）：

字段	类型	必填	说明
orderId	long	是	订单 ID
响应 data： 撤单结果

POST /v2/order/cancel-batch — 批量撤单
请求体（JSON）：

字段	类型	必填	说明
orderIds	string	是	订单 ID 数组的 JSON 字符串，如 "[123456,789012]"
响应 data： boolean，true 表示成功

POST /v2/order/cancel-all — 撤销所有订单
请求体（JSON）：

字段	类型	必填	说明
symbol	string	否	交易对，不传则撤销所有交易对订单
响应 data： boolean，true 表示成功

GET /v2/order/trade-list — 查询成交明细
请求参数：

参数	位置	类型	必填	说明
orderId	query	long	否	订单 ID
symbol	query	string	否	交易对
startTime	query	long	否	开始时间戳（毫秒）
endTime	query	long	否	结束时间戳（毫秒）
page	query	int	否	页码，默认 1，最小 1
size	query	int	否	每页条数，默认 10，最大 100
响应 data： 分页结果，items 为 OrderTradeVO 数组

字段	类型	说明
orderId	string	订单 ID
execId	string	成交 ID
symbol	string	交易对
quantity	string	成交数量
price	string	成交价格
fee	string	手续费
feeCoin	string	手续费币种
X-Timestamp	long	成交时间戳（毫秒）
5.4 计划委托与止盈止损
POST /v2/entrust/create-plan — 创建计划委托
请求体（JSON）：

字段	类型	必填	说明
symbol	string	是	交易对
orderSide	string	是	买卖方向：BUY / SELL
positionSide	string	是	仓位方向：LONG / SHORT
entrustType	string	是	委托类型：STOP（限价）/ STOP_MARKET（市价）
timeInForce	string	是	有效方式：GTC / IOC / FOK / GTX
triggerPriceType	string	是	触发价格类型：MARK_PRICE（标记价格）/ LATEST_PRICE（最新价格）
origQty	decimal	是	委托数量（张），必须大于 0
stopPrice	decimal	否	触发价格
price	decimal	否	委托价格（限价单必填）
positionId	string	否	仓位 ID
marketOrderLevel	int	否	市价最优档：1（对手价）/ 5 / 10 / 15
expireTime	long	否	过期时间戳（毫秒）
响应 data： 委托 ID

POST /v2/entrust/create-profit — 创建止盈止损
请求体（JSON）：

字段	类型	必填	说明
symbol	string	是	交易对
origQty	decimal	是	委托数量（张），0 表示按仓位全量触发
positionSide	string	否	仓位方向：LONG / SHORT / BOTH
orderSide	string	否	买卖方向：BUY / SELL
triggerPriceType	string	否	触发价格类型：MARK_PRICE / LATEST_PRICE
triggerProfitPrice	decimal	否	止盈触发价
triggerStopPrice	decimal	否	止损触发价
profitOrderType	string	否	止盈订单类型：MARKET / LIMIT
stopOrderType	string	否	止损订单类型：MARKET / LIMIT
profitOrderPrice	decimal	否	止盈委托价（profitOrderType=LIMIT 时必填）
stopOrderPrice	decimal	否	止损委托价（stopOrderType=LIMIT 时必填）
positionId	string	否	仓位 ID
reduceOnly	boolean	否	是否只减仓，默认 true
profitFlag	int	否	1（全部止盈止损）/ 2（部分止盈止损）
expireTime	long	否	过期时间戳（毫秒）
响应 data： 止盈止损 ID

GET /v2/entrust/plan-list — 查询当前计划委托
请求参数：

参数	位置	类型	必填	说明
symbol	query	string	否	交易对
page	query	int	否	页码
size	query	int	否	每页条数
响应 data： 分页结果，items 为 PlanEntrustVO 数组，字段定义见下方。

GET /v2/entrust/plan-list-history — 查询历史计划委托
请求参数：

参数	位置	类型	必填	说明
symbol	query	string	否	交易对
startTime	query	long	否	开始时间戳（毫秒）
endTime	query	long	否	结束时间戳（毫秒）
id	query	long	否	游标 ID
direction	query	string	否	PREV / NEXT，默认 NEXT
limit	query	int	否	每页条数
响应 data： 游标分页，items 为 PlanEntrustVO 数组，字段定义见下方。

GET /v2/entrust/plan-detail — 根据 ID 查询计划委托
请求参数：

参数	位置	类型	必填	说明
entrustId	query	long	是	计划委托 ID
响应 data： PlanEntrustVO，字段定义见下方。

PlanEntrustVO 字段定义：

字段	类型	说明
entrustId	string	委托 ID
symbol	string	交易对
entrustType	string	委托类型：STOP（限价）/ STOP_MARKET（市价）
orderSide	string	买卖方向：BUY / SELL
positionSide	string	仓位方向：LONG / SHORT
timeInForce	string	有效方式
closePosition	boolean	是否触发全平
price	string	委托价格
origQty	string	委托数量（张）
stopPrice	string	触发价格
triggerPriceType	string	触发价格类型
isOrdinary	boolean	是否普通计划单
state	string	状态：NOT_TRIGGERED（未触发）/ TRIGGERING（触发中）/ TRIGGERED（已触发）/ USER_REVOCATION（用户撤销）/ PLATFORM_REVOCATION（平台撤销）/ EXPIRED（已过期）
marketOrderLevel	int	市价最优档
createdTime	long	创建时间戳（毫秒）
GET /v2/entrust/profit-list — 查询当前止盈止损
请求参数：

参数	位置	类型	必填	说明
symbol	query	string	否	交易对
page	query	int	否	页码
size	query	int	否	每页条数
响应 data： 分页结果，items 为 ProfitEntrustVO 数组，字段定义见下方。

GET /v2/entrust/profit-detail — 根据 ID 查询止盈止损
请求参数：

参数	位置	类型	必填	说明
profitId	query	long	是	止盈止损 ID
响应 data： ProfitEntrustVO，字段定义见下方。

ProfitEntrustVO 字段定义：

字段	类型	说明
profitId	string	委托 ID
symbol	string	交易对
positionSide	string	仓位方向：LONG / SHORT
origQty	string	委托数量（张）
triggerPriceType	string	触发价格类型：MARK_PRICE / LATEST_PRICE
triggerProfitPrice	string	止盈触发价
triggerStopPrice	string	止损触发价
entryPrice	string	开仓均价
positionSize	string	持仓数量（张）
isolatedMargin	string	逐仓保证金
executedQty	string	实际成交数量
state	string	状态：NOT_TRIGGERED（未触发）/ TRIGGERING（触发中）/ TRIGGERED（已触发）/ USER_REVOCATION（用户撤销）/ PLATFORM_REVOCATION（平台撤销）/ EXPIRED（已过期）
createdTime	long	创建时间戳（毫秒）
POST /v2/entrust/cancel-plan — 撤销计划委托
请求体（JSON）：

字段	类型	必填	说明
entrustId	long	是	计划委托 ID
响应 data： 撤销结果

POST /v2/entrust/cancel-profit-stop — 撤销止盈止损
请求体（JSON）：

字段	类型	必填	说明
profitId	long	是	止盈止损 ID
响应 data： 撤销结果

POST /v2/entrust/update-profit-stop — 修改止盈止损
请求体（JSON）：

字段	类型	必填	说明
profitId	long	是	止盈止损 ID
triggerProfitPrice	decimal	否	新止盈触发价
triggerStopPrice	decimal	否	新止损触发价
profitOrderPrice	decimal	否	新止盈委托价
stopOrderPrice	decimal	否	新止损委托价
响应 data： 修改结果

POST /v2/entrust/cancel-all-plan — 撤销所有计划委托
请求体（JSON）：

字段	类型	必填	说明
symbol	string	否	交易对，不传则撤销所有交易对计划委托
响应 data： boolean，true 表示成功

POST /v2/entrust/cancel-all-profit-stop — 撤销所有止盈止损
请求体（JSON）：

字段	类型	必填	说明
symbol	string	否	交易对，不传则撤销所有交易对止盈止损
响应 data： boolean，true 表示成功

5.5 全部委托
GET /v2/order-entrust/list — 查询全部委托
请求参数：

参数	位置	类型	必填	说明
type	query	string	否	委托类型：ORDER（限价/市价委托，默认）/ ENTRUST（计划委托）
symbol	query	string	否	交易对
page	query	int	否	页码
size	query	int	否	每页条数
响应 data： 分页结果，items 为 OrderEntrustVO 数组

OrderEntrustVO 字段定义：

字段	类型	说明
id	string	委托 ID（订单 ID 或计划委托 ID）
type	string	类型：ORDER / ENTRUST
symbol	string	交易对
orderType	string	订单类型
orderSide	string	买卖方向：BUY / SELL
positionSide	string	仓位方向：LONG / SHORT
timeInForce	string	有效方式
closePosition	boolean	是否条件全平仓
price	string	委托价格
origQty	string	委托数量（张）
avgPrice	string	成交均价
executedQty	string	已成交数量（张）
marginFrozen	string	占用保证金
triggerProfitPrice	string	止盈触发价
triggerStopPrice	string	止损触发价
leverage	int	杠杆倍数
entrustOrderId	long	条件触发 ID
closeProfit	string	平仓盈亏
state	string	状态：NEW / PARTIALLY_FILLED / FILLED / CANCELED / REJECTED / EXPIRED
createdTime	long	创建时间戳（毫秒）
entrustType	string	计划委托类型（type=ENTRUST 时有值）
stopPrice	string	触发价格（type=ENTRUST 时有值）
triggerPriceType	string	触发价格类型（type=ENTRUST 时有值）
isOrdinary	boolean	是否普通计划单（type=ENTRUST 时有值）
marketOrderLevel	int	市价最优档
forceClose	boolean	是否强平
POST /v2/order-entrust/cancel — 撤销委托
请求体（JSON）：

字段	类型	必填	说明
type	string	是	委托类型：ORDER / ENTRUST
id	long	是	委托 ID
响应 data： 撤销结果

POST /v2/order-entrust/cancel-all — 撤销所有委托
请求体（JSON）：

字段	类型	必填	说明
symbol	string	否	交易对，不传则撤销所有交易对的订单和计划委托
响应 data： boolean，true 表示成功

5.6 持仓管理
GET /v2/position/list — 获取持仓信息
请求参数：

参数	位置	类型	必填	说明
symbol	query	string	否	交易对，不传则返回所有持仓
响应 data： PositionVO 数组

字段	类型	说明
symbol	string	交易对
positionId	string	持仓 ID
positionType	string	仓位类型：CROSSED（全仓）/ ISOLATED（逐仓）
positionSide	string	持仓方向：LONG / SHORT
positionSize	string	持仓数量（张）
closeOrderSize	string	平仓挂单数量（张）
availableCloseSize	string	可平仓数量（张）
entryPrice	string	开仓均价
isolatedMargin	string	逐仓保证金
openOrderMarginFrozen	string	开仓订单保证金占用
realizedProfit	string	已实现盈亏
autoMargin	boolean	是否自动追加保证金
leverage	int	杠杆倍数
contractSize	string	合约乘数
POST /v2/position/adjust-leverage — 调整杠杆倍数
请求体（JSON）：

字段	类型	必填	说明
symbol	string	是	交易对
leverage	int	是	目标杠杆倍数，最小 1
响应 data： 调整结果

POST /v2/position/margin — 修改逐仓保证金
请求体（JSON）：

字段	类型	必填	说明
symbol	string	是	交易对
positionSide	string	是	持仓方向：LONG / SHORT
positionId	long	是	持仓 ID
margin	decimal	是	调整数量，必须大于 0
type	string	是	调整方向：ADD（增加）/ SUB（减少）
响应 data： 调整结果

POST /v2/position/auto-margin — 修改自动追加保证金
请求体（JSON）：

字段	类型	必填	说明
symbol	string	是	交易对
positionSide	string	是	持仓方向：LONG / SHORT
autoMargin	boolean	是	是否开启自动追加保证金
响应 data： 修改结果

POST /v2/position/close-all — 一键平仓
请求体（JSON）：

字段	类型	必填	说明
symbol	string	否	交易对，不传则平所有持仓
响应 data： 平仓结果

POST /v2/position/merge — 合并仓位
请求体（JSON）：

字段	类型	必填	说明
symbol	string	是	交易对
响应 data： 合并结果

POST /v2/position/change-type — 修改持仓模式
请求体（JSON）：

字段	类型	必填	说明
symbol	string	是	交易对
positionType	string	是	目标仓位类型：CROSSED（全仓）/ ISOLATED（逐仓）
positionModel	string	是	仓位类型: AGGREGATION(合仓) / DISAGGREGATION(分仓)
响应 data： 修改结果

GET /v2/position/confs — 获取持仓配置信息
请求参数：

字段	类型	必填	说明
symbol	string	是	交易对
响应 data： PositionConfVO 数组

字段	类型	说明
symbol	string	交易对
positionType	string	仓位类型：CROSSED（全仓）/ ISOLATED（逐仓）
positionSide	string	持仓方向：LONG / SHORT / BOTH
positionModel	string	持仓方式:DISAGGREGATION（双向持仓）/ AGGREGATION（单向持仓）
autoMargin	bool	是否自动追加保证金
leverage	integer	杠杆倍数
GET /v2/position/history/detail — 获取历史仓位详情
请求参数：

字段	类型	必填	说明
symbol	string	是	交易对
positionId	long	是	仓位ID
响应 data： PositionLogVO

字段	类型	说明
id	long	持仓记录 ID
symbolId	int	交易对 ID
symbol	string	交易对
userId	long	用户 ID
accountId	long	账户 ID
leverage	int	杠杆倍数
positionType	string	仓位类型：CROSSED（全仓）/ ISOLATED（逐仓）
positionSide	string	持仓方向：LONG / SHORT
positionModel	string	持仓方式：AGGREGATION（单向持仓）/ DISAGGREGATION（双向持仓）
entryPrice	string	开仓均价
closePrice	string	平仓均价
maxPositionSize	string	最大持仓数量（张）
closeOrderSize	string	平仓数量（张）
realizedProfit	string	已实现盈亏
tradeFee	string	交易手续费
profitRate	string	收益率
takeOver	boolean	是否接管（强平接管）
createdTime	long	创建时间戳（毫秒）
updatedTime	long	更新时间戳（毫秒）
finished	boolean	是否已完结（已全部平仓）
unsettledProfit	string	未结算盈亏
fundingFee	string	资金费用
liqPrice	string	强平价格，未触发强平时为 0
6. WebSocket API
6.1 概述
本文档描述了 Tiger 交易平台 Open WebSocket API 的接入协议，用于实时接收账户余额、订单状态、持仓信息、价格数据等推送。该服务面向 API 用户，所有频道均需认证后方可订阅。

6.2 连接信息
6.2.1 连接地址
测试环境: wss://openapi.1bullex.com/fstream/ws/open
生产环境: wss://openapi.onebullex.com/fstream/ws/open
Copy to clipboardErrorCopied
协议：标准 WebSocket（WSS）
支持文本消息（JSON）和二进制消息（UTF-8 编码的 JSON）
一个连接可同时订阅多个频道和多个交易对
6.2.2 连接生命周期
客户端发起 WebSocket 连接
连接建立后，发送订阅请求（携带签名认证）
服务端验证签名，返回订阅结果
服务端通过二进制消息推送实时数据
服务端每 15 秒发送心跳 ping，客户端需回复 pong
超过 60 秒未收到 pong 响应，服务端主动断开连接
6.3 认证机制
6.3.1 API 密钥
需要先获取 API 密钥对：AccessKey 和 SecretKey
AccessKey：用于标识 API 用户，填入请求的 key 字段
SecretKey：用于签名验证，需安全存储
API 需开启合约交易权限（contractTrade = true）
6.3.2 签名算法
使用 HMAC-SHA256 对订阅参数进行签名。

签名步骤：

将 args 数组中每个参数对象，按键名字母序排序，将键值直接拼接（无分隔符）
示例：{"symbol": "btc_usdt"} → "symbolbtc_usdt"
将所有参数的拼接字符串放入数组：["symbolbtc_usdt"]
将数组转为 JSON 字符串：["symbolbtc_usdt"]
使用 SecretKey 对该 JSON 字符串进行 HMAC-SHA256 签名，结果为十六进制字符串
Python 示例：

import hmac, hashlib, json

def generate_signature(args, secret_key):
    ordered_strings = []
    for arg in args:
        s = ""
        for k, v in sorted(arg.items()):
            s += f"{k}{v}"
        ordered_strings.append(s)

    sign_text = json.dumps(ordered_strings, separators=(',', ':'))
    return hmac.new(
        secret_key.encode('utf-8'),
        sign_text.encode('utf-8'),
        hashlib.sha256
    ).hexdigest()
Copy to clipboardErrorCopied
6.4 请求协议格式
6.4.1 订阅请求
{
    "op": "SUBSCRIBE",
    "channel": "ORDERS",
    "key": "your_access_key",
    "signature": "calculated_signature",
    "args": [
        {"symbol": "btc_usdt"}
    ]
}
Copy to clipboardErrorCopied
6.4.2 取消订阅请求
{
    "op": "UN_SUBSCRIBE",
    "channel": "ORDERS",
    "key": "your_access_key",
    "signature": "calculated_signature",
    "args": [
        {"symbol": "btc_usdt"}
    ]
}
Copy to clipboardErrorCopied
6.4.3 请求字段说明
字段	类型	必填	说明
op	String	是	操作类型：SUBSCRIBE（订阅）/ UN_SUBSCRIBE（取消订阅）
channel	String	是	频道类型，见第五节
key	String	是	AccessKey
signature	String	是	HMAC-SHA256 签名
args	Array	是	订阅参数数组
6.5 支持的频道类型
频道	说明	订阅参数	描述
ACCOUNT	账户余额推送	{"ccy": "USDT"}	推送账户余额变化
PRICE	价格推送	{"symbol": "btc_usdt"}	推送标记价格、指数价格、最新成交
ORDERS	订单推送	{"symbol": "btc_usdt"}	推送订单状态变化
POSITIONS	持仓推送	{"symbol": "btc_usdt"}	推送持仓信息变化
QUIRE_LEVERAGE	杠杆配置推送	{"symbol": "btc_usdt"}	推送杠杆配置变化
ALLOCATION_RATIO	资金费率推送	{"symbol": "btc_usdt"}	推送资金费率信息
所有频道均需认证，不支持匿名订阅。

6.6 响应格式
6.6.1 订阅成功响应
{
    "event": "SUBSCRIBE",
    "args": [{"symbol": "btc_usdt"}],
    "code": 200,
    "msg": "success"
}
Copy to clipboardErrorCopied
6.6.2 取消订阅成功响应
{
    "event": "UN_SUBSCRIBE",
    "args": [{"symbol": "btc_usdt"}],
    "code": 200,
    "msg": "success"
}
Copy to clipboardErrorCopied
6.6.3 错误响应
{
    "event": "ERROR",
    "args": [{"symbol": "btc_usdt"}],
    "code": 400,
    "msg": "signature error"
}
Copy to clipboardErrorCopied
6.6.4 响应字段说明
字段	类型	说明
event	String	事件类型：SUBSCRIBE / UN_SUBSCRIBE / ERROR
args	Array	请求参数回显
code	Integer	状态码：200 成功，400 失败
msg	String	响应消息
6.7 推送数据格式
推送数据使用 二进制消息 传输，内容为 UTF-8 编码的 JSON 字符串。客户端收到二进制消息后，直接解码为 UTF-8 文本，再解析 JSON。

ws.onmessage = function(event) {
    if (typeof event.data === 'string') {
        // 文本消息：订阅响应或心跳
        if (event.data === 'ping') {
            ws.send('pong');
            return;
        }
        const data = JSON.parse(event.data);
    } else {
        // 二进制消息：业务数据推送
        const text = new TextDecoder().decode(event.data);
        const data = JSON.parse(text);
    }
};
Copy to clipboardErrorCopied
6.7.1 账户余额推送 (ACCOUNT)
{
    "arg": {
        "channel": "ACCOUNT",
        "uId": 123456
    },
    "data": [{
        "ts": 1640995200000,
        "detail": {
            "ccy": "USDT",
            "ts": 1640995200000,
            "equity": "10000.00",
            "balance": "10000.00",
            "availEq": "9000.00",
            "isolateEquity": "0.00",
            "frozenEq": "1000.00",
            "initialMargin": "500.00",
            "maintMargin": "250.00",
            "orderFrozen": "1000.00",
            "crossUpl": "100.00",
            "marginRatio": "0.00",
            "isolatedUpl": "0.00"
        }
    }]
}
Copy to clipboardErrorCopied
字段说明：

字段	类型	说明
ccy	String	币种，如 USDT
ts	Long	资金最后更新时间（毫秒时间戳）
equity	String	币种总权益 = balance + crossUpl + isolatedUpl
balance	String	币种余额（钱包余额）
availEq	String	可用保证金 = balance - orderFrozen - isolatedMargin - crossedMargin
isolateEquity	String	逐仓仓位权益 = isolatedUpl + isolatedMargin
frozenEq	String	占用资金 = orderFrozen + isolatedMargin + crossedMargin
initialMargin	String	初始保证金（所有仓位汇总）
maintMargin	String	维持保证金（所有仓位汇总）
orderFrozen	String	挂单冻结数量
crossUpl	String	全仓未实现盈亏
marginRatio	String	保证金率
isolatedUpl	String	逐仓未实现盈亏
6.7.2 价格推送 (PRICE)
{
    "arg": {
        "channel": "PRICE",
        "symbol": "btc_usdt"
    },
    "data": [{
        "contractType": "PERPETUAL",
        "symbol": "btc_usdt",
        "idxPx": "50000.00",
        "markPx": "50005.00",
        "latestTrade": [{
            "tradeId": 123456789,
            "px": "50010.00",
            "sz": "0.1",
            "side": "BUY"
        }],
        "ts": 1640995200000
    }]
}
Copy to clipboardErrorCopied
字段说明：

字段	类型	说明
contractType	String	合约类型：PERPETUAL（永续合约）
symbol	String	交易对符号
idxPx	String	指数价格
markPx	String	标记价格
latestTrade	Array	最新成交信息数组
ts	Long	数据更新时间（毫秒时间戳）
latestTrade 字段：

字段	类型	说明
tradeId	Long	成交 ID
px	String	成交价格
sz	String	成交数量（张数）
side	String	成交方向：BUY / SELL
6.7.3 订单推送 (ORDERS)
{
    "arg": {
        "channel": "ORDERS",
        "uId": 123456,
        "contractType": "PERPETUAL",
        "symbol": "btc_usdt"
    },
    "orderPushType": "SINGLE",
    "data": [{
        "contractType": "PERPETUAL",
        "symbol": "btc_usdt",
        "ccy": "USDT",
        "orderId": 123456789,
        "clientOrderId": "myorder001",
        "px": "50000.00",
        "sz": "0.1",
        "notional": "5000.00",
        "fillNotional": "2500.00",
        "orderType": "LIMIT",
        "orderSide": "BUY",
        "positionSide": "LONG",
        "positionType": "CROSSED",
        "tgtCcy": "0.00",
        "fillMarkPx": "50005.00",
        "state": "NEW",
        "lever": 10,
        "sourceType": "DEFAULT",
        "fillPx": "50010.00",
        "tradeId": 987654321,
        "fillSz": "0.05",
        "fillPnl": "0.50",
        "fillTime": 1640995200000,
        "fillFee": "0.25",
        "fillFeeCcy": "0.000005",
        "isMaker": false,
        "accFillSz": "0.05",
        "avgPx": "50010.00",
        "fee": "0.25",
        "feeCcy": "0.000005",
        "pnl": "0.50",
        "lastPx": "50010.00",
        "algoClOrdId": null,
        "isTpLimit": false,
        "uTime": 1640995200000,
        "cTime": 1640995100000
    }]
}
Copy to clipboardErrorCopied
字段说明：

字段	类型	说明
contractType	String	合约类型：PERPETUAL（永续合约）
symbol	String	交易对符号
ccy	String	保证金币种
orderId	Long	系统订单 ID
clientOrderId	String	客户端自定义订单 ID
px	String	委托价格
sz	String	原始委托数量（张数）
notional	String	预估名义价值
fillNotional	String	已成交价值
orderType	String	订单类型：LIMIT（限价）/ MARKET（市价）
orderSide	String	订单方向：BUY / SELL
positionSide	String	仓位方向：LONG / SHORT / BOTH
positionType	String	仓位类型：CROSSED（全仓）/ ISOLATED（逐仓）
tgtCcy	String	市价单委托数量单位
fillMarkPx	String	成交时标记价格
state	String	订单状态（见下方说明）
lever	Integer	杠杆倍数
sourceType	String	订单来源：DEFAULT（普通）/ ENTRUST（计划委托）/ PROFIT（止盈止损）/ REVERSE（反手单）
fillPx	String	最新成交价格
tradeId	Long	最新成交 ID
fillSz	String	最新成交数量（张数）
fillPnl	String	成交收益（平仓单）
fillTime	Long	成交时间（毫秒时间戳）
fillFee	String	最新一笔成交手续费金额
fillFeeCcy	String	最新一笔成交手续费（币数量）
isMaker	Boolean	是否为 Maker
accFillSz	String	累计成交数量（张数）
avgPx	String	成交均价
fee	String	订单累计手续费金额
feeCcy	String	订单累计手续费（币数量）
pnl	String	收益（平仓订单）
lastPx	String	最新成交价
algoClOrdId	Long	触发的止盈止损 ID
isTpLimit	Boolean	是否限价止盈
uTime	Long	订单更新时间（毫秒时间戳）
cTime	Long	订单创建时间（毫秒时间戳）
orderPushType 说明：

值	说明
SINGLE	单笔订单更新推送
ALL	全量订单快照推送（首次订阅时）
订单状态流转：

NEW → PARTIALLY_FILLED → FILLED（成交完成）
NEW → CANCELED（用户撤单）
NEW → REJECTED（订单被拒绝）
NEW → EXPIRED（订单过期）
Copy to clipboardErrorCopied
6.7.4 持仓推送 (POSITIONS)
{
    "arg": {
        "channel": "POSITIONS",
        "symbol": "btc_usdt",
        "uId": 123456
    },
    "data": [{
        "positionType": "CROSSED",
        "positionModel": "LONG_SHORT",
        "positionId": 123456789,
        "positionSide": "LONG",
        "pos": "0.1",
        "availPos": "0.08",
        "upl": "50.00",
        "uplRatio": "0.1000",
        "lever": 10,
        "liqPx": "45000.00",
        "markPx": "50005.00",
        "imr": "0.00",
        "margin": "500.00",
        "mgnRatio": "0.2500",
        "mmr": "250.00",
        "adl": "0",
        "ccy": "USDT",
        "realizedPnl": "25.50",
        "cTime": 1640995000000,
        "uTime": 1640995200000,
        "pTime": 1640995201000
    }]
}
Copy to clipboardErrorCopied
字段说明：

字段	类型	说明
positionType	String	保证金模式：CROSSED（全仓）/ ISOLATED（逐仓）
positionModel	String	持仓模式：LONG_SHORT（双向持仓）/ AGGREGATION（单向持仓）
positionId	Long	仓位 ID
positionSide	String	仓位方向：LONG / SHORT / BOTH
pos	String	仓位大小（张数）
availPos	String	可用仓位 = pos - 平仓挂单数量
upl	String	未实现收益（基于标记价格）
uplRatio	String	未实现收益率
lever	Integer	杠杆倍数
liqPx	String	预估强平价格（-- 表示无强平风险）
markPx	String	最新标记价格
imr	String	初始保证金（全仓时显示）
margin	String	保证金（逐仓时显示）
mgnRatio	String	保证金率
mmr	String	维持保证金
adl	String	ADL 排队指标（0-4）
ccy	String	保证金币种
realizedPnl	String	已实现收益
cTime	Long	持仓创建时间（毫秒时间戳）
uTime	Long	持仓更新时间（毫秒时间戳）
pTime	Long	持仓推送时间（毫秒时间戳）
6.7.5 杠杆配置推送 (QUIRE_LEVERAGE)
{
    "arg": {
        "channel": "QUIRE_LEVERAGE",
        "symbol": "btc_usdt"
    },
    "data": {
        "contractType": "PERPETUAL",
        "symbol": "btc_usdt",
        "details": [{
            "positionType": "CROSSED",
            "positionSide": "LONG",
            "lever": 10
        }],
        "ts": 1640995200000
    }
}
Copy to clipboardErrorCopied
字段说明：

字段	类型	说明
contractType	String	合约类型：PERPETUAL
symbol	String	交易对符号
details	Array	杠杆配置详情数组
ts	Long	配置更新时间（毫秒时间戳）
details 字段：

字段	类型	说明
positionType	String	保证金模式：CROSSED / ISOLATED
positionSide	String	仓位方向：LONG / SHORT / BOTH
lever	Integer	杠杆倍数
6.7.6 资金费率推送 (ALLOCATION_RATIO)
{
    "arg": {
        "channel": "ALLOCATION_RATIO",
        "symbol": "btc_usdt"
    },
    "data": {
        "symbol": "btc_usdt",
        "alloRate": "0.0001",
        "ts": 1640995200000
    }
}
Copy to clipboardErrorCopied
字段说明：

字段	类型	说明
symbol	String	交易对符号
alloRate	String	资金费率（正数：多头付空头；负数：空头付多头）
ts	Long	费率更新时间（毫秒时间戳）
6.8 心跳机制
服务端每 15 秒 发送 ping 文本消息
客户端收到 ping 后需回复 pong
超过 60 秒 未收到 pong，服务端主动断开连接
建议客户端实现自动重连和重新订阅逻辑
6.9 错误处理
6.9.1 常见错误
错误码	错误信息	说明
400	signature error	签名验证失败
400	no market account	账户不存在或未开通合约交易
400	Invalid parameter	请求参数格式错误
6.9.2 其他异常情况
key 字段为空：连接被静默关闭
API 已删除或已禁用：签名验证失败
未开启合约交易权限：签名验证失败
JSON 解析错误：连接被关闭
6.10 数据传输说明
6.10.1 消息类型
方向	格式	说明
客户端 → 服务端	JSON 文本	订阅/取消订阅请求
服务端 → 客户端（响应）	JSON 文本	订阅确认、错误响应
服务端 → 客户端（推送）	二进制（UTF-8 JSON）	业务数据推送
服务端 → 客户端（心跳）	文本 ping	心跳检测
客户端 → 服务端（心跳）	文本 pong	心跳响应
6.10.2 数值精度
所有价格和数量字段均为字符串类型，避免浮点精度丢失
建议使用 BigDecimal 等高精度数值类型处理
6.10.3 时间戳
所有时间戳均为毫秒级 Unix 时间戳
6.11 注意事项
SecretKey 必须安全保存，不能泄露
合理处理连接断开和重连逻辑
及时响应服务端心跳，避免连接被断开
正确区分文本消息和二进制消息
避免过于频繁的订阅/取消订阅操作
一个连接可同时订阅多个频道和多个交易对
首次订阅 ORDERS 频道时，服务端会推送全量订单快照（orderPushType: "ALL"）
7. 错误码
7.1 V2 错误码
错误码	说明
sign-error	签名验证失败（缺少 Header 或签名不匹配）
system-error	系统内部错误
invalid_symbol	无效交易对
8. 常见问题
8.1 如何获取 API Key？
请联系 OneBullEx 官方团队：

邮箱: it@onebullex.com
官网: https://www.onebullex.com
8.2 签名的有效期是多久？
签名时间戳有效期为 30 秒。请确保您的服务器时间已同步。

8.3 接口限频规则是什么？
限频控制基于 API Key 和 IP 地址。具体限频规则可咨询客服。

8.4 逐仓和全仓有什么区别？
逐仓（MarginMode=1）: 每个仓位独立保证金，风险隔离
全仓（MarginMode=2）: 所有仓位共享保证金，资金效率更高
8.5 如何处理网络错误？
建议策略：

使用超时机制（建议 5-10 秒）
实现重试机制（建议重试 3 次）
记录失败请求便于人工处理
8.6 下单失败的原因有哪些？
常见原因：

余额不足
价格无效（超出允许范围）
数量无效（低于最小值或高于最大值）
交易对暂停交易
系统维护中
请查看错误码和错误信息获取具体原因。

8.7 如何查询 WebSocket 连接状态？
WebSocket 服务器会定期推送心跳消息。如果长时间未收到心跳，请重新连接。

9. 支持
团队: OneBullEx Team
官网: https://www.onebullex.com
邮箱: it@onebullex.com

如有 API 接入问题、技术支持或商务合作需求，欢迎随时联系我们。