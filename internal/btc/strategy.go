package btc

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"poly-scan/internal/execution"
	"poly-scan/internal/notification"
	"poly-scan/internal/polymarket"
)

// BTCMarketConfig holds configuration for BTC market trading
type BTCMarketConfig struct {
	WindowDuration    time.Duration // 5 minutes
	PredictBeforeEnd  time.Duration // How long before window end to predict
	MinConfidence     float64       // Minimum confidence to trade
	MaxPositionSize   float64       // Maximum position in USD
	MinPriceChangePct float64       // Minimum price change to consider
	ExecutionLeadTime time.Duration // How long before window end to execute
	CooldownPerMarket time.Duration // Cooldown between trades on same market

	// Order pricing
	UseDynamicPricing bool    // Use orderbook-based pricing
	PriceSlippage     float64 // Acceptable price slippage (e.g., 0.02 = 2%)

	// Risk management
	EnableRiskMgmt bool // Enable risk management

	// Stop-loss and Take-profit settings
	EnableExitStrategy     bool          // Enable automatic exit strategy
	ExitBeforeEnd          time.Duration // Force exit this long before window end
	TrendReversalThreshold float64       // Trend reversal threshold for stop-loss (0-1)
	TakeProfitPct          float64       // Take profit when price reaches this % gain
	StopLossPct            float64       // Stop loss when price drops this % from entry

	// Time-of-day trading hours (UTC)
	TradingTimezone    string // Timezone for trading hours (e.g. "Asia/Shanghai")
	BoostHoursStart    int    // Start hour for boosted sizing (0-23, in TradingTimezone)
	BoostHoursEnd      int    // End hour for boosted sizing
	QuietHoursStart    int    // Start hour for reduced sizing (-1 to disable)
	QuietHoursEnd      int    // End hour for reduced sizing
	QuietSizeMultiplier float64 // Position size multiplier during quiet hours (0.5 = half)
	BoostSizeMultiplier float64 // Position size multiplier during boost hours (1.5 = 50% more)
}

// DefaultBTCMarketConfig returns default configuration
// Optimized for oracle-lag arbitrage strategy based on research:
// - 0.07% minimum price change (proven 61.4% win rate threshold)
// - 55-second oracle lag exploitation window
// - Low odds entry (token price <= 0.35)
func DefaultBTCMarketConfig() BTCMarketConfig {
	return BTCMarketConfig{
		WindowDuration:         5 * time.Minute,
		PredictBeforeEnd:       120 * time.Second, // Extended: catch price moves before market adjusts (oracle-lag)
		MinConfidence:          0.52,             // 52% threshold (GTC maker orders are fee-free, EV+ at 52%)
		MaxPositionSize:        15.0,
		MinPriceChangePct:      0.01,             // 0.01% threshold (~$7 for BTC; confidence filters noise)
		ExecutionLeadTime:      15 * time.Second, // Reduced from 20s for faster execution
		CooldownPerMarket:      1 * time.Minute,
		UseDynamicPricing:      true,
		PriceSlippage:          0.03,
		EnableRiskMgmt:         true,
		EnableExitStrategy:     true,
		ExitBeforeEnd:          8 * time.Second,
		TrendReversalThreshold: 0.3,
		TakeProfitPct:          0.50, // Take profit at 50% gain
		StopLossPct:            0.30, // Stop loss at 30% decline (tiered: wider for cheap tokens)
		TradingTimezone:        "Asia/Shanghai",
		BoostHoursStart:        6,    // UTC+8 06:00 — historically most profitable
		BoostHoursEnd:          9,    // UTC+8 09:00
		QuietHoursStart:        18,   // UTC+8 18:00 — historically net negative
		QuietHoursEnd:          22,   // UTC+8 22:00
		QuietSizeMultiplier:    0.5,  // Half position during quiet hours
		BoostSizeMultiplier:    1.5,  // 50% more during boost hours
	}
}

// BTCMarket represents a BTC Up/Down market
type BTCMarket struct {
	ID             string
	Question       string
	UpTokenID      string
	DownTokenID    string
	WindowStart    time.Time
	WindowEnd      time.Time
	StartPrice     float64
	EndPrice       float64
	UpTokenPrice   float64 // Market price from outcomePrices
	DownTokenPrice float64 // Market price from outcomePrices
}

// Position represents an active position
type Position struct {
	MarketID      string
	TokenID       string
	Side          string // "UP" or "DOWN"
	EntryPrice    float64
	Size          float64
	OpenTime      time.Time
	IsActive      bool
	CloseReason   string
	WindowEnd     time.Time // When the market window ends
	OriginalTrend string    // Original trend direction when position opened
	OriginalConf  float64   // Original confidence when position opened
	ExitAttempted bool      // Whether we already tried to exit
	ExitRetries   int       // Number of exit retry attempts
	BTCStartPrice float64   // BTC spot price at window start (for outcome determination)
}

// BTCStrategy handles BTC 5-minute market trading
type BTCStrategy struct {
	client           *polymarket.Client
	execEngine       *execution.Engine
	spotMonitor      *SpotMonitor
	chainlinkMonitor *ChainlinkMonitor
	predictor        *MarketPredictor
	riskManager      *RiskManager
	config           BTCMarketConfig

	// Performance tracking
	performanceTracker *PerformanceTracker
	notifier           *notification.Notifier

	// Active markets
	markets      map[string]*BTCMarket
	marketState  map[string]map[string]*polymarket.OrderbookUpdate
	lastExecTime map[string]time.Time
	mu           sync.RWMutex

	// Position tracking
	positions  map[string]*Position
	positionMu sync.RWMutex

	// Separate mutex for lastExecTime to avoid deadlock
	execTimeMu sync.Mutex

	// Track executed windows to avoid repeated API calls (Cloudflare rate limit protection)
	executedWindows map[string]bool
	execWinMu       sync.Mutex

	// Track retry attempts per window to prevent FOK/execution spam
	windowRetries   map[string]int
	windowRetriesMu sync.Mutex

	// Log throttling — only log scan loop details every 5 seconds
	lastScanLog     time.Time
	lastScanLogMu   sync.Mutex

	// 退出策略价格刷新节流
	lastPriceRefresh time.Time
	lastPnLLog       time.Time

	// Trade statistics
	totalTrades   int
	winningTrades int
	losingTrades  int
	totalPnL      float64
	statsMu       sync.RWMutex

	// Control
	stopChan chan struct{}
	wg       sync.WaitGroup
	running  bool
	stopped  bool // Whether trading is manually stopped
}

// NewBTCStrategy creates a new BTC strategy
func NewBTCStrategy(client *polymarket.Client, execEngine *execution.Engine, config BTCMarketConfig) *BTCStrategy {
	spotMonitor := NewSpotMonitor()
	chainlinkMonitor := NewChainlinkMonitor()
	// Use strategy config for predictor to ensure consistent thresholds
	predictorConfig := DefaultPredictorConfig()
	predictorConfig.MinConfidence = config.MinConfidence
	predictorConfig.MinPriceChangePct = config.MinPriceChangePct
	predictor := NewMarketPredictor(spotMonitor, chainlinkMonitor, predictorConfig)
	riskManager := NewRiskManager(DefaultRiskConfig())
	performanceTracker := NewPerformanceTracker()
	notifier := notification.NewNotifier(notification.DefaultConfig())

	// Initialize strategy counters from loaded performance data
	stats := performanceTracker.GetStats()

	return &BTCStrategy{
		client:             client,
		execEngine:         execEngine,
		spotMonitor:        spotMonitor,
		chainlinkMonitor:   chainlinkMonitor,
		predictor:          predictor,
		riskManager:        riskManager,
		config:             config,
		performanceTracker: performanceTracker,
		notifier:           notifier,
		totalTrades:        stats.TotalTrades,
		winningTrades:      stats.WinningTrades,
		losingTrades:       stats.LosingTrades,
		totalPnL:           stats.TotalPnL,
		markets:            make(map[string]*BTCMarket),
		marketState:        make(map[string]map[string]*polymarket.OrderbookUpdate),
		lastExecTime:       make(map[string]time.Time),
		positions:          make(map[string]*Position),
		executedWindows:    make(map[string]bool),
		windowRetries:     make(map[string]int),
		stopChan:           make(chan struct{}),
	}
}

// getTimeSizeMultiplier returns a position size multiplier based on time of day.
// Boost hours increase sizing, quiet hours decrease it.
func (s *BTCStrategy) getTimeSizeMultiplier() float64 {
	loc, err := time.LoadLocation(s.config.TradingTimezone)
	if err != nil {
		return 1.0
	}
	hour := time.Now().In(loc).Hour()

	// Boost hours (e.g., UTC+8 06:00-09:00)
	if s.config.BoostHoursStart <= s.config.BoostHoursEnd {
		if hour >= s.config.BoostHoursStart && hour < s.config.BoostHoursEnd {
			return s.config.BoostSizeMultiplier
		}
	} else { // wraps midnight
		if hour >= s.config.BoostHoursStart || hour < s.config.BoostHoursEnd {
			return s.config.BoostSizeMultiplier
		}
	}

	// Quiet hours (e.g., UTC+8 18:00-22:00)
	if s.config.QuietHoursStart >= 0 {
		if s.config.QuietHoursStart <= s.config.QuietHoursEnd {
			if hour >= s.config.QuietHoursStart && hour < s.config.QuietHoursEnd {
				return s.config.QuietSizeMultiplier
			}
		} else { // wraps midnight
			if hour >= s.config.QuietHoursStart || hour < s.config.QuietHoursEnd {
				return s.config.QuietSizeMultiplier
			}
		}
	}

	return 1.0
}

// Start begins the BTC strategy
func (s *BTCStrategy) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.mu.Unlock()

	// Load persisted positions
	if err := s.loadPositions(); err != nil {
		log.Printf("[BTC] Warning: Failed to load positions: %v", err)
	}

	// Initialize risk manager balance from real USDC balance
	usdcBalance, err := s.execEngine.CheckUSDCBalance()
	if err == nil && usdcBalance > 0 {
		s.riskManager.InitBalance(usdcBalance)
		log.Printf("[BTC] \U0001f4b0 Risk manager initialized with real USDC balance: $%.2f", usdcBalance)
	} else {
		log.Printf("[BTC] \u26a0\ufe0f Could not fetch USDC balance for risk manager init, using default $1000")
	}

	// Start price monitors
	s.spotMonitor.Start()
	s.chainlinkMonitor.Start()

	// Fetch active BTC markets
	if err := s.refreshMarkets(); err != nil {
		log.Printf("[BTC] Failed to fetch markets: %v", err)
	}

	// Start main loop
	s.wg.Add(4)
	go func() { defer s.wg.Done(); s.mainLoop() }()
	go func() { defer s.wg.Done(); s.marketRefreshLoop() }()
	go func() { defer s.wg.Done(); s.statusLoop() }()
	go func() { defer s.wg.Done(); s.claimLoop() }()
	log.Println("[BTC] Strategy started - monitoring BTC 5-minute markets")
	log.Println("[BTC] Configuration:")
	log.Printf("  - Min Confidence: %.0f%%", s.config.MinConfidence*100)
	log.Printf("  - Min Price Change: %.3f%%", s.config.MinPriceChangePct)
	log.Printf("  - Dynamic Pricing: %v", s.config.UseDynamicPricing)
	log.Printf("  - Risk Management: %v", s.config.EnableRiskMgmt)
	return nil
}

// Stop stops the strategy
func (s *BTCStrategy) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	close(s.stopChan)
	s.mu.Unlock()

	// Wait for all goroutines to finish (outside lock to avoid deadlock)
	s.wg.Wait()

	// Save performance data before shutdown
	if err := s.performanceTracker.Save(); err != nil {
		log.Printf("[BTC] Warning: failed to save performance data: %v", err)
	}

	s.spotMonitor.Stop()
	s.chainlinkMonitor.Stop()
	log.Println("[BTC] Strategy stopped")
}

// mainLoop is the main trading loop
func (s *BTCStrategy) mainLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.checkTradingOpportunities()
			if s.config.EnableExitStrategy {
				s.checkExitStrategy()
			}
		}
	}
}

// marketRefreshLoop periodically refreshes the list of BTC markets
func (s *BTCStrategy) marketRefreshLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			if err := s.refreshMarkets(); err != nil {
				log.Printf("[BTC] Failed to refresh markets: %v", err)
			}
		}
	}
}

// statusLoop prints periodic status updates
func (s *BTCStrategy) statusLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.printStatus()
		}
	}
}

// refreshMarkets fetches active BTC markets from Polymarket
func (s *BTCStrategy) refreshMarkets() error {
	now := time.Now()
	nowUTC := now.UTC()

	// Calculate current 5-minute window in UTC (Polymarket uses UTC timestamps)
	minutes := nowUTC.Minute()
	windowStart := minutes / 5
	windowStartTime := time.Date(nowUTC.Year(), nowUTC.Month(), nowUTC.Day(), nowUTC.Hour(), windowStart*5, 0, 0, time.UTC)

	// Current and next window
	windows := []time.Time{windowStartTime, windowStartTime.Add(5 * time.Minute)}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Clean up expired windows (keep only current and next window)
	activeWindowMap := make(map[string]bool)
	for _, w := range windows {
		activeWindowMap[fmt.Sprintf("btc-updown-5m-%d", w.Unix())] = true
	}

	for marketID := range s.markets {
		if !activeWindowMap[marketID] {
			log.Printf("[BTC] 🧹 Removing expired window: %s", marketID)
			delete(s.markets, marketID)
		}
	}

	// Clean up stale entries in auxiliary maps to prevent unbounded growth
	s.execTimeMu.Lock()
	for marketID := range s.lastExecTime {
		if !activeWindowMap[marketID] {
			delete(s.lastExecTime, marketID)
		}
	}
	s.execTimeMu.Unlock()

	s.execWinMu.Lock()
	for marketID := range s.executedWindows {
		if !activeWindowMap[marketID] {
			delete(s.executedWindows, marketID)
		}
	}
	s.execWinMu.Unlock()

	s.windowRetriesMu.Lock()
	for marketID := range s.windowRetries {
		if !activeWindowMap[marketID] {
			delete(s.windowRetries, marketID)
		}
	}
	s.windowRetriesMu.Unlock()

	for _, windowStart := range windows {
		timestamp := windowStart.Unix()
		marketID := fmt.Sprintf("btc-updown-5m-%d", timestamp)

		if _, exists := s.markets[marketID]; !exists {
			parsedMarket, err := s.client.GetBTCMarket(timestamp)
			if err != nil {
				log.Printf("[BTC] Failed to fetch market %s: %v", marketID, err)
				continue
			}

			var upTokenID, downTokenID string
			var upTokenPrice, downTokenPrice float64
			for _, token := range parsedMarket.Tokens {
				if token.Outcome == "Up" {
					upTokenID = token.TokenID
					upTokenPrice = token.Price
				} else if token.Outcome == "Down" {
					downTokenID = token.TokenID
					downTokenPrice = token.Price
				}
			}

			if upTokenID == "" || downTokenID == "" {
				log.Printf("[BTC] Missing token IDs for market %s", marketID)
				continue
			}

						spotPrice := s.spotMonitor.GetCurrentPrice()
			s.markets[marketID] = &BTCMarket{
				ID:             marketID,
				Question:       parsedMarket.Question,
				UpTokenID:      upTokenID,
				DownTokenID:    downTokenID,
				WindowStart:    windowStart,
				WindowEnd:      windowStart.Add(5 * time.Minute),
				StartPrice:     spotPrice, // Record BTC spot price at window start
				UpTokenPrice:   upTokenPrice,
				DownTokenPrice: downTokenPrice,
			}
			log.Printf("[BTC] ✅ Tracking new window: %s - %s",
				windowStart.Format("15:04:05"),
				windowStart.Add(5*time.Minute).Format("15:04:05"))
			log.Printf("[BTC]    Up Token: %s...", upTokenID[:20])
			log.Printf("[BTC]    Down Token: %s...", downTokenID[:20])
		}
	}

	return nil
}

// SetStopped enables/disables manual trading stop (monitoring and position management continue)
func (s *BTCStrategy) SetStopped(stopped bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = stopped
	if stopped {
		log.Printf("[BTC] ⚠️ Trading manually STOPPED by user")
	} else {
		log.Printf("[BTC] ✅ Trading manually RESUMED by user")
	}
}

func (s *BTCStrategy) IsStopped() bool {
	s.mu.RLock()
	manualStop := s.stopped
	s.mu.RUnlock()
	if manualStop {
		return true
	}
	// Also check file-based stop signal (from dashboard API)
	if _, err := os.Stat("data/trading_stopped"); err == nil {
		return true
	}
	return false
}

func (s *BTCStrategy) checkTradingOpportunities() {
	// Skip new trade entries when manually stopped
	if s.IsStopped() {
		return
	}

	now := time.Now()

	// Collect candidates under RLock, release before executing remote calls
	type tradeCandidate struct {
		market     *BTCMarket
		prediction *Prediction
	}
	var candidates []tradeCandidate

	// Snapshot cooldown data under its own mutex first, BEFORE acquiring mu.RLock
	s.execTimeMu.Lock()
	execTimeSnapshot := make(map[string]time.Time, len(s.lastExecTime))
	for k, v := range s.lastExecTime {
		execTimeSnapshot[k] = v
	}
	s.execTimeMu.Unlock()

	// Snapshot executedWindows to skip early (avoid predictions + log spam)
	s.execWinMu.Lock()
	execWinSnapshot := make(map[string]bool, len(s.executedWindows))
	for k, v := range s.executedWindows {
		execWinSnapshot[k] = v
	}
	s.execWinMu.Unlock()

	s.mu.RLock()
	for marketID, market := range s.markets {
		// Check if window is still active
		if now.After(market.WindowEnd) {
			continue
		}

		// Skip already attempted windows early (before prediction)
		if execWinSnapshot[marketID] {
			continue
		}

		// Check cooldown from snapshot (no nested lock)
		if lastExec, exists := execTimeSnapshot[marketID]; exists {
			if time.Since(lastExec) < s.config.CooldownPerMarket {
				continue
			}
		}

		// Time remaining in window
		timeRemaining := time.Until(market.WindowEnd)

		// Only trade in the last few seconds
		if timeRemaining > s.config.PredictBeforeEnd {
			continue
		}
		if timeRemaining < s.config.ExecutionLeadTime {
			continue
		}

		// Throttle scan logs to every 5 seconds
		s.lastScanLogMu.Lock()
		shouldLog := time.Since(s.lastScanLog) >= 5*time.Second
		if shouldLog {
			s.lastScanLog = time.Now()
		}
		s.lastScanLogMu.Unlock()

		if shouldLog {
			log.Printf("[BTC] ⏰ Window %s in trading window: timeRemaining=%v", marketID, timeRemaining)
		}

		// Make prediction with enhanced model
		prediction := s.predictor.PredictAtWindowEnd(market.WindowStart, s.config.WindowDuration)
		if prediction == nil {
			if shouldLog {
				log.Printf("[BTC] 🔍 No prediction for %s (time remaining: %v)", marketID, timeRemaining)
			}
			continue
		}

		log.Printf("[BTC] 🔍 Prediction: direction=%s confidence=%.2f%% priceChange=%.4f%%",
			prediction.Direction, prediction.Confidence*100, prediction.PriceChange)

		// Check confidence
		if prediction.Confidence < s.config.MinConfidence {
			log.Printf("[BTC] ⏭️ Skipping: confidence %.2f < threshold %.2f", prediction.Confidence, s.config.MinConfidence)
			continue
		}

		// Check price change threshold
		if abs(prediction.PriceChange) < s.config.MinPriceChangePct {
			log.Printf("[BTC] ⏭️ Skipping: priceChange %.4f < threshold %.4f", prediction.PriceChange, s.config.MinPriceChangePct)
			continue
		}

		// Risk management check
		if s.config.EnableRiskMgmt {
			canTrade, reason := s.riskManager.CanTrade(prediction.Confidence)
			if !canTrade {
				log.Printf("[BTC] ⚠️ Risk manager blocked trade: %s", reason)
				continue
			}
		}

		log.Printf("[BTC] ✅ All checks passed, adding candidate for %s", marketID)

		candidates = append(candidates, tradeCandidate{market: market, prediction: prediction})
	}
	s.mu.RUnlock()

	// Execute trades outside of RLock to avoid blocking market refresh
	for _, c := range candidates {
		// Check executed windows OUTSIDE of mu.RLock to avoid nested lock
		s.execWinMu.Lock()
		if s.executedWindows[c.market.ID] {
			log.Printf("[BTC] ⏭️ Skipping: already attempted window %s", c.market.ID)
			s.execWinMu.Unlock()
			continue
		}
		s.executedWindows[c.market.ID] = true
		s.execWinMu.Unlock()

		s.executeTrade(c.market, c.prediction)
	}
}

// executeTrade executes a trade based on prediction
func (s *BTCStrategy) executeTrade(market *BTCMarket, prediction *Prediction) {
	// Get fresh prices from API (market prices change rapidly near settlement)
	slug := fmt.Sprintf("btc-updown-5m-%d", market.WindowStart.Unix())
	freshUpPrice, freshDownPrice, err := s.client.GetMarketPrices(slug)
	if err != nil {
		log.Printf("[BTC] ⚠️ Failed to get fresh prices: %v, using cached", err)
		freshUpPrice = market.UpTokenPrice
		freshDownPrice = market.DownTokenPrice
	}

	// Use API prices for trading decision (orderbook asks are unreliable near settlement)
	// API outcomePrices reflect actual market prices, while asks can be skewed
	upTokenPrice := freshUpPrice
	downTokenPrice := freshDownPrice

	// Log prices for debugging
	log.Printf("[BTC] 💰 Market prices: UP=$%.4f, DOWN=$%.4f (sum=$%.4f)", upTokenPrice, downTokenPrice, upTokenPrice+downTokenPrice)

	// Check if we should trade based on token prices (low odds filter)
	shouldTrade, actualDirection, reason := s.predictor.ShouldTrade(prediction.Direction, upTokenPrice, downTokenPrice)
	if !shouldTrade {
		log.Printf("[BTC] ⚠️ Trade rejected: %s", reason)
		s.execWinMu.Lock()
		s.executedWindows[market.ID] = false
		s.execWinMu.Unlock()
		return
	}
	if actualDirection != prediction.Direction {
		log.Printf("[BTC] %s", reason)
	}

	// Determine which token to buy based on actualDirection
	var tokenID string
	var direction string
	if actualDirection == "up" {
		tokenID = market.UpTokenID
		direction = "UP"
	} else {
		tokenID = market.DownTokenID
		direction = "DOWN"
	}

	if tokenID == "" {
		log.Printf("[BTC] No token ID for %s direction", actualDirection)
		s.execWinMu.Lock()
		s.executedWindows[market.ID] = false
		s.execWinMu.Unlock()
		return
	}

	// Calculate position size using Kelly Criterion via risk manager
	var positionSize float64

	usdcBalance, err := s.execEngine.CheckUSDCBalance()
	if err != nil || usdcBalance <= 0 {
		log.Printf("[BTC] ⚠️ Failed to fetch USDC balance (err: %v), using fallback $50", err)
		usdcBalance = 50.0
	} else {
		log.Printf("[BTC] 💰 Real-time USDC balance: $%.2f", usdcBalance)
	}

	// Determine token price for sizing
	estimatedPrice := 0.50
	if actualDirection == "up" && upTokenPrice > 0.01 {
		estimatedPrice = upTokenPrice
	} else if actualDirection == "down" && downTokenPrice > 0.01 {
		estimatedPrice = downTokenPrice
	}

	// Use risk manager's Kelly Criterion for optimal position sizing
	positionSize = s.riskManager.CalculatePositionSize(prediction.Confidence, estimatedPrice, usdcBalance)

	// Apply time-of-day sizing multiplier
	timeMultiplier := s.getTimeSizeMultiplier()
	if timeMultiplier != 1.0 {
		positionSize *= timeMultiplier
		log.Printf("[BTC] 🕐 Time-of-day multiplier: %.1fx → %.2f shares", timeMultiplier, positionSize)
	}

	// Allow fixed trade amount override (e.g. POLY_TRADE_AMOUNT=10 for $10 per trade)
	if customAmt := os.Getenv("POLY_TRADE_AMOUNT"); customAmt != "" {
		if val, err := strconv.ParseFloat(customAmt, 64); err == nil && val > 0 {
			positionSize = val / estimatedPrice
			if positionSize < 5 {
				positionSize = 5
			}
		}
	}

	log.Printf("[BTC] 📐 Kelly sizing: %.2f shares (balance=$%.2f, confidence=%.1f%%, price=$%.4f)",
		positionSize, usdcBalance, prediction.Confidence*100, estimatedPrice)

	// === Orderbook-aware pricing with complementary matching ===
	// The CLOB shows implied prices from both sides (complementary matching).
	// For active markets, asks reflect real liquidity including cross-matched orders.
	//
	// Fee structure: Polymarket charges fee_rate_bps (typically 1000 = 10%) on min(price, 1-price)
	// - Taker (FOK): pays full fee → effective cost = price + 0.10 * min(price, 1-price)
	// - Maker (GTC): pays 0 fee → effective cost = price
	// EV > 0 when: confidence > effective_cost
	//
	// Calculate max profitable price for each order type:
	// FOK: solve price + 0.10 * min(price, 1-price) ≤ confidence
	//   For price ≤ 0.50: 1.10 * price ≤ confidence → price ≤ confidence / 1.10
	//   For price > 0.50: price + 0.10 * (1-price) ≤ confidence → 0.90*price ≤ confidence - 0.10 → price ≤ (confidence-0.10)/0.90
	// GTC (maker): price ≤ confidence
	maxFOKPrice := prediction.Confidence / 1.10 // Conservative: assumes price ≤ 0.50
	maxGTCPrice := prediction.Confidence - 0.01 // Small margin for GTC maker orders
	if maxGTCPrice < 0.10 {
		log.Printf("[BTC] ⚠️ Confidence too low for profitable trade: %.1f%% (max GTC $%.2f, FOK $%.2f)",
			prediction.Confidence*100, maxGTCPrice, maxFOKPrice)
		s.execWinMu.Lock()
		s.executedWindows[market.ID] = false
		s.execWinMu.Unlock()
		return
	}

	// Get the CLOB-computed buy price (uses complementary matching internally)
	// This is the actual executable price, even when visible book has only extreme prices.
	var clobBuyPrice float64
	if clobPrice, clobErr := s.client.GetCLOBPrice(tokenID, "buy"); clobErr == nil && clobPrice > 0.01 && clobPrice < 0.99 {
		clobBuyPrice = clobPrice
		log.Printf("[BTC] 📊 CLOB price endpoint: %s buy=$%.2f", direction, clobBuyPrice)
	} else {
		// Fallback: try Gamma API spread
		if spread, spreadErr := s.client.GetMarketSpread(slug); spreadErr == nil {
			if actualDirection == "up" {
				clobBuyPrice = spread.BestAsk
			} else {
				clobBuyPrice = 1 - spread.BestBid
			}
			log.Printf("[BTC] 📊 Gamma spread fallback: %s effective price=$%.2f", direction, clobBuyPrice)
		} else {
			// Last resort: use estimated outcome price
			clobBuyPrice = estimatedPrice
			log.Printf("[BTC] 📊 Using outcome price fallback: $%.2f", clobBuyPrice)
		}
	}

	// Determine order price — use GTC at CLOB price + 1¢ (cross spread for fill)
	var orderPrice float64
	var orderType string
	if clobBuyPrice > 0 && clobBuyPrice+0.01 <= maxGTCPrice {
		orderPrice = clobBuyPrice + 0.01
		orderType = "GTC"
		log.Printf("[BTC] 📊 CLOB $%.2f + $0.01 = $%.2f ≤ max $%.2f → GTC cross-spread",
			clobBuyPrice, orderPrice, maxGTCPrice)
	} else if clobBuyPrice > 0 && clobBuyPrice <= maxGTCPrice {
		orderPrice = clobBuyPrice
		orderType = "GTC"
		log.Printf("[BTC] 📊 CLOB $%.2f ≤ max $%.2f → GTC at CLOB price",
			clobBuyPrice, maxGTCPrice)
	} else if clobBuyPrice > 0 && clobBuyPrice <= maxGTCPrice+0.03 {
		orderPrice = maxGTCPrice
		orderType = "GTC"
		log.Printf("[BTC] 📊 CLOB $%.2f slightly above max $%.2f → GTC bid at max",
			clobBuyPrice, maxGTCPrice)
	} else if clobBuyPrice > 0 {
		// CLOB price too expensive — try opposite token
		oppositeConfidence := 1.0 - prediction.Confidence
		var oppositeTokenID string
		var oppositeDirection string
		originalDirection := direction
		if direction == "UP" {
			oppositeTokenID = market.DownTokenID
			oppositeDirection = "DOWN"
		} else {
			oppositeTokenID = market.UpTokenID
			oppositeDirection = "UP"
		}

		var oppClobPrice float64
		if oppPrice, oppErr := s.client.GetCLOBPrice(oppositeTokenID, "buy"); oppErr == nil && oppPrice > 0.01 && oppPrice < 0.99 {
			oppClobPrice = oppPrice
		}

		oppMaxGTCPrice := oppositeConfidence - 0.01
		// Minimum contrarian price to avoid dust orders that never fill
		minContrarianPrice := 0.05

		if oppClobPrice >= minContrarianPrice && oppClobPrice+0.01 <= oppMaxGTCPrice {
			tokenID = oppositeTokenID
			direction = oppositeDirection
			orderPrice = oppClobPrice + 0.01
			orderType = "GTC"
			log.Printf("[BTC] 🔄 Contrarian: %s CLOB $%.2f too expensive (max $%.2f), buying %s at $%.2f",
				originalDirection, clobBuyPrice, maxGTCPrice, oppositeDirection, orderPrice)
		} else if oppClobPrice >= minContrarianPrice && oppClobPrice <= oppMaxGTCPrice {
			tokenID = oppositeTokenID
			direction = oppositeDirection
			orderPrice = oppClobPrice
			orderType = "GTC"
			log.Printf("[BTC] 🔄 Contrarian: buying %s at CLOB $%.2f (max $%.2f)",
				oppositeDirection, orderPrice, oppMaxGTCPrice)
		} else {
			// Track price-skip retries to prevent log spam
			s.windowRetriesMu.Lock()
			s.windowRetries[market.ID]++
			priceRetries := s.windowRetries[market.ID]
			s.windowRetriesMu.Unlock()
			if priceRetries <= 1 {
				log.Printf("[BTC] ⚠️ Both tokens above max: %s CLOB $%.2f (max $%.2f), %s CLOB $%.2f (max $%.2f), will retry",
					originalDirection, clobBuyPrice, maxGTCPrice, oppositeDirection, oppClobPrice, oppMaxGTCPrice)
			}
			if priceRetries < 3 {
				s.execWinMu.Lock()
				s.executedWindows[market.ID] = false
				s.execWinMu.Unlock()
			}
			return
		}
	} else {
		// No price data at all — use outcome price + slippage
		slippage := s.config.PriceSlippage
		if slippage < 0.10 {
			slippage = 0.10
		}
		orderPrice = estimatedPrice * (1 + slippage)
		if orderPrice > maxGTCPrice {
			orderPrice = maxGTCPrice
		}
		orderType = "GTC"
		log.Printf("[BTC] 📊 No CLOB price → outcome $%.2f + slippage → GTC at $%.2f",
			estimatedPrice, orderPrice)
	}

	// Round to nearest 0.01 tick (Polymarket minimum tick size)
	orderPrice = math.Round(orderPrice*100) / 100
	if orderPrice > 0.99 {
		orderPrice = 0.99
	} else if orderPrice < 0.01 {
		orderPrice = 0.01
	}

	// Enforce Polymarket minimum order value ($1) with margin
	minOrderValue := 2.0 // $2 minimum to stay safely above $1 API limit
	orderValue := positionSize * orderPrice
	if orderValue < minOrderValue {
		positionSize = math.Ceil(minOrderValue / orderPrice)
		log.Printf("[BTC] 📐 Position adjusted: %.0f shares to meet $%.0f min order (was $%.2f)", positionSize, minOrderValue, orderValue)
	}

	// CRITICAL: Re-validate position size against MaxPositionUSD after final orderPrice
	// Kelly sizing uses estimatedPrice which may differ from actual CLOB orderPrice
	maxPositionUSD := s.riskManager.GetMaxPositionUSD()
	if positionSize*orderPrice > maxPositionUSD {
		oldSize := positionSize
		positionSize = math.Floor(maxPositionUSD / orderPrice)
		if positionSize < 5 {
			positionSize = 5
		}
		log.Printf("[BTC] ⚠️ Position capped: %.0f→%.0f shares (%.2f×$%.4f=$%.2f exceeded $%.0f limit)",
			oldSize, positionSize, oldSize, orderPrice, oldSize*orderPrice, maxPositionUSD)
	}

	log.Printf("[BTC] 🎯 TRADING SIGNAL")
	log.Printf("  Market: %s", market.Question)
	log.Printf("  Direction: %s", direction)
	log.Printf("  Confidence: %.1f%%", prediction.Confidence*100)
	log.Printf("  Price Change: %.3f%%", prediction.PriceChange)
	log.Printf("  Spot Price: $%.2f", prediction.SpotPrice)
	log.Printf("  Position: %.2f shares @ $%.4f (%s)", positionSize, orderPrice, orderType)

	// Simulation mode: log the trade signal but don't execute
	if os.Getenv("POLY_SIMULATION") == "1" || os.Getenv("POLY_SIMULATION") == "true" {
		log.Printf("[BTC] 🧪 SIMULATION: Would BUY %.2f shares of %s @ $%.4f ($%.2f total)",
			positionSize, direction, orderPrice, positionSize*orderPrice)

		// Record simulated position using OpenPosition for proper state tracking
		s.OpenPosition(market.ID, tokenID, direction, orderPrice, positionSize,
			market.WindowEnd, prediction.Direction, prediction.Confidence)
		// Store BTC start price for outcome determination after market is removed
		s.positionMu.Lock()
		if pos, ok := s.positions[market.ID]; ok {
			pos.BTCStartPrice = market.StartPrice
		}
		s.positionMu.Unlock()
		return
	}

	// Cancel any stale GTC orders first to free locked USDC
	if err := s.execEngine.CancelAllOrders(); err != nil {
		log.Printf("[BTC] ⚠️ Failed to cancel stale orders: %v", err)
	}
	orders := []execution.Order{
		{
			TokenID:   tokenID,
			Price:     orderPrice,
			Size:      positionSize,
			Side:      "BUY",
			OrderType: orderType,
		},
	}

	// Execute synchronously and wait for result
	result, err := s.execEngine.ExecuteBatch(fmt.Sprintf("btc-%s", market.ID), orders)
	if err != nil {
		log.Printf("[BTC] ❌ Order execution failed: %v", err)
		// Limit retries per window to prevent spam
		s.windowRetriesMu.Lock()
		s.windowRetries[market.ID]++
		retries := s.windowRetries[market.ID]
		s.windowRetriesMu.Unlock()
		if retries < 3 {
			log.Printf("[BTC] 🔄 Will retry (attempt %d/3)", retries)
			s.execWinMu.Lock()
			s.executedWindows[market.ID] = false
			s.execWinMu.Unlock()
		} else {
			log.Printf("[BTC] ⛔ Max retries reached for %s, giving up", market.ID)
		}
		return
	}

	// Check if order was actually filled
	if !result.Success {
		log.Printf("[BTC] ❌ Order failed: %s", result.ErrorMessage)
		s.windowRetriesMu.Lock()
		s.windowRetries[market.ID]++
		retries := s.windowRetries[market.ID]
		s.windowRetriesMu.Unlock()
		if retries < 3 {
			s.execWinMu.Lock()
			s.executedWindows[market.ID] = false
			s.execWinMu.Unlock()
		}
		return
	}

	// For GTC orders, "pending" means placed on book — track position optimistically
	// For FOK orders, "pending" would be unusual (FOK should be immediate)
	isPending := false
	if len(result.Orders) > 0 && result.Orders[0].Status == "pending" {
		if orderType == "FOK" {
			// FOK should not be pending — treat as failed, allow retry
			log.Printf("[BTC] ⚠️ FOK order returned pending status — treating as not filled, will retry")
			s.execWinMu.Lock()
			s.executedWindows[market.ID] = false
			s.execWinMu.Unlock()
			return
		}
		isPending = true
	}

	if result.TotalFilled <= 0 && !isPending {
		log.Printf("[BTC] ❌ Order not filled: success=%v, filled=%.2f, orders=%d, type=%s",
			result.Success, result.TotalFilled, len(result.Orders), orderType)
		if len(result.Orders) > 0 {
			log.Printf("[BTC]   Order[0] status=%s, filled=%.6f, size=%.2f",
				result.Orders[0].Status, result.Orders[0].FilledSize, result.Orders[0].Size)
		}
		// For FOK failures, allow retry on next tick (market may have moved)
		s.execWinMu.Lock()
		s.executedWindows[market.ID] = false
		s.execWinMu.Unlock()
		return
	}

	// Determine actual size and price
	actualSize := result.TotalFilled
	actualPrice := orderPrice
	if len(result.Orders) > 0 && result.Orders[0].AvgPrice > 0 {
		parsedPrice := result.Orders[0].AvgPrice
		// Sanity check: parsed avg price should be within 50% of order price
		if parsedPrice > orderPrice*0.5 && parsedPrice < orderPrice*2.0 {
			actualPrice = parsedPrice
		} else {
			log.Printf("[BTC] ⚠️ Parsed avg price $%.4f seems wrong vs order price $%.4f, using order price", parsedPrice, orderPrice)
		}
	}

	if isPending {
		// GTC order placed on book — use ordered size for position tracking
		// Balance verification will confirm actual fill later
		actualSize = positionSize
		log.Printf("[BTC] 📋 GTC order placed on book (%.2f shares @ $%.4f), tracking position", actualSize, actualPrice)
	} else if actualSize < 1.0 && positionSize >= 5.0 {
		// Sanity check: if parsed fill is implausibly small but we ordered >= 5 shares,
		// the takingAmount parsing is likely wrong — use ordered size
		log.Printf("[BTC] ⚠️ Parsed fill (%.6f) implausibly small vs ordered (%.2f), using ordered size", actualSize, positionSize)
		actualSize = positionSize
	} else {
		log.Printf("[BTC] ✅ Order filled: %.2f shares @ $%.4f", actualSize, actualPrice)
	}

	// Open position for tracking
	s.OpenPosition(market.ID, tokenID, direction, actualPrice, actualSize, market.WindowEnd, prediction.Trend, prediction.Confidence)
	// Store BTC start price for outcome determination
	s.positionMu.Lock()
	if pos, ok := s.positions[market.ID]; ok {
		pos.BTCStartPrice = market.StartPrice
	}
	s.positionMu.Unlock()

	s.notifier.Notify(notification.NewTradeExecutedEvent(market.ID, direction, actualSize, actualPrice, prediction.Confidence))

	// Record execution time using separate mutex to avoid deadlock
	s.execTimeMu.Lock()
	if s.lastExecTime == nil {
		s.lastExecTime = make(map[string]time.Time)
	}
	s.lastExecTime[market.ID] = time.Now()
	s.execTimeMu.Unlock()
}

// printStatus prints current status
func (s *BTCStrategy) printStatus() {
	s.mu.RLock()
	spotPrice := s.spotMonitor.GetCurrentPrice()
	trend, strength := s.spotMonitor.GetTrend(30)
	marketCount := len(s.markets)
	s.mu.RUnlock()

	// Get technical indicators
	history := s.spotMonitor.GetPriceHistory()
	var rsi, macdHist, bollingerPct float64
	if len(history) >= 20 {
		prices := make([]float64, len(history))
		for i, pp := range history {
			prices[i] = pp.Price
		}
		indicators := NewTechnicalIndicators()
		rsi = indicators.RSI(prices, 14)
		macd := indicators.MACD(prices)
		macdHist = macd.Histogram
		bollinger := indicators.Bollinger(prices, 20, 2.0)
		bollingerPct = bollinger.PercentB
	}

	// Get risk stats
	trades, pnl, drawdown := s.riskManager.GetDailyStats()

	log.Println("--- [BTC Strategy Status] ---")
	log.Printf("  Spot Price: $%.2f", spotPrice)
	log.Printf("  Trend (30s): %s (%.3f%%)", trend, strength)
	log.Printf("  RSI(14): %.1f | MACD Hist: %.2f | Bollinger %%B: %.1f%%", rsi, macdHist, bollingerPct)
	log.Printf("  Active Windows: %d", marketCount)

	s.mu.RLock()
	for _, m := range s.markets {
		remaining := time.Until(m.WindowEnd)
		if remaining > 0 {
			log.Printf("    - %s (ends in %v)", m.Question, remaining.Round(time.Second))
		}
	}
	s.mu.RUnlock()

	// Print active positions
	s.positionMu.RLock()
	activePositions := 0
	for _, pos := range s.positions {
		if pos.IsActive {
			activePositions++
			log.Printf("  📊 Position: %s @ $%.4f", pos.Side, pos.EntryPrice)
		}
	}
	s.positionMu.RUnlock()

	if activePositions > 0 {
		log.Printf("  Active Positions: %d", activePositions)
	}

	// Print risk stats
	log.Printf("  Daily: %d trades | PnL: $%.2f | Drawdown: %.1f%%", trades, pnl, drawdown)
	log.Printf("  Total: %d trades | Wins: %d | Losses %d | PnL: $%.2f",
		s.totalTrades, s.winningTrades, s.losingTrades, s.totalPnL)
	log.Println("-----------------------------")
}

// OpenPosition creates a new position
func (s *BTCStrategy) OpenPosition(marketID, tokenID, side string, entryPrice, size float64, windowEnd time.Time, trend string, confidence float64) {
	s.positionMu.Lock()

	position := &Position{
		MarketID:      marketID,
		TokenID:       tokenID,
		Side:          side,
		EntryPrice:    entryPrice,
		Size:          size,
		OpenTime:      time.Now(),
		IsActive:      true,
		WindowEnd:     windowEnd,
		OriginalTrend: trend,
		OriginalConf:  confidence,
		ExitAttempted: false,
	}

	s.positions[marketID] = position
	s.positionMu.Unlock()

	s.statsMu.Lock()
	s.totalTrades++
	s.statsMu.Unlock()

	log.Printf("[BTC] 📊 Opened position: %s %s @ $%.4f (%.0f shares) | Window ends in %v",
		side, marketID, entryPrice, size, time.Until(windowEnd).Round(time.Second))

	if err := s.savePositions(); err != nil {
		log.Printf("[BTC] Warning: Failed to save positions: %v", err)
	}
}

// GetPosition returns the current position for a market
func (s *BTCStrategy) GetPosition(marketID string) *Position {
	s.positionMu.RLock()
	defer s.positionMu.RUnlock()
	return s.positions[marketID]
}

// UpdateMarketTokenIDs updates token IDs for a market
func (s *BTCStrategy) UpdateMarketTokenIDs(marketID, upTokenID, downTokenID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if market, exists := s.markets[marketID]; exists {
		market.UpTokenID = upTokenID
		market.DownTokenID = downTokenID
	}
}

// savePositions persists positions to disk
func (s *BTCStrategy) savePositions() error {
	s.positionMu.RLock()
	data, err := json.MarshalIndent(s.positions, "", "  ")
	s.positionMu.RUnlock()
	return s.writePositionsData(data, err)
}

// savePositionsLocked 在调用者已持有 positionMu 时使用
func (s *BTCStrategy) savePositionsLocked() error {
	data, err := json.MarshalIndent(s.positions, "", "  ")
	return s.writePositionsData(data, err)
}

func (s *BTCStrategy) writePositionsData(data []byte, marshalErr error) error {
	if marshalErr != nil {
		return fmt.Errorf("failed to marshal positions: %v", marshalErr)
	}

	if err := os.MkdirAll("data", 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %v", err)
	}

	if err := os.WriteFile("data/positions.json", data, 0644); err != nil {
		return fmt.Errorf("failed to write positions file: %v", err)
	}

	return nil
}

// loadPositions loads positions from disk
func (s *BTCStrategy) loadPositions() error {
	data, err := os.ReadFile("data/positions.json")
	if err != nil {
		if os.IsNotExist(err) {
			log.Println("[BTC] No saved positions found, starting fresh")
			return nil
		}
		return fmt.Errorf("failed to read positions file: %v", err)
	}

	var positions map[string]*Position
	if err := json.Unmarshal(data, &positions); err != nil {
		return fmt.Errorf("failed to unmarshal positions: %v", err)
	}

	s.positionMu.Lock()
	s.positions = positions
	s.positionMu.Unlock()

	activeCount := 0
	for _, pos := range positions {
		if pos.IsActive {
			activeCount++
		}
	}

	log.Printf("[BTC] ✅ Loaded %d positions from disk (%d active)", len(positions), activeCount)

	if activeCount > 0 {
		log.Printf("[BTC] 🔍 Verifying active positions against actual balances...")
		if err := s.verifyPositions(); err != nil {
			log.Printf("[BTC] ⚠️  Balance verification failed: %v", err)
		}
	}

	return nil
}

// checkExitStrategy monitors active positions and executes exit when conditions are met
func (s *BTCStrategy) checkExitStrategy() {
	now := time.Now()

	s.positionMu.RLock()
	var activePositions []*Position
	for _, pos := range s.positions {
		if pos.IsActive && !pos.ExitAttempted {
			activePositions = append(activePositions, pos)
		}
	}
	s.positionMu.RUnlock()

	if len(activePositions) == 0 {
		return
	}

	// 每 500ms 刷新一次活跃持仓对应市场的实时价格（高频交易级别）
	if now.Sub(s.lastPriceRefresh) >= 500*time.Millisecond {
		s.lastPriceRefresh = now
				s.refreshActivePrices(activePositions)

		// Log real-time PnL for each active position (every 10s to avoid spam)
		if now.Sub(s.lastPnLLog) >= 10*time.Second {
			s.lastPnLLog = now
			for _, pos := range activePositions {
				s.mu.RLock()
				market, mExists := s.markets[pos.MarketID]
				s.mu.RUnlock()
				if !mExists {
					continue
				}
				var currentPrice float64
				if pos.Side == "UP" {
					currentPrice = market.UpTokenPrice
				} else {
					currentPrice = market.DownTokenPrice
				}
				if currentPrice > 0 {
					unrealizedPnL := (currentPrice - pos.EntryPrice) * pos.Size
					pctChange := (currentPrice - pos.EntryPrice) / pos.EntryPrice * 100
					timeLeft := time.Until(pos.WindowEnd)
					log.Printf("[BTC] 📊 Position %s | %s | Entry=$%.4f Now=$%.4f | PnL=$%.2f (%.1f%%) | %s left",
						pos.MarketID, pos.Side, pos.EntryPrice, currentPrice, unrealizedPnL, pctChange, timeLeft.Round(time.Second))
				}
			}
		}
	}

	for _, position := range activePositions {
		shouldExit, reason := s.shouldExitPosition(position, now)
		if shouldExit {
			if os.Getenv("POLY_SIMULATION") == "1" || os.Getenv("POLY_SIMULATION") == "true" {
				// Simulation: record exit without actual sell order
				s.mu.RLock()
				market, mExists := s.markets[position.MarketID]
				s.mu.RUnlock()
				var exitPrice float64
				if mExists {
					if position.Side == "UP" {
						exitPrice = market.UpTokenPrice
					} else {
						exitPrice = market.DownTokenPrice
					}
				}
				if exitPrice <= 0 {
					exitPrice = position.EntryPrice * 0.5
				}
				pnl := (exitPrice - position.EntryPrice) * position.Size
				log.Printf("[BTC] 🧪 SIMULATION EXIT: %s @ $%.4f | PnL=$%.2f | %s",
					position.MarketID, exitPrice, pnl, reason)
				s.recordTradeResult(position, exitPrice, false)
				s.mu.Lock()
				delete(s.positions, position.MarketID)
				s.mu.Unlock()
				s.savePositions()
			} else {
				s.executeExit(position, reason)
			}
		}
	}
}

// refreshActivePrices 拉取活跃持仓所对应市场的最新 token 价格
func (s *BTCStrategy) refreshActivePrices(positions []*Position) {
	refreshed := make(map[string]bool)
	for _, pos := range positions {
		if refreshed[pos.MarketID] {
			continue
		}
		refreshed[pos.MarketID] = true

		s.mu.RLock()
		market, exists := s.markets[pos.MarketID]
		s.mu.RUnlock()
		if !exists {
			continue
		}

		slug := fmt.Sprintf("btc-updown-5m-%d", market.WindowStart.Unix())
		upPrice, downPrice, err := s.client.GetMarketPrices(slug)
		if err != nil {
			log.Printf("[BTC] ⚠️ Exit price refresh failed for %s: %v", pos.MarketID, err)
			continue
		}

		s.mu.Lock()
		market.UpTokenPrice = upPrice
		market.DownTokenPrice = downPrice
		s.mu.Unlock()
	}
}

// shouldExitPosition determines if a position should be exited
func (s *BTCStrategy) shouldExitPosition(position *Position, now time.Time) (bool, string) {
	// Get current market info
	s.mu.RLock()
	market, marketExists := s.markets[position.MarketID]
	s.mu.RUnlock()

	if !marketExists {
		return false, ""
	}

	// Get current token price
	var currentPrice float64
	if position.Side == "UP" {
		currentPrice = market.UpTokenPrice
	} else {
		currentPrice = market.DownTokenPrice
	}

	// Calculate time remaining
	timeRemaining := time.Until(position.WindowEnd)
	positionAge := time.Since(position.OpenTime)

	// 1. Time-based forced exit (exit before window ends)
	// Skip if position is < 60s old (GTC orders need time to fill and settle)
	if timeRemaining <= s.config.ExitBeforeEnd && timeRemaining > 0 && positionAge > 60*time.Second {
		return true, fmt.Sprintf("time_exit (%.0fs before end)", timeRemaining.Seconds())
	}

	// 2. Check if window already ended
	if timeRemaining <= 0 {
		return false, "" // Let claim logic handle this
	}

	// Skip take profit/stop loss checks for positions < 30s old
	if positionAge < 30*time.Second {
		return false, ""
	}

	// 3. Trend reversal check
	if s.config.TrendReversalThreshold > 0 {
		currentTrend, trendStrength := s.spotMonitor.GetTrend(15)
		originalTrend := position.OriginalTrend

		// Check if trend has reversed
		if (originalTrend == "up" && currentTrend == "down") ||
			(originalTrend == "down" && currentTrend == "up") {
			if trendStrength >= s.config.TrendReversalThreshold {
				return true, fmt.Sprintf("trend_reversal (%s -> %s, strength=%.2f)",
					originalTrend, currentTrend, trendStrength)
			}
		}
	}

	// 4. Take profit check — tiered by entry price
	// Ultra-cheap tokens (<$0.10): hold to settlement for max payout (no take profit)
	// Cheap tokens ($0.10-$0.25): take profit at 100% gain
	// Other tokens: use configured take profit
	takeProfitPct := s.config.TakeProfitPct
	if customTakeProfit := os.Getenv("POLY_TAKE_PROFIT_PCT"); customTakeProfit != "" {
		if val, err := strconv.ParseFloat(customTakeProfit, 64); err == nil && val > 0 && val <= 5.0 {
			takeProfitPct = val
		}
	}

	// Tiered take profit based on entry price
	if position.EntryPrice < 0.10 {
		takeProfitPct = 0 // Hold ultra-cheap tokens to settlement
	} else if position.EntryPrice < 0.25 {
		takeProfitPct = 1.0 // 100% gain for cheap tokens
	}

	if takeProfitPct > 0 && currentPrice > 0 && position.EntryPrice > 0 {
		priceChange := (currentPrice - position.EntryPrice) / position.EntryPrice
		if priceChange >= takeProfitPct {
			return true, fmt.Sprintf("take_profit (+%.1f%%)", priceChange*100)
		}
	}

	// 5. Stop loss check — wider for cheap tokens to avoid premature exits
	stopLossPct := s.config.StopLossPct
	if customStopLoss := os.Getenv("POLY_STOP_LOSS_PCT"); customStopLoss != "" {
		if val, err := strconv.ParseFloat(customStopLoss, 64); err == nil && val > 0 && val <= 1.0 {
			stopLossPct = val
		}
	}

	// Wider stop loss for cheap tokens (they're volatile but high reward)
	if position.EntryPrice < 0.10 {
		stopLossPct = 0.50 // 50% stop loss for ultra-cheap (max loss is small anyway)
	} else if position.EntryPrice < 0.25 {
		stopLossPct = 0.35 // 35% stop loss for cheap tokens
	}
	if stopLossPct > 0 && currentPrice > 0 && position.EntryPrice > 0 {
		priceChange := (currentPrice - position.EntryPrice) / position.EntryPrice
		if priceChange <= -stopLossPct {
			return true, fmt.Sprintf("stop_loss (%.1f%%)", priceChange*100)
		}
	}

	return false, ""
}

// executeExit executes an exit order for a position
func (s *BTCStrategy) executeExit(position *Position, reason string) {
	// Verify we actually hold the tokens before trying to sell
	balances, err := s.execEngine.CheckBalances([]string{position.TokenID})
	if err != nil {
		log.Printf("[BTC] ⚠️ Exit skipped for %s: balance check failed: %v", position.MarketID, err)
		return
	}
	balance, ok := balances[position.TokenID]
	if !ok || balance < 4.0 {
		log.Printf("[BTC] ⚠️ Exit skipped for %s: insufficient balance (%.4f)", position.MarketID, balance)
		return
	}

	s.positionMu.Lock()
	position.ExitAttempted = true
	// Sync size with actual balance
	if balance > position.Size {
		position.Size = balance
	}
	s.positionMu.Unlock()

	log.Printf("[BTC] 🚪 EXIT TRIGGER: %s | Reason: %s", position.MarketID, reason)

	// Get current market price for the token
	s.mu.RLock()
	market, marketExists := s.markets[position.MarketID]
	s.mu.RUnlock()

	var sellPrice float64
	if marketExists {
		if position.Side == "UP" {
			sellPrice = market.UpTokenPrice // Sell the UP token we own
		} else {
			sellPrice = market.DownTokenPrice // Sell the DOWN token we own
		}
	}

	// Ensure we have a valid sell price
	if sellPrice <= 0.01 {
		sellPrice = position.EntryPrice * 0.85 // Fallback: 15% below entry
	}
	if sellPrice > 0.99 {
		sellPrice = 0.99
	}
	if sellPrice < 0.01 {
		sellPrice = 0.01
	}

	// Apply slippage for faster fill
	sellPrice = sellPrice * (1 - s.config.PriceSlippage)
	if sellPrice < 0.01 {
		sellPrice = 0.01
	}

	log.Printf("[BTC] 📉 Selling %.2f shares @ $%.4f (entry was $%.4f)",
		position.Size, sellPrice, position.EntryPrice)

	// Query actual token balance to avoid "not enough balance" errors
	// (entry size may differ from actual holdings due to fees/partial fills)
	actualSize := position.Size
	if balances, err := s.execEngine.CheckBalances([]string{position.TokenID}); err == nil {
		if bal, ok := balances[position.TokenID]; ok && bal > 0 {
			if bal < actualSize {
				log.Printf("[BTC] 📊 Actual token balance %.4f < position size %.2f, using actual", bal, actualSize)
				actualSize = math.Floor(bal*100) / 100 // Round down to avoid over-selling
			}
		}
	}
	if actualSize < 1 {
		log.Printf("[BTC] ⚠️ Token balance too small (%.4f), letting claim handle it", actualSize)
		return
	}

	orders := []execution.Order{
		{
			TokenID:   position.TokenID,
			Price:     sellPrice,
			Size:      actualSize,
			Side:      "SELL",
			OrderType: "FOK", // Use FOK for exit to avoid orders stuck on book
		},
	}

	result, err := s.execEngine.ExecuteBatch(fmt.Sprintf("exit-%s", position.MarketID), orders)

	if err != nil || !result.Success || result.TotalFilled <= 0 {
		// FOK failed, retry with wider spread GTC as fallback
		log.Printf("[BTC] ⚠️ FOK exit failed, retrying with GTC at deeper discount...")
		gtcPrice := sellPrice * 0.90 // Additional 10% discount
		if gtcPrice < 0.01 {
			gtcPrice = 0.01
		}
		gtcOrders := []execution.Order{
			{
				TokenID:   position.TokenID,
				Price:     gtcPrice,
				Size:      actualSize,
				Side:      "SELL",
				OrderType: "GTC",
			},
		}
		result, err = s.execEngine.ExecuteBatch(fmt.Sprintf("exit-gtc-%s", position.MarketID), gtcOrders)
	}

	if err != nil || !result.Success {
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		} else {
			errMsg = result.ErrorMessage
		}
		log.Printf("[BTC] ❌ Exit failed for %s: %s", position.MarketID, errMsg)
		// Allow limited retries (max 3)
		s.positionMu.Lock()
		position.ExitRetries++
		if position.ExitRetries < 3 {
			position.ExitAttempted = false
		} else {
			log.Printf("[BTC] ⚠️ Exit exhausted retries for %s, letting claim handle it", position.MarketID)
		}
		s.positionMu.Unlock()
		return
	}

	// Calculate actual exit price
	exitPrice := sellPrice
	if len(result.Orders) > 0 && result.Orders[0].AvgPrice > 0 {
		exitPrice = result.Orders[0].AvgPrice
	}

	// Sanity check: binary token prices must be in [0, 1]
	if exitPrice > 1.0 {
		log.Printf("[BTC] ⚠️ Exit price $%.4f exceeds $1.0 (possible price inversion), clamping", exitPrice)
		exitPrice = 1.0
	}
	if exitPrice < 0.0 {
		exitPrice = 0.0
	}

	s.positionMu.Lock()
	position.IsActive = false
	position.CloseReason = reason
	s.positionMu.Unlock()

	pnl := (exitPrice - position.EntryPrice) * position.Size
	log.Printf("[BTC] ✅ Exit successful: %s @ $%.4f | PnL: $%.2f | Reason: %s",
		position.MarketID, exitPrice, pnl, reason)

	s.recordTradeResult(position, exitPrice, true)
	s.notifier.Notify(notification.NewTradeSettledEvent(position.MarketID, pnl, pnl > 0))

	if err := s.savePositions(); err != nil {
		log.Printf("[BTC] Warning: Failed to save positions: %v", err)
	}
}

// claimLoop periodically checks for settled markets and claims winnings
func (s *BTCStrategy) claimLoop() {
	ticker := time.NewTicker(2 * time.Minute) // Check every 2min to catch 5min windows promptly
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.checkAndClaim()
		}
	}
}

// checkAndClaim checks for settled markets with positions and claims winnings
func (s *BTCStrategy) checkAndClaim() {
	now := time.Now()

	log.Printf("[BTC] 🔍 Running claim check and balance verification...")

	if err := s.verifyPositions(); err != nil {
		log.Printf("[BTC] ⚠️  Balance verification failed: %v", err)
	}

	// TRIGGER REAL AUTO-REDEEM FOR SETTLED REWARDS TO USDC
	if result, err := s.execEngine.AutoRedeem(); err != nil {
		if strings.Contains(err.Error(), "insufficient MATIC") || strings.Contains(err.Error(), "insufficient funds") {
			log.Printf("[BTC] ⚠️  Auto_redeem: MATIC 余额不足，无法支付 gas 费用。请向 EOA 钱包充值 MATIC")
		} else {
			log.Printf("[BTC] ⚠️  Auto_redeem failed: %v", err)
		}
	} else if result != nil && result.TotalClaimed > 0 {
		log.Printf("[BTC] ✅ Auto_redeem: 成功领取 %d 个已结算仓位", result.TotalClaimed)
	}

	// Collect positions to claim under read lock
	s.positionMu.RLock()
	type claimTarget struct {
		marketID string
		tokenID  string
		size     float64
	}
	var targets []claimTarget
	for marketID, position := range s.positions {
		if !position.IsActive {
			continue
		}
		// Claim eligible: window must have ended + 90s buffer for settlement
		if now.After(position.WindowEnd.Add(90 * time.Second)) {
			targets = append(targets, claimTarget{
				marketID: marketID,
				tokenID:  position.TokenID,
				size:     position.Size,
			})
		}
	}
	s.positionMu.RUnlock()

	if len(targets) == 0 {
		return
	}

	isSimulation := os.Getenv("POLY_SIMULATION") == "1" || os.Getenv("POLY_SIMULATION") == "true"

	// In simulation mode, determine outcome from market data instead of executing
	if isSimulation {
		s.positionMu.Lock()
		defer s.positionMu.Unlock()

		updated := false
		for _, t := range targets {
			position, exists := s.positions[t.marketID]
			if !exists || !position.IsActive {
				continue
			}

			exitPrice, won := s.determineMarketOutcome(position)
			position.IsActive = false
			position.CloseReason = "simulation_settled"
			pnl := (exitPrice - position.EntryPrice) * position.Size

			if won {
				log.Printf("[BTC] 🧪 SIMULATION SETTLED: %s %s WON | Entry=$%.4f Exit=$%.2f | PnL=$%.2f",
					position.Side, t.marketID, position.EntryPrice, exitPrice, pnl)
			} else {
				log.Printf("[BTC] 🧪 SIMULATION SETTLED: %s %s LOST | Entry=$%.4f Exit=$%.2f | PnL=$%.2f",
					position.Side, t.marketID, position.EntryPrice, exitPrice, pnl)
			}
			s.recordTradeResult(position, exitPrice, won)
			updated = true
		}

		if updated {
			if err := s.savePositionsLocked(); err != nil {
				log.Printf("[BTC] Warning: Failed to save positions: %v", err)
			}
		}
		return
	}

	// For 5-minute binary markets, orderbooks are deleted immediately after settlement.
	// No point trying to sell on orderbook — just auto_redeem and determine outcome.
	s.positionMu.Lock()
	defer s.positionMu.Unlock()

	updated := false
	for _, t := range targets {
		position, exists := s.positions[t.marketID]
		if !exists || !position.IsActive {
			continue
		}

		position.IsActive = false
		position.CloseReason = "market_settled"
		updated = true

		exitPrice, won := s.determineMarketOutcome(position)
		if won {
			log.Printf("[BTC] ✅ Market %s settled in our favor (exit=$%.2f)", t.marketID, exitPrice)
		} else {
			log.Printf("[BTC] ❌ Market %s settled against us (exit=$%.2f)", t.marketID, exitPrice)
		}
		s.recordTradeResult(position, exitPrice, won)
		s.performanceTracker.RecordClaimAttempt(won)

		if won {
			s.notifier.Notify(notification.NewClaimSuccessEvent(t.marketID, position.Size*exitPrice))
		}
	}

	if updated {
		if err := s.savePositionsLocked(); err != nil {
			log.Printf("[BTC] Warning: Failed to save positions: %v", err)
		}
	}
}

// verifyPositions checks actual token balances and marks positions as inactive if tokens are gone
// Uses split-lock pattern: read lock to collect IDs, remote call without lock, write lock to update
func (s *BTCStrategy) verifyPositions() error {
	// Skip balance verification in simulation mode — no real tokens to check
	if os.Getenv("POLY_SIMULATION") == "1" || os.Getenv("POLY_SIMULATION") == "true" {
		return nil
	}

	// Phase 1: collect token IDs under read lock
	s.positionMu.RLock()
	var tokenIDs []string
	type posSnapshot struct {
		marketID string
		tokenID  string
	}
	var snapshots []posSnapshot
	now := time.Now()
	for marketID, pos := range s.positions {
		if pos.IsActive {
			// Skip positions opened less than 60 seconds ago — GTC orders need time to fill
			if now.Sub(pos.OpenTime) < 60*time.Second {
				continue
			}
			tokenIDs = append(tokenIDs, pos.TokenID)
			snapshots = append(snapshots, posSnapshot{marketID: marketID, tokenID: pos.TokenID})
		}
	}
	s.positionMu.RUnlock()

	if len(tokenIDs) == 0 {
		return nil
	}

	// Phase 2: remote call WITHOUT holding any lock
	balances, err := s.execEngine.CheckBalances(tokenIDs)
	if err != nil {
		return fmt.Errorf("failed to check balances: %v", err)
	}

	// Phase 3: update positions under write lock
	s.positionMu.Lock()
	defer s.positionMu.Unlock()

	updated := false
	for _, snap := range snapshots {
		pos, exists := s.positions[snap.marketID]
		if !exists || !pos.IsActive {
			continue
		}
		balance, ok := balances[snap.tokenID]
		if !ok || balance < 0.01 {
			log.Printf("[BTC] ❌ Position %s has zero balance (expected %.2f), cancelling stale orders and marking inactive", snap.marketID, pos.Size)
			// Cancel any unfilled GTC orders for this market
			if cancelErr := s.execEngine.CancelAllOrders(); cancelErr != nil {
				log.Printf("[BTC] ⚠️ Failed to cancel stale orders: %v", cancelErr)
			}
			pos.IsActive = false
			pos.CloseReason = "balance_verification_failed"
			updated = true

			// Zero balance = tokens not held. Cannot determine actual outcome without tokens.
			// Record as loss of entry cost (conservative accounting).
			// If order was never filled, USDC was refunded so real impact is ~0,
			// but recording as loss keeps PnL conservative.
			s.recordTradeResult(pos, 0.0, false)
			log.Printf("[BTC] 📉 Position %s balance verification failed, recording as loss: PnL=$%.2f", snap.marketID, -pos.EntryPrice*pos.Size)
		} else {
			// Sync tracked size with actual on-chain balance
			if pos.Size < 1.0 && balance >= 1.0 {
				log.Printf("[BTC] 🔧 Position %s size corrected: %.6f → %.2f (from on-chain balance)", snap.marketID, pos.Size, balance)
				pos.Size = balance
				updated = true
			}
			log.Printf("[BTC] ✅ Position %s verified: balance %.2f", snap.marketID, balance)
		}
	}

	if updated {
		if err := s.savePositionsLocked(); err != nil {
			log.Printf("[BTC] ⚠️  Failed to save updated positions: %v", err)
		} else {
			log.Printf("[BTC] 💾 Updated positions saved to disk")
		}
	}

	return nil
}

// determineMarketOutcome checks if a position won or lost by querying the token price.
// For settled binary markets, winning tokens resolve to ~$1.0, losing tokens to ~$0.0.
func (s *BTCStrategy) determineMarketOutcome(position *Position) (exitPrice float64, won bool) {
	// Try to get the resolved token price from the CLOB API
	price, err := s.client.GetPrice(position.TokenID)
	if err == nil {
		if price >= 0.90 {
			return 1.0, true // Our token won
		} else if price <= 0.10 {
			return 0.0, false // Our token lost
		}
		// Price in between — use actual price
		return price, price > position.EntryPrice
	}
	log.Printf("[BTC] ⚠️ Could not query token price for %s: %v, using spot price fallback", position.MarketID, err)

	// Fallback: use BTC spot price to determine direction
	spotPrice := s.spotMonitor.GetCurrentPrice()

	// First try position's stored BTC start price (persists after market removal)
	if position.BTCStartPrice > 0 && spotPrice > 0 {
		btcWentUp := spotPrice > position.BTCStartPrice
		if (position.Side == "UP" && btcWentUp) || (position.Side == "DOWN" && !btcWentUp) {
			return 1.0, true
		}
		return 0.0, false
	}

	// Then try market's start price (may be gone if market was cleaned up)
	s.mu.RLock()
	market, exists := s.markets[position.MarketID]
	s.mu.RUnlock()

	if exists && market.StartPrice > 0 && spotPrice > 0 {
		btcWentUp := spotPrice > market.StartPrice
		if (position.Side == "UP" && btcWentUp) || (position.Side == "DOWN" && !btcWentUp) {
			return 1.0, true
		}
		return 0.0, false
	}

	// Last resort: assume loss (safer than assuming win)
	log.Printf("[BTC] ⚠️ Cannot determine outcome for %s, recording as loss (safe default)", position.MarketID)
	return 0.0, false
}

// recordTradeResult records a trade result to both stats and performance tracker
func (s *BTCStrategy) recordTradeResult(position *Position, exitPrice float64, claimSuccess bool) {
	// Validate exitPrice bounds — binary market tokens are always 0.0 to 1.0
	if exitPrice > 1.0 {
		log.Printf("[BTC] ⚠️ exitPrice %.4f exceeds 1.0, clamping to 1.0", exitPrice)
		exitPrice = 1.0
	}
	if exitPrice < 0.0 {
		exitPrice = 0.0
	}

	pnl := (exitPrice - position.EntryPrice) * position.Size

	s.statsMu.Lock()
	if pnl > 0 {
		s.winningTrades++
	} else if pnl < 0 {
		s.losingTrades++
	}
	s.totalPnL += pnl
	s.statsMu.Unlock()

	tradeResult := TradeResult{
		Timestamp:  time.Now(),
		MarketID:   position.MarketID,
		Direction:  position.Side,
		EntryPrice: position.EntryPrice,
		ExitPrice:  exitPrice,
		Size:       position.Size,
		PnL:        pnl,
		Confidence: position.OriginalConf,
		OpenTime:   position.OpenTime,
		CloseTime:  time.Now(),
		Success:    pnl > 0,
	}
	s.performanceTracker.RecordTrade(tradeResult)

	log.Printf("[BTC] 📈 Trade result: %s PnL=$%.2f (entry=$%.4f, exit=$%.4f)",
		position.MarketID, pnl, position.EntryPrice, exitPrice)

	s.notifier.Notify(notification.NewTradeSettledEvent(position.MarketID, pnl, pnl > 0))
}

// GetPerformanceStats returns current performance statistics
func (s *BTCStrategy) GetPerformanceStats() PerformanceStats {
	return s.performanceTracker.GetStats()
}

// GetNotifier returns the notifier instance
func (s *BTCStrategy) GetNotifier() *notification.Notifier {
	return s.notifier
}
