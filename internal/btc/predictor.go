package btc

import (
	"fmt"
	"sync"
	"time"
)

// PredictorConfig holds prediction configuration
// Optimized based on oracle-lag arbitrage research:
// - 0.07% price movement threshold (proven 61% win rate strategy)
// - Max token price 0.62 (buy low odds only)
// - >5 minutes remaining for entry timing
type PredictorConfig struct {
	WindowSeconds     int     // How many seconds before window end to predict
	MinConfidence     float64 // Minimum confidence to trade (0-1)
	MinPriceChangePct float64 // Minimum price change percentage (0.07% based on research)
	MaxTokenPrice     float64 // Maximum token price to buy (0.62 based on research)
	TrendLookbackSecs int     // Seconds to look back for trend analysis
}

// DefaultPredictorConfig returns default config optimized for oracle-lag strategy
// Based on oracle-lag-sniper research achieving 61.4% win rate:
// - Entry: price moved >=0.07%, >5min remaining, token <=$0.35
// - Exploits ~55 second lag between Chainlink updates and Polymarket repricing
func DefaultPredictorConfig() PredictorConfig {
	return PredictorConfig{
		WindowSeconds:     45,   // Predict 45 seconds before window end (match strategy PredictBeforeEnd)
		MinConfidence:     0.52, // 52% confidence threshold (EV+ with GTC maker orders)
		MinPriceChangePct: 0.01, // 0.01% minimum change (~$7 for BTC at $70K; confidence threshold filters noise)
		MaxTokenPrice:     0.48, // Live test: accept tokens <$0.48 for trade frequency
		TrendLookbackSecs: 15,   // Look back 15 seconds for trend
	}
}

// MarketPredictor predicts the outcome of a BTC Up/Down market
// Uses actual Chainlink Data Streams prices (via Polymarket RTDS) for settlement prediction
type MarketPredictor struct {
	spotMonitor      *SpotMonitor
	chainlinkMonitor *ChainlinkMonitor
	config           PredictorConfig
	mu               sync.RWMutex
}

// Prediction represents a market prediction
type Prediction struct {
	Direction      string  // "up" or "down"
	Confidence     float64 // 0-1
	PriceChange    float64 // Percentage change
	Timestamp      time.Time
	SpotPrice      float64
	ChainlinkPrice float64 // Actual Chainlink price used for prediction
	Trend          string  // "up", "down", or "neutral"
	TrendStrength  float64 // 0-1
	Reason         string  // Why this prediction was made
	OracleLagMs    int64   // Estimated oracle lag in milliseconds
}

// NewMarketPredictor creates a new market predictor
func NewMarketPredictor(spot *SpotMonitor, chainlink *ChainlinkMonitor, config PredictorConfig) *MarketPredictor {
	return &MarketPredictor{
		spotMonitor:      spot,
		chainlinkMonitor: chainlink,
		config:           config,
	}
}

// PredictAtWindowEnd predicts the outcome using Chainlink oracle price analysis
// CRITICAL: Polymarket BTC Up/Down markets settle based on Chainlink Data Streams price
// UP: Chainlink end >= Chainlink start
// DOWN: Chainlink end < Chainlink start
//
// Oracle-lag strategy insights:
// - Chainlink updates every ~55 seconds on average
// - Polymarket reprices with delay, creating arbitrage window
// - Entry conditions: price moved >=0.07%, >5min remaining, token <=$0.62
func (p *MarketPredictor) PredictAtWindowEnd(windowStart time.Time, windowDuration time.Duration) *Prediction {
	now := time.Now()
	windowEnd := windowStart.Add(windowDuration)

	timeRemaining := time.Until(windowEnd)

	// Predict in the last N seconds of the window
	if timeRemaining > time.Duration(p.config.WindowSeconds)*time.Second {
		return nil
	}
	if timeRemaining < 500*time.Millisecond {
		return nil // Too late to trade
	}

	// Check if we have Chainlink price data
	if p.chainlinkMonitor == nil {
		fmt.Printf("[PREDICTOR] Skipping: ChainlinkMonitor not initialized\n")
		return nil
	}

	chainlinkHistory := p.chainlinkMonitor.GetPriceHistory()
	if len(chainlinkHistory) < 5 {
		fmt.Printf("[PREDICTOR] Skipping: insufficient Chainlink history (%d points)\n", len(chainlinkHistory))
		return nil
	}

	// Get Chainlink price at window start (settlement comparison point)
	chainlinkStartPrice := p.chainlinkMonitor.GetPriceAt(windowStart)
	if chainlinkStartPrice == 0 {
		chainlinkStartPrice = chainlinkHistory[0].Price
		fmt.Printf("[PREDICTOR] Warning: Using earliest Chainlink price: %.2f\n", chainlinkStartPrice)
	}

	// Get current Chainlink price (from actual Chainlink Data Streams via RTDS)
	currentChainlinkPrice := chainlinkHistory[len(chainlinkHistory)-1].Price

	// Calculate Chainlink window change (determines settlement!)
	var chainlinkWindowChangePct float64
	if chainlinkStartPrice != 0 {
		chainlinkWindowChangePct = (currentChainlinkPrice - chainlinkStartPrice) / chainlinkStartPrice * 100
	}

	// Apply minimum price change filter (0.07% from research)
	absChange := abs(chainlinkWindowChangePct)
	if absChange < p.config.MinPriceChangePct {
		fmt.Printf("[PREDICTOR] Skipping: price change %.4f%% < threshold %.2f%%\n",
			absChange, p.config.MinPriceChangePct)
		return nil
	}

	// Get Chainlink trend for prediction
	chainlinkTrend, chainlinkTrendStrength := p.chainlinkMonitor.GetTrend(p.config.TrendLookbackSecs)

	// Predict expected Chainlink price at window end
	expectedChainlinkPrice, baseConfidence := p.chainlinkMonitor.CalculateExpectedSettlement(timeRemaining)

	// Get spot price data for additional confirmation
	spotHistory := p.spotMonitor.GetPriceHistory()
	var spotPrice float64
	var spotTrend string
	var spotTrendStrength float64
	if len(spotHistory) > 0 {
		spotPrice = spotHistory[len(spotHistory)-1].Price
		spotTrend, spotTrendStrength = p.spotMonitor.GetTrend(p.config.TrendLookbackSecs)
	}

	// Determine direction based on predicted Chainlink settlement
	var direction string
	var confidence float64

	if expectedChainlinkPrice > 0 && chainlinkStartPrice > 0 {
		if expectedChainlinkPrice >= chainlinkStartPrice {
			direction = "up"
		} else {
			direction = "down"
		}
		confidence = baseConfidence
	} else {
		direction = chainlinkTrend
		confidence = 0.5 + (chainlinkTrendStrength * 0.2)
	}

	// Enhanced confidence calculation based on oracle-lag research
	// Stronger trends and larger price moves = higher confidence
	confidence = p.calculateEnhancedConfidence(
		chainlinkWindowChangePct,
		chainlinkTrend,
		chainlinkTrendStrength,
		spotTrend,
		spotTrendStrength,
		confidence,
	)

	// Adjust confidence based on agreement between Chainlink and spot trends
	if spotTrend == chainlinkTrend && spotTrendStrength > 0.3 && chainlinkTrendStrength > 0.3 {
		confidence = min(confidence+0.05, 0.90)
	} else if spotTrend != chainlinkTrend && spotTrendStrength > 0.5 && chainlinkTrendStrength > 0.5 {
		confidence = max(confidence-0.15, 0.3)
	}

	// Get oracle lag estimate
	oracleLag := p.chainlinkMonitor.GetOracleLagEstimate()

	// Build reason string
	reason := fmt.Sprintf("Chainlink: start=%.2f current=%.2f change=%.4f%% trend=%s strength=%.2f | Spot: trend=%s strength=%.2f | OracleLag: %v",
		chainlinkStartPrice, currentChainlinkPrice, chainlinkWindowChangePct,
		chainlinkTrend, chainlinkTrendStrength, spotTrend, spotTrendStrength, oracleLag)

	// Log prediction details
	fmt.Printf("[PREDICTOR] Prediction: direction=%s confidence=%.2f change=%.4f%%\n",
		direction, confidence, chainlinkWindowChangePct)

	// Only return if confidence meets threshold
	if confidence < p.config.MinConfidence {
		return nil
	}

	return &Prediction{
		Direction:      direction,
		Confidence:     confidence,
		PriceChange:    chainlinkWindowChangePct,
		Timestamp:      now,
		SpotPrice:      spotPrice,
		ChainlinkPrice: currentChainlinkPrice,
		Trend:          chainlinkTrend,
		TrendStrength:  chainlinkTrendStrength,
		Reason:         reason,
		OracleLagMs:    oracleLag.Milliseconds(),
	}
}

// calculateEnhancedConfidence calculates confidence using oracle-lag strategy patterns
// Based on research showing 61.4% win rate with these factors:
// - Price movement magnitude (>=0.07% threshold)
// - Trend strength and consistency
// - Chainlink vs spot agreement
// Total bonus capped at +0.20 above base to prevent over-trading
func (p *MarketPredictor) calculateEnhancedConfidence(
	windowChange float64,
	chainlinkTrend string,
	chainlinkTrendStrength float64,
	spotTrend string,
	spotTrendStrength float64,
	baseConfidence float64,
) float64 {
	confidence := baseConfidence
	bonus := 0.0

	// Price movement magnitude bonus (key factor from research)
	absChange := abs(windowChange)
	if absChange >= 0.15 {
		bonus += 0.10 // Strong movement
	} else if absChange >= 0.10 {
		bonus += 0.07 // Good movement
	} else if absChange >= 0.07 {
		bonus += 0.04 // Research threshold met
	} else if absChange >= 0.04 {
		bonus += 0.02 // Moderate movement — still directionally informative
	} else if absChange >= 0.02 {
		bonus += 0.01 // Small but detectable movement
	}

	// Trend strength bonus
	if chainlinkTrendStrength > 0.7 {
		bonus += 0.06
	} else if chainlinkTrendStrength > 0.5 {
		bonus += 0.03
	}

	// Agreement bonus when both Chainlink and spot agree
	if chainlinkTrend == spotTrend && chainlinkTrendStrength > 0.3 && spotTrendStrength > 0.3 {
		bonus += 0.05
	}

	// Cap total bonus at +0.20 to prevent confidence inflation
	if bonus > 0.20 {
		bonus = 0.20
	}
	confidence += bonus

	// Cap confidence
	if confidence > 0.90 {
		confidence = 0.90
	}

	return confidence
}

// determineDirection determines trade direction from multiple signals
func (p *MarketPredictor) determineDirection(windowChange, shortTermChange float64, trend string, trendStrength float64) string {
	upScore := 0.0
	downScore := 0.0

	// Window change signal (CRITICAL: defines if we win or lose)
	if windowChange > 0.005 {
		upScore += 4.0
	} else if windowChange < -0.005 {
		downScore += 4.0
	}

	// Short-term momentum signal
	if shortTermChange > 0.005 {
		upScore += 1.5
	} else if shortTermChange < -0.005 {
		downScore += 1.5
	}

	// Trend signal
	if trend == "up" && trendStrength > 0.3 {
		upScore += 1.0 * trendStrength
	} else if trend == "down" && trendStrength > 0.3 {
		downScore += 1.0 * trendStrength
	}

	if upScore > downScore {
		return "up"
	} else if downScore > upScore {
		return "down"
	}

	// Default to window change direction
	if windowChange >= 0 {
		return "up"
	}
	return "down"
}

// calculateConfidence calculates confidence score (legacy method, kept for compatibility)
func (p *MarketPredictor) calculateConfidence(windowChange, shortTermChange float64, trend string, trendStrength float64) float64 {
	confidence := 0.5 // Base confidence

	// Window change contribution
	absWindowChange := abs(windowChange)
	if absWindowChange > 0.05 {
		confidence += 0.15
	} else if absWindowChange > 0.02 {
		confidence += 0.10
	} else if absWindowChange > 0.01 {
		confidence += 0.05
	}

	// Short-term momentum contribution
	absShortTerm := abs(shortTermChange)
	if absShortTerm > 0.02 {
		confidence += 0.20
	} else if absShortTerm > 0.01 {
		confidence += 0.15
	} else if absShortTerm > 0.005 {
		confidence += 0.10
	}

	// Trend alignment contribution
	if trendStrength > 0.5 {
		confidence += 0.10
	} else if trendStrength > 0.3 {
		confidence += 0.05
	}

	// Cap confidence
	if confidence > 0.90 {
		confidence = 0.90
	}

	return confidence
}

// buildReason creates a human-readable reason string
func (p *MarketPredictor) buildReason(windowChange, shortTermChange float64, trend string, trendStrength float64, confidence float64) string {
	reason := ""

	if windowChange > 0.01 {
		reason += "window_up "
	} else if windowChange < -0.01 {
		reason += "window_down "
	}

	if shortTermChange > 0.005 {
		reason += "momentum_up "
	} else if shortTermChange < -0.005 {
		reason += "momentum_down "
	}

	if trend != "neutral" && trendStrength > 0.3 {
		reason += "trend_" + trend
	}

	return reason
}

// ShouldTrade checks if we should trade based on token price and expected value
// Returns: (shouldTrade, actualDirection, reason)
func (p *MarketPredictor) ShouldTrade(direction string, upTokenPrice, downTokenPrice float64) (bool, string, string) {
	// Check if market is already settled (both prices at extremes)
	if (upTokenPrice > 0.98 && downTokenPrice > 0.98) || (upTokenPrice < 0.02 && downTokenPrice < 0.02) {
		return false, direction, fmt.Sprintf("Market appears settled: UP=$%.4f, DOWN=$%.4f", upTokenPrice, downTokenPrice)
	}

	// Normal case: check predicted direction's token price
	var tokenPrice float64
	var tokenName string

	if direction == "up" {
		tokenPrice = upTokenPrice
		tokenName = "UP"
	} else {
		tokenPrice = downTokenPrice
		tokenName = "DOWN"
	}

	// If the predicted winning side is cheap enough, trade it directly
	if tokenPrice <= p.config.MaxTokenPrice && tokenPrice >= 0.02 {
		return true, direction, fmt.Sprintf("Buying %s at $%.4f", tokenName, tokenPrice)
	}

	if tokenPrice < 0.02 {
		return false, direction, fmt.Sprintf("%s token price too low ($%.4f) - market may be settled", tokenName, tokenPrice)
	}

	// Predicted side is too expensive → consider buying the CHEAP opposite side
	// This is a contrarian play: market overestimates the predicted direction
	var oppositePrice float64
	var oppositeName, oppositeDir string
	if direction == "up" {
		oppositePrice = downTokenPrice
		oppositeName = "DOWN"
		oppositeDir = "down"
	} else {
		oppositePrice = upTokenPrice
		oppositeName = "UP"
		oppositeDir = "up"
	}

	// Only buy the cheap side if it has good risk/reward (price ≤ MaxTokenPrice)
	if oppositePrice <= p.config.MaxTokenPrice && oppositePrice >= 0.02 {
		return true, oppositeDir, fmt.Sprintf("Contrarian: %s too expensive ($%.4f), buying %s at $%.4f instead",
			tokenName, tokenPrice, oppositeName, oppositePrice)
	}

	return false, direction, fmt.Sprintf("Both sides overpriced: %s=$%.4f, %s=$%.4f (Max: $%.2f)",
		tokenName, tokenPrice, oppositeName, oppositePrice, p.config.MaxTokenPrice)
}
