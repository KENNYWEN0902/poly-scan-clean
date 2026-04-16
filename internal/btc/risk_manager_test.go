package btc

import (
	"testing"
	"time"
)

func TestNewRiskManager(t *testing.T) {
	rm := NewRiskManager(DefaultRiskConfig())
	if rm.peakBalance != 1000 {
		t.Errorf("expected peakBalance=1000, got %f", rm.peakBalance)
	}
	if rm.currentBalance != 1000 {
		t.Errorf("expected currentBalance=1000, got %f", rm.currentBalance)
	}
}

func TestInitBalance(t *testing.T) {
	rm := NewRiskManager(DefaultRiskConfig())
	rm.InitBalance(500.0)
	if rm.currentBalance != 500 {
		t.Errorf("expected currentBalance=500, got %f", rm.currentBalance)
	}
	if rm.peakBalance != 500 {
		t.Errorf("expected peakBalance=500, got %f", rm.peakBalance)
	}

	// Zero balance should not update
	rm.InitBalance(0)
	if rm.currentBalance != 500 {
		t.Errorf("expected currentBalance unchanged at 500, got %f", rm.currentBalance)
	}
}

func TestCanTrade_BasicAllowed(t *testing.T) {
	rm := NewRiskManager(DefaultRiskConfig())
	ok, reason := rm.CanTrade(0.60)
	if !ok {
		t.Errorf("expected trade allowed, got blocked: %s", reason)
	}
}

func TestCanTrade_BelowMinConfidence(t *testing.T) {
	rm := NewRiskManager(DefaultRiskConfig())
	ok, _ := rm.CanTrade(0.50) // below 0.55 floor
	if ok {
		t.Error("expected trade blocked for confidence below floor")
	}
}

func TestCanTrade_DailyLossLimit(t *testing.T) {
	config := DefaultRiskConfig()
	config.MaxDailyLoss = 10.0
	rm := NewRiskManager(config)

	// Record a big loss
	rm.RecordTrade(TradeRecord{PnL: -15.0, Timestamp: time.Now()})

	ok, reason := rm.CanTrade(0.70)
	if ok {
		t.Errorf("expected trade blocked after exceeding daily loss limit, reason: %s", reason)
	}
}

func TestCanTrade_DailyTradeLimit(t *testing.T) {
	config := DefaultRiskConfig()
	config.MaxDailyTrades = 3
	rm := NewRiskManager(config)

	for i := 0; i < 3; i++ {
		rm.RecordTrade(TradeRecord{PnL: 1.0, Timestamp: time.Now()})
	}

	ok, _ := rm.CanTrade(0.70)
	if ok {
		t.Error("expected trade blocked after reaching daily trade limit")
	}
}

func TestCanTrade_ConsecutiveLosses(t *testing.T) {
	config := DefaultRiskConfig()
	config.MaxConsecutiveLoss = 3
	rm := NewRiskManager(config)

	for i := 0; i < 3; i++ {
		rm.RecordTrade(TradeRecord{PnL: -2.0, Timestamp: time.Now()})
	}

	ok, _ := rm.CanTrade(0.70)
	if ok {
		t.Error("expected trade blocked after 3 consecutive losses")
	}
}

func TestCanTrade_CooldownPeriod(t *testing.T) {
	config := DefaultRiskConfig()
	config.MaxConsecutiveLoss = 2
	config.CooldownAfterLoss = 1 * time.Second
	rm := NewRiskManager(config)

	for i := 0; i < 2; i++ {
		rm.RecordTrade(TradeRecord{PnL: -2.0, Timestamp: time.Now()})
	}

	// Should be in cooldown
	ok, _ := rm.CanTrade(0.70)
	if ok {
		t.Error("expected trade blocked during cooldown")
	}

	// Wait for cooldown to expire
	time.Sleep(1100 * time.Millisecond)
	// A fresh trade at 0.70 confidence should still fail because consecutiveLosses >= MaxConsecutiveLoss
	ok2, _ := rm.CanTrade(0.70)
	if ok2 {
		t.Error("expected trade still blocked: consecutive losses not reset")
	}
}

func TestCanTrade_DrawdownLimit(t *testing.T) {
	config := DefaultRiskConfig()
	config.MaxDrawdownPct = 0.10 // 10%
	rm := NewRiskManager(config)
	rm.InitBalance(1000.0)

	// Simulate 15% drawdown
	rm.RecordTrade(TradeRecord{PnL: -150.0, Timestamp: time.Now()})

	ok, _ := rm.CanTrade(0.70)
	if ok {
		t.Error("expected trade blocked after exceeding max drawdown")
	}
}

func TestRecordTrade_WinResetConsecutiveLosses(t *testing.T) {
	rm := NewRiskManager(DefaultRiskConfig())

	rm.RecordTrade(TradeRecord{PnL: -5.0, Timestamp: time.Now()})
	rm.RecordTrade(TradeRecord{PnL: -5.0, Timestamp: time.Now()})
	if rm.consecutiveLosses != 2 {
		t.Errorf("expected 2 consecutive losses, got %d", rm.consecutiveLosses)
	}

	// A win should reset consecutive losses
	rm.RecordTrade(TradeRecord{PnL: 10.0, Timestamp: time.Now()})
	if rm.consecutiveLosses != 0 {
		t.Errorf("expected 0 consecutive losses after win, got %d", rm.consecutiveLosses)
	}
	if rm.consecutiveWins != 1 {
		t.Errorf("expected 1 consecutive win, got %d", rm.consecutiveWins)
	}
}

func TestRecordTrade_PeakBalanceUpdate(t *testing.T) {
	rm := NewRiskManager(DefaultRiskConfig())
	rm.InitBalance(100.0)

	rm.RecordTrade(TradeRecord{PnL: 50.0, Timestamp: time.Now()})
	if rm.peakBalance != 150.0 {
		t.Errorf("expected peakBalance=150, got %f", rm.peakBalance)
	}

	rm.RecordTrade(TradeRecord{PnL: -20.0, Timestamp: time.Now()})
	if rm.peakBalance != 150.0 {
		t.Errorf("expected peakBalance to remain 150, got %f", rm.peakBalance)
	}
}

func TestCalculatePositionSize_Basic(t *testing.T) {
	config := DefaultRiskConfig()
	config.RiskPerTrade = 0.05
	config.MaxPositionUSD = 50.0
	rm := NewRiskManager(config)

	shares := rm.CalculatePositionSize(0.70, 0.50, 1000.0)
	// Kelly: p=0.70, q=0.30, b=(1-0.50)/0.50=1.0
	// f* = (0.70*1.0 - 0.30)/1.0 = 0.40, fractional=0.20, capped at 0.05
	// positionUSD = 1000*0.05 = 50, shares = 50/0.50 = 100
	expected := 100.0
	if shares != expected {
		t.Errorf("expected %.1f shares, got %.1f", expected, shares)
	}
}

func TestCalculatePositionSize_CapsAtMax(t *testing.T) {
	config := DefaultRiskConfig()
	config.RiskPerTrade = 0.10
	config.MaxPositionUSD = 20.0
	rm := NewRiskManager(config)

	shares := rm.CalculatePositionSize(0.90, 0.40, 1000.0)
	// Kelly: p=0.90, q=0.10, b=(1-0.40)/0.40=1.5
	// f* = (0.90*1.5 - 0.10)/1.5 = 0.833, fractional=0.417, capped at 0.10
	// positionUSD = min(1000*0.10, 20) = 20, shares = 20/0.40 = 50
	maxShares := 20.0 / 0.40
	if shares != maxShares {
		t.Errorf("expected shares capped at %.1f, got %.1f", maxShares, shares)
	}
}

func TestCalculatePositionSize_MinimumShares(t *testing.T) {
	config := DefaultRiskConfig()
	config.RiskPerTrade = 0.01
	config.MaxPositionUSD = 50.0
	rm := NewRiskManager(config)

	shares := rm.CalculatePositionSize(0.55, 0.90, 10.0)
	// Kelly: p=0.55, q=0.45, b=(1-0.90)/0.90=0.111
	// f* = (0.55*0.111 - 0.45)/0.111 = -3.50, negative → 5 min shares
	if shares < 5.0 {
		t.Errorf("expected minimum 5 shares, got %.2f", shares)
	}
}

func TestCalculatePositionSize_ZeroPrice(t *testing.T) {
	rm := NewRiskManager(DefaultRiskConfig())
	shares := rm.CalculatePositionSize(0.70, 0, 1000.0)
	// Should use default 0.50
	if shares <= 0 {
		t.Errorf("expected positive shares even with zero price, got %.2f", shares)
	}
}

func TestCalculatePositionSize_KellyNegative(t *testing.T) {
	// When Kelly says don't bet (edge is negative), should return minimum 5 shares
	rm := NewRiskManager(DefaultRiskConfig())
	rm.InitBalance(1000.0)
	// confidence=0.40 at price=0.50 → p=0.40, b=1.0, f*=(0.40-0.60)/1.0=-0.20 → negative
	shares := rm.CalculatePositionSize(0.40, 0.50, 1000.0)
	if shares != 5.0 {
		t.Errorf("expected minimum 5 shares for negative Kelly, got %.2f", shares)
	}
}

func TestCalculatePositionSize_KellyLowOdds(t *testing.T) {
	// Token at $0.30 with 70% confidence — good Kelly edge
	config := DefaultRiskConfig()
	config.MaxPositionUSD = 100.0
	config.RiskPerTrade = 0.10
	rm := NewRiskManager(config)
	rm.InitBalance(500.0)
	// p=0.70, b=(1-0.30)/0.30=2.333, f*=(0.70*2.333-0.30)/2.333=0.571, half=0.286, capped at 0.10
	// positionUSD = 500*0.10 = 50, shares = 50/0.30 = 166.67
	shares := rm.CalculatePositionSize(0.70, 0.30, 500.0)
	expectedUSD := 500.0 * 0.10 // capped at RiskPerTrade
	expectedShares := expectedUSD / 0.30
	if shares < expectedShares-0.1 || shares > expectedShares+0.1 {
		t.Errorf("expected ~%.1f shares, got %.1f", expectedShares, shares)
	}
}

func TestGetDailyStats(t *testing.T) {
	rm := NewRiskManager(DefaultRiskConfig())
	rm.InitBalance(500.0)
	rm.RecordTrade(TradeRecord{PnL: 10.0, Timestamp: time.Now()})
	rm.RecordTrade(TradeRecord{PnL: -5.0, Timestamp: time.Now()})

	trades, pnl, drawdown := rm.GetDailyStats()
	if trades != 2 {
		t.Errorf("expected 2 trades, got %d", trades)
	}
	if pnl != 5.0 {
		t.Errorf("expected PnL=5.0, got %f", pnl)
	}
	// peak=510, current=505 => drawdown = 5/510 * 100 ≈ 0.98%
	if drawdown < 0 || drawdown > 5 {
		t.Errorf("expected small drawdown, got %f%%", drawdown)
	}
}
