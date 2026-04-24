package btc

import (
	"log"
	"sync"
	"time"
)

// RiskManager manages trading risk
type RiskManager struct {
	config RiskConfig
	mu     sync.RWMutex

	// Daily tracking
	dailyPnL      float64
	dailyTrades   int
	lastResetDate string

	// Consecutive losses
	consecutiveLosses int
	consecutiveWins   int

	// Drawdown tracking
	peakBalance    float64
	currentBalance float64

	// Trade history
	tradeHistory []TradeRecord

	// Cooldown state
	lastLossTime time.Time
	inCooldown   bool
}

// RiskConfig holds risk management configuration
type RiskConfig struct {
	MaxDailyLoss       float64       // Maximum daily loss in USD
	MaxDrawdownPct     float64       // Maximum drawdown percentage
	MaxConsecutiveLoss int           // Maximum consecutive losses before cooldown
	CooldownAfterLoss  time.Duration // Cooldown period after hitting loss limit
	MaxDailyTrades     int           // Maximum number of trades per day
	MinConfidenceFloor float64       // Minimum confidence floor (never trade below this)
	RiskPerTrade       float64       // Maximum risk per trade as % of balance
	MaxPositionUSD     float64       // Maximum position size in USD
}

// DefaultRiskConfig returns default risk configuration
func DefaultRiskConfig() RiskConfig {
	return RiskConfig{
		MaxDailyLoss:       6.0,              // 50 USDC 本金先把单日最大亏损压小
		MaxDrawdownPct:     0.15,
		MaxConsecutiveLoss: 2,
		CooldownAfterLoss:  15 * time.Minute,
		MaxDailyTrades:     6,
		MinConfidenceFloor: 0.60,
		RiskPerTrade:       0.01,             // 先按 1% 风险测试
		MaxPositionUSD:     3.0,
	}
}

// TradeRecord records a trade for risk tracking
type TradeRecord struct {
	Timestamp  time.Time
	MarketID   string
	Direction  string
	Size       float64
	EntryPrice float64
	ExitPrice  float64
	PnL        float64
	Confidence float64
}

// NewRiskManager creates a new risk manager
func NewRiskManager(config RiskConfig) *RiskManager {
	return &RiskManager{
		config:         config,
		peakBalance:    1000,
		currentBalance: 1000, // Initialize to same as peak
		tradeHistory:   make([]TradeRecord, 0, 100),
		lastResetDate:  time.Now().Format("2006-01-02"),
	}
}

// InitBalance initializes the risk manager with actual USDC balance
func (rm *RiskManager) InitBalance(balance float64) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	if balance > 0 {
		rm.currentBalance = balance
		rm.peakBalance = balance
	}
}

// CanTrade checks if trading is allowed based on risk rules
func (rm *RiskManager) CanTrade(confidence float64) (bool, string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Check cooldown from consecutive losses
	if rm.inCooldown {
		if time.Since(rm.lastLossTime) < rm.config.CooldownAfterLoss {
			return false, "Cooldown after consecutive losses"
		}
		rm.inCooldown = false
	}

	// Check daily loss limit
	if rm.dailyPnL <= -rm.config.MaxDailyLoss {
		return false, "Daily loss limit reached"
	}

	// Check daily trade limit
	if rm.dailyTrades >= rm.config.MaxDailyTrades {
		return false, "Daily trade limit reached"
	}

	// Check drawdown
	if rm.peakBalance > 0 {
		drawdown := (rm.peakBalance - rm.currentBalance) / rm.peakBalance
		if drawdown >= rm.config.MaxDrawdownPct {
			return false, "Maximum drawdown reached"
		}
	}

	// Check minimum confidence
	if confidence < rm.config.MinConfidenceFloor {
		return false, "Confidence below minimum floor"
	}

	// Check consecutive losses
	if rm.consecutiveLosses >= rm.config.MaxConsecutiveLoss {
		return false, "Too many consecutive losses"
	}

	return true, ""
}

// CalculatePositionSize calculates the recommended position size using Kelly Criterion
// Kelly formula: f* = (p*b - q) / b
// where p = win probability (confidence), q = 1-p, b = odds (payout ratio)
// For Polymarket binary tokens: b = (1 - price) / price
// We use fractional Kelly (50%) for safety — halves the Kelly fraction to reduce variance
func (rm *RiskManager) CalculatePositionSize(confidence, currentPrice, balance float64) float64 {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if currentPrice <= 0 || currentPrice >= 1.0 {
		currentPrice = 0.50
	}

	// Calculate Kelly fraction
	p := confidence
	q := 1.0 - p
	b := (1.0 - currentPrice) / currentPrice // Polymarket odds

	kellyFraction := (p*b - q) / b
	if kellyFraction <= 0 {
		return 1 // 保守复测版：即使 Kelly 不支持，也不再强制放大到 5 shares
	}

	// Fractional Kelly (50%) to reduce variance
	kellyFraction *= 0.5

	effectiveRiskPerTrade := rm.config.RiskPerTrade
	maxPosition := rm.config.MaxPositionUSD

	// Cap Kelly fraction at effective risk-per-trade limit
	if kellyFraction > effectiveRiskPerTrade {
		kellyFraction = effectiveRiskPerTrade
	}

	positionUSD := balance * kellyFraction

	// Cap at max position
	if positionUSD > maxPosition {
		positionUSD = maxPosition
	}

	// Convert to shares
	shares := positionUSD / currentPrice

	// 保守复测版：不强制抬到 5 shares
	if shares < 1 {
		shares = 1
	}

	// Cap maximum shares based on USD value
	maxSharesByValue := maxPosition / currentPrice
	if shares > maxSharesByValue {
		shares = maxSharesByValue
	}

	return shares
}

// GetMaxPositionUSD returns the configured maximum position size in USD
func (rm *RiskManager) GetMaxPositionUSD() float64 {
	return rm.config.MaxPositionUSD
}

// RecordTrade records a trade for risk tracking
func (rm *RiskManager) RecordTrade(record TradeRecord) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// Reset daily counters if date changed
	today := time.Now().Format("2006-01-02")
	if today != rm.lastResetDate {
		rm.dailyPnL = 0
		rm.dailyTrades = 0
		rm.lastResetDate = today
	}

	rm.tradeHistory = append(rm.tradeHistory, record)
	rm.dailyTrades++
	rm.dailyPnL += record.PnL

	// Update balance
	rm.currentBalance += record.PnL

	// Track peak balance
	if rm.currentBalance > rm.peakBalance {
		rm.peakBalance = rm.currentBalance
	}

	// Update consecutive wins/losses
	if record.PnL > 0 {
		rm.consecutiveWins++
		rm.consecutiveLosses = 0
	} else if record.PnL < 0 {
		rm.consecutiveLosses++
		rm.consecutiveWins = 0
		rm.lastLossTime = time.Now()

		// Trigger cooldown if too many consecutive losses
		if rm.consecutiveLosses >= rm.config.MaxConsecutiveLoss {
			rm.inCooldown = true
			log.Printf("[RISK] ⚠️ %d consecutive losses, entering cooldown for %v",
				rm.consecutiveLosses, rm.config.CooldownAfterLoss)
		}
	}

	log.Printf("[RISK] Trade recorded: PnL=$%.2f, Daily PnL=$%.2f, Consecutive losses=%d",
		record.PnL, rm.dailyPnL, rm.consecutiveLosses)
}

// GetDailyStats returns daily trading statistics
func (rm *RiskManager) GetDailyStats() (trades int, pnl float64, drawdown float64) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	drawdown = 0
	if rm.peakBalance > 0 {
		drawdown = (rm.peakBalance - rm.currentBalance) / rm.peakBalance * 100
	}

	return rm.dailyTrades, rm.dailyPnL, drawdown
}

// GetRiskStatus returns current risk status
func (rm *RiskManager) GetRiskStatus() map[string]interface{} {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return map[string]interface{}{
		"daily_trades":       rm.dailyTrades,
		"daily_pnl":          rm.dailyPnL,
		"consecutive_losses": rm.consecutiveLosses,
		"consecutive_wins":   rm.consecutiveWins,
		"current_balance":    rm.currentBalance,
		"peak_balance":       rm.peakBalance,
		"in_cooldown":        rm.inCooldown,
		"total_trades":       len(rm.tradeHistory),
	}
}
