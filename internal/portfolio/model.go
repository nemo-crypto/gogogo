package portfolio

import "time"

type BalanceSnapshot struct {
	ID                    int64
	AccountID             string
	Exchange              string
	Asset                 string
	Free                  float64
	Locked                float64
	Total                 float64
	USDValue              float64
	WalletBalance         string
	OpenOrderMarginFrozen string
	IsolatedMargin        string
	CrossedMargin         string
	AvailableBalance      string
	Bonus                 string
	RawJSON               string
	SnapshotTime          time.Time
	CreatedAt             time.Time
}

type PositionSnapshot struct {
	ID                  int64
	AccountID           string
	Exchange            string
	MarketType          string
	Symbol              string
	PositionSide        string
	PositionModel       string
	Quantity            float64
	EntryPrice          float64
	MarkPrice           float64
	LiquidationPrice    float64
	Leverage            float64
	MarginMode          string
	UnrealizedPnL       float64
	Notional            float64
	LiquidationDistance float64
	ExchangePositionID  string
	CloseOrderSize      string
	AvailableCloseSize  string
	IsolatedMargin      string
	OpenOrderMargin     string
	RealizedProfit      string
	AutoMargin          bool
	ContractSize        string
	RawJSON             string
	SnapshotTime        time.Time
	CreatedAt           time.Time
}

type PaperPositionStatus string

const (
	PaperPositionOpen   PaperPositionStatus = "open"
	PaperPositionClosed PaperPositionStatus = "closed"
)

type PaperPositionRecord struct {
	ID              int64
	AccountID       string
	StrategyID      string
	Exchange        string
	MarketType      string
	Symbol          string
	PositionSide    string
	Quantity        float64
	EntryPrice      float64
	MarkPrice       float64
	TakeProfitPrice float64
	StopLossPrice   float64
	InitialStopLoss float64
	RealizedPnL     float64
	CloseReason     string
	Status          PaperPositionStatus
	OpenedAt        time.Time
	ClosedAt        *time.Time
	UpdatedAt       time.Time
}

type MarginSnapshot struct {
	ID                int64
	AccountID         string
	Exchange          string
	MarketType        string
	Equity            float64
	MarginBalance     float64
	InitialMargin     float64
	MaintenanceMargin float64
	MarginRatio       float64
	AvailableBalance  float64
	SnapshotTime      time.Time
	CreatedAt         time.Time
}

type PositionConfig struct {
	ID            int64
	AccountID     string
	Exchange      string
	Symbol        string
	PositionType  string
	PositionSide  string
	PositionModel string
	AutoMargin    bool
	Leverage      int
	RawJSON       string
	SnapshotTime  time.Time
	CreatedAt     time.Time
}

type ContractSpec struct {
	ID                        int64
	Exchange                  string
	Symbol                    string
	ContractType              string
	UnderlyingType            string
	ContractSize              string
	TradeSwitch               bool
	State                     int
	InitLeverage              int
	InitPositionType          string
	BaseAsset                 string
	QuoteAsset                string
	BaseCoinPrecision         int
	BaseCoinDisplayPrecision  int
	QuoteCoinPrecision        int
	QuoteCoinDisplayPrecision int
	QuantityPrecision         int
	PricePrecision            int
	SupportOrderType          string
	SupportTimeInForce        string
	SupportEntrustType        string
	SupportPositionType       string
	MinPrice                  string
	MinQty                    string
	MinNotional               string
	MaxNotional               string
	MultiplierDown            string
	MultiplierUp              string
	MaxOpenOrders             int
	MaxEntrusts               int
	MakerFee                  string
	TakerFee                  string
	LiquidationFee            string
	MarketTakeBound           string
	DepthPrecisionMerge       int
	LabelsJSON                string
	OnboardTime               time.Time
	EnglishName               string
	ChineseName               string
	MinStepPrice              string
	BaseCoinName              string
	QuoteCoinName             string
	TickSize                  string
	StepSize                  string
	RawJSON                   string
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

type LeverageBracket struct {
	ID                 int64
	Exchange           string
	Symbol             string
	Bracket            int
	MaxNominalValue    string
	MaintMarginRate    string
	StartMarginRate    string
	MaxStartMarginRate string
	MaxLeverage        string
	MinLeverage        string
	RawJSON            string
	CreatedAt          time.Time
}
