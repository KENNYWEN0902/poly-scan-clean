package btc

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"sync"
	"time"
)

// PerformanceTracker tracks trading performance metrics
type PerformanceTracker struct {
	mu sync.RWMutex

	// Overall stats
	totalTrades    int
	winningTrades  int
	losingTrades   int
	totalPnL       float64
	bestTrade      float64
	worstTrade     float64
	tradeDurations []time.Duration

	// Claim stats
	claimAttempts  int
	claimSuccesses int

	// Drawdown tracking
	peakBalance    float64
	currentBalance float64
	maxDrawdown    float64

	// Time tracking
	startTime   time.Time
	lastUpdated time.Time

	// Daily stats
	dailyStats map[string]*DailyPerformance

	// Trade history for calculations
	tradeResults []TradeResult
}

// TradeResult records the result of a completed trade
type TradeResult struct {
	Timestamp  time.Time `json:"timestamp"`
	MarketID   string    `json:"market_id"`
	Direction  string    `json:"direction"`
	EntryPrice float64   `json:"entry_price"`
	ExitPrice  float64   `json:"exit_price"`
	Size       float64   `json:"size"`
	PnL        float64   `json:"pnl"`
	Confidence float64   `json:"confidence"`
	OpenTime   time.Time `json:"open_time"`
	CloseTime  time.Time `json:"close_time"`
	Success    bool      `json:"success"`
}

// DailyPerformance tracks daily performance metrics
type DailyPerformance struct {
	Date           string  `json:"date"`
	Trades         int     `json:"trades"`
	Wins           int     `json:"wins"`
	Losses         int     `json:"losses"`
	PnL            float64 `json:"pnl"`
	WinRate        float64 `json:"win_rate"`
	AvgConfidence  float64 `json:"avg_confidence"`
	ClaimAttempts  int     `json:"claim_attempts"`
	ClaimSuccesses int     `json:"claim_successes"`
}

// PerformanceStats represents aggregated performance statistics
type PerformanceStats struct {
	TotalTrades      int     `json:"total_trades"`
	WinningTrades    int     `json:"winning_trades"`
	LosingTrades     int     `json:"losing_trades"`
	WinRate          float64 `json:"win_rate"`
	TotalPnL         float64 `json:"total_pnl"`
	AveragePnL       float64 `json:"average_pnl"`
	BestTrade        float64 `json:"best_trade"`
	WorstTrade       float64 `json:"worst_trade"`
	ClaimAttempts    int     `json:"claim_attempts"`
	ClaimSuccesses   int     `json:"claim_successes"`
	ClaimSuccessRate float64 `json:"claim_success_rate"`
	AverageHoldTime  string  `json:"average_hold_time"`
	MaxDrawdown      float64 `json:"max_drawdown"`
	SharpeRatio      float64 `json:"sharpe_ratio"`
	LastUpdated      string  `json:"last_updated"`
	StartTime        string  `json:"start_time"`
	Uptime           string  `json:"uptime"`
}

// NewPerformanceTracker creates a new performance tracker
func NewPerformanceTracker() *PerformanceTracker {
	pt := &PerformanceTracker{
		startTime:    time.Now(),
		lastUpdated:  time.Now(),
		dailyStats:   make(map[string]*DailyPerformance),
		tradeResults: make([]TradeResult, 0, 1000),
		peakBalance:  0, // Tracks peak cumulative PnL (currentBalance is cumulative PnL from 0)
	}

	// Load existing data
	if err := pt.Load(); err != nil {
		log.Printf("[PERFORMANCE] Starting fresh: %v", err)
	}

	return pt
}

// RecordTrade records a completed trade result
func (pt *PerformanceTracker) RecordTrade(result TradeResult) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	pt.totalTrades++
	pt.totalPnL += result.PnL
	pt.lastUpdated = time.Now()

	if result.PnL > 0 {
		pt.winningTrades++
	} else if result.PnL < 0 {
		pt.losingTrades++
	}

	// Track best/worst
	if result.PnL > pt.bestTrade || pt.bestTrade == 0 {
		pt.bestTrade = result.PnL
	}
	if result.PnL < pt.worstTrade || pt.worstTrade == 0 {
		pt.worstTrade = result.PnL
	}

	// Track duration
	if !result.CloseTime.IsZero() && !result.OpenTime.IsZero() {
		duration := result.CloseTime.Sub(result.OpenTime)
		pt.tradeDurations = append(pt.tradeDurations, duration)
	}

	// Update balance tracking
	pt.currentBalance += result.PnL
	if pt.currentBalance > pt.peakBalance {
		pt.peakBalance = pt.currentBalance
	}

	// Calculate drawdown (absolute USD from peak PnL)
	drawdown := pt.peakBalance - pt.currentBalance
	if drawdown > pt.maxDrawdown {
		pt.maxDrawdown = drawdown
	}

	// Store result
	pt.tradeResults = append(pt.tradeResults, result)

	// Update daily stats
	date := result.Timestamp.Format("2006-01-02")
	if _, exists := pt.dailyStats[date]; !exists {
		pt.dailyStats[date] = &DailyPerformance{Date: date}
	}
	daily := pt.dailyStats[date]
	daily.Trades++
	daily.PnL += result.PnL
	daily.AvgConfidence = (daily.AvgConfidence*float64(daily.Trades-1) + result.Confidence) / float64(daily.Trades)
	if result.PnL > 0 {
		daily.Wins++
	} else if result.PnL < 0 {
		daily.Losses++
	}
	if daily.Trades > 0 {
		daily.WinRate = float64(daily.Wins) / float64(daily.Trades) * 100
	}

	log.Printf("[PERFORMANCE] Recorded trade: PnL=$%.2f, Total: %d trades, Win Rate: %.1f%%",
		result.PnL, pt.totalTrades, pt.calculateWinRate())

	// Save after every trade
	go pt.Save()
}

// RecordClaimAttempt records a claim attempt
func (pt *PerformanceTracker) RecordClaimAttempt(success bool) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	pt.claimAttempts++
	if success {
		pt.claimSuccesses++
	}

	// Update today's daily stats
	today := time.Now().Format("2006-01-02")
	if _, exists := pt.dailyStats[today]; !exists {
		pt.dailyStats[today] = &DailyPerformance{Date: today}
	}
	pt.dailyStats[today].ClaimAttempts++
	if success {
		pt.dailyStats[today].ClaimSuccesses++
	}

	pt.lastUpdated = time.Now()

	// Calculate inline to avoid recursive lock (GetClaimSuccessRate acquires RLock)
	var claimRate float64
	if pt.claimAttempts > 0 {
		claimRate = float64(pt.claimSuccesses) / float64(pt.claimAttempts) * 100
	}
	log.Printf("[PERFORMANCE] Claim recorded: success=%v, rate=%.1f%%",
		success, claimRate)
}

// GetStats returns current performance statistics
func (pt *PerformanceTracker) GetStats() PerformanceStats {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	stats := PerformanceStats{
		TotalTrades:    pt.totalTrades,
		WinningTrades:  pt.winningTrades,
		LosingTrades:   pt.losingTrades,
		WinRate:        pt.calculateWinRate(),
		TotalPnL:       pt.totalPnL,
		BestTrade:      pt.bestTrade,
		WorstTrade:     pt.worstTrade,
		ClaimAttempts:  pt.claimAttempts,
		ClaimSuccesses: pt.claimSuccesses,
		MaxDrawdown:    pt.maxDrawdown, // Absolute USD drawdown
		StartTime:      pt.startTime.Format(time.RFC3339),
		LastUpdated:    pt.lastUpdated.Format(time.RFC3339),
		Uptime:         time.Since(pt.startTime).Round(time.Second).String(),
	}

	// Calculate average PnL
	if pt.totalTrades > 0 {
		stats.AveragePnL = pt.totalPnL / float64(pt.totalTrades)
	}

	// Calculate claim success rate
	if pt.claimAttempts > 0 {
		stats.ClaimSuccessRate = float64(pt.claimSuccesses) / float64(pt.claimAttempts) * 100
	}

	// Calculate average hold time
	if len(pt.tradeDurations) > 0 {
		var total time.Duration
		for _, d := range pt.tradeDurations {
			total += d
		}
		stats.AverageHoldTime = (total / time.Duration(len(pt.tradeDurations))).Round(time.Second).String()
	}

	// Calculate Sharpe ratio (simplified)
	stats.SharpeRatio = pt.calculateSharpeRatio()

	return stats
}

// GetWinRate returns the current win rate
func (pt *PerformanceTracker) GetWinRate() float64 {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return pt.calculateWinRate()
}

// GetClaimSuccessRate returns the claim success rate
func (pt *PerformanceTracker) GetClaimSuccessRate() float64 {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	if pt.claimAttempts == 0 {
		return 0
	}
	return float64(pt.claimSuccesses) / float64(pt.claimAttempts) * 100
}

// GetDailyStats returns performance stats for a specific date
func (pt *PerformanceTracker) GetDailyStats(date string) *DailyPerformance {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return pt.dailyStats[date]
}

// GetWeeklyStats returns aggregated weekly performance
func (pt *PerformanceTracker) GetWeeklyStats() *DailyPerformance {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	weekly := &DailyPerformance{
		Date: fmt.Sprintf("Week of %s", time.Now().AddDate(0, 0, -7).Format("2006-01-02")),
	}

	now := time.Now()
	weekAgo := now.AddDate(0, 0, -7)

	for date, daily := range pt.dailyStats {
		t, err := time.Parse("2006-01-02", date)
		if err != nil {
			continue
		}
		if t.After(weekAgo) || t.Equal(weekAgo) {
			weekly.Trades += daily.Trades
			weekly.Wins += daily.Wins
			weekly.Losses += daily.Losses
			weekly.PnL += daily.PnL
			weekly.ClaimAttempts += daily.ClaimAttempts
			weekly.ClaimSuccesses += daily.ClaimSuccesses
		}
	}

	if weekly.Trades > 0 {
		weekly.WinRate = float64(weekly.Wins) / float64(weekly.Trades) * 100
	}

	return weekly
}

// GetAllDailyStats returns all daily performance records
func (pt *PerformanceTracker) GetAllDailyStats() map[string]*DailyPerformance {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	result := make(map[string]*DailyPerformance)
	for k, v := range pt.dailyStats {
		result[k] = v
	}
	return result
}

// Save persists performance data to disk
func (pt *PerformanceTracker) Save() error {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	data := struct {
		TotalTrades    int                          `json:"total_trades"`
		WinningTrades  int                          `json:"winning_trades"`
		LosingTrades   int                          `json:"losing_trades"`
		TotalPnL       float64                      `json:"total_pnl"`
		BestTrade      float64                      `json:"best_trade"`
		WorstTrade     float64                      `json:"worst_trade"`
		ClaimAttempts  int                          `json:"claim_attempts"`
		ClaimSuccesses int                          `json:"claim_successes"`
		MaxDrawdown    float64                      `json:"max_drawdown"`
		PeakBalance    float64                      `json:"peak_balance"`
		CurrentBalance float64                      `json:"current_balance"`
		StartTime      time.Time                    `json:"start_time"`
		LastUpdated    time.Time                    `json:"last_updated"`
		DailyStats     map[string]*DailyPerformance `json:"daily_stats"`
		TradeResults   []TradeResult                `json:"trade_results"`
	}{
		TotalTrades:    pt.totalTrades,
		WinningTrades:  pt.winningTrades,
		LosingTrades:   pt.losingTrades,
		TotalPnL:       pt.totalPnL,
		BestTrade:      pt.bestTrade,
		WorstTrade:     pt.worstTrade,
		ClaimAttempts:  pt.claimAttempts,
		ClaimSuccesses: pt.claimSuccesses,
		MaxDrawdown:    pt.maxDrawdown,
		PeakBalance:    pt.peakBalance,
		CurrentBalance: pt.currentBalance,
		StartTime:      pt.startTime,
		LastUpdated:    pt.lastUpdated,
		DailyStats:     pt.dailyStats,
		TradeResults:   pt.tradeResults,
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal performance data: %v", err)
	}

	os.MkdirAll("data", 0755)
	if err := os.WriteFile("data/performance.json", jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write performance file: %v", err)
	}

	log.Printf("[PERFORMANCE] Saved performance data to disk")
	return nil
}

// Load restores performance data from disk
func (pt *PerformanceTracker) Load() error {
	data, err := os.ReadFile("data/performance.json")
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No file exists yet
		}
		return fmt.Errorf("failed to read performance file: %v", err)
	}

	var saved struct {
		TotalTrades    int                          `json:"total_trades"`
		WinningTrades  int                          `json:"winning_trades"`
		LosingTrades   int                          `json:"losing_trades"`
		TotalPnL       float64                      `json:"total_pnl"`
		BestTrade      float64                      `json:"best_trade"`
		WorstTrade     float64                      `json:"worst_trade"`
		ClaimAttempts  int                          `json:"claim_attempts"`
		ClaimSuccesses int                          `json:"claim_successes"`
		MaxDrawdown    float64                      `json:"max_drawdown"`
		PeakBalance    float64                      `json:"peak_balance"`
		CurrentBalance float64                      `json:"current_balance"`
		StartTime      time.Time                    `json:"start_time"`
		LastUpdated    time.Time                    `json:"last_updated"`
		DailyStats     map[string]*DailyPerformance `json:"daily_stats"`
		TradeResults   []TradeResult                `json:"trade_results"`
	}

	if err := json.Unmarshal(data, &saved); err != nil {
		return fmt.Errorf("failed to unmarshal performance data: %v", err)
	}

	pt.mu.Lock()
	defer pt.mu.Unlock()

	pt.totalTrades = saved.TotalTrades
	pt.winningTrades = saved.WinningTrades
	pt.losingTrades = saved.LosingTrades
	pt.totalPnL = saved.TotalPnL
	pt.bestTrade = saved.BestTrade
	pt.worstTrade = saved.WorstTrade
	pt.claimAttempts = saved.ClaimAttempts
	pt.claimSuccesses = saved.ClaimSuccesses
	pt.maxDrawdown = saved.MaxDrawdown
	pt.peakBalance = saved.PeakBalance
	pt.currentBalance = saved.CurrentBalance
	pt.startTime = saved.StartTime
	pt.lastUpdated = saved.LastUpdated
	pt.dailyStats = saved.DailyStats
	pt.tradeResults = saved.TradeResults

	if pt.dailyStats == nil {
		pt.dailyStats = make(map[string]*DailyPerformance)
	}
	if pt.tradeResults == nil {
		pt.tradeResults = make([]TradeResult, 0)
	}

	log.Printf("[PERFORMANCE] Loaded %d historical trades from disk", pt.totalTrades)
	return nil
}

// Internal helper methods

func (pt *PerformanceTracker) calculateWinRate() float64 {
	if pt.totalTrades == 0 {
		return 0
	}
	return float64(pt.winningTrades) / float64(pt.totalTrades) * 100
}

func (pt *PerformanceTracker) calculateSharpeRatio() float64 {
	if len(pt.tradeResults) < 2 {
		return 0
	}

	// Calculate returns
	returns := make([]float64, len(pt.tradeResults))
	for i, trade := range pt.tradeResults {
		if trade.EntryPrice > 0 {
			returns[i] = trade.PnL / (trade.Size * trade.EntryPrice)
		}
	}

	// Calculate mean return
	var sum float64
	for _, r := range returns {
		sum += r
	}
	mean := sum / float64(len(returns))

	// Calculate standard deviation
	var variance float64
	for _, r := range returns {
		diff := r - mean
		variance += diff * diff
	}
	stdDev := math.Sqrt(variance / float64(len(returns)-1))

	if stdDev == 0 {
		return 0
	}

	return (mean / stdDev) * math.Sqrt(250)
}


