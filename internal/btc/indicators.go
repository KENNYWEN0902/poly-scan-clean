package btc

import (
	"math"
)

// TechnicalIndicators calculates various technical indicators
type TechnicalIndicators struct{}

func NewTechnicalIndicators() *TechnicalIndicators {
	return &TechnicalIndicators{}
}

// RSI calculates the Relative Strength Index
func (ti *TechnicalIndicators) RSI(prices []float64, period int) float64 {
	if len(prices) < period+1 {
		return 50
	}

	var gains, losses float64
	count := 0

	for i := len(prices) - period; i < len(prices); i++ {
		change := prices[i] - prices[i-1]
		if change > 0 {
			gains += change
		} else if change < 0 {
			losses += math.Abs(change)
		}
		count++
	}

	if count == 0 {
		return 50
	}

	avgGain := gains / float64(count)
	avgLoss := losses / float64(count)

	if avgGain == 0 && avgLoss == 0 {
		return 50
	}
	if avgGain == 0 {
		return 5
	}
	if avgLoss == 0 {
		return 95
	}

	rs := avgGain / avgLoss
	rsi := 100 - (100 / (1 + rs))

	if rsi < 1 {
		return 1
	}
	if rsi > 99 {
		return 99
	}
	return rsi
}

// MACDResult holds the MACD calculation result
type MACDResult struct {
	MACD      float64
	Signal    float64
	Histogram float64
}

// MACD calculates Moving Average Convergence Divergence
func (ti *TechnicalIndicators) MACD(prices []float64) MACDResult {
	if len(prices) < 26 {
		return MACDResult{}
	}

	// Build MACD line series for proper signal line calculation
	var macdSeries []float64
	for i := 26; i <= len(prices); i++ {
		subset := prices[:i]
		ema12 := ti.EMA(subset, 12)
		ema26 := ti.EMA(subset, 26)
		macdSeries = append(macdSeries, ema12-ema26)
	}

	macdLine := macdSeries[len(macdSeries)-1]

	// Signal line is a 9-period EMA of the MACD series
	var signalLine float64
	if len(macdSeries) >= 9 {
		signalLine = ti.EMA(macdSeries, 9)
	} else {
		signalLine = macdLine
	}

	histogram := macdLine - signalLine

	return MACDResult{
		MACD:      macdLine,
		Signal:    signalLine,
		Histogram: histogram,
	}
}

// EMA calculates Exponential Moving Average
func (ti *TechnicalIndicators) EMA(prices []float64, period int) float64 {
	if len(prices) < period {
		if len(prices) == 0 {
			return 0
		}
		return prices[len(prices)-1]
	}

	multiplier := 2.0 / float64(period+1)

	var sum float64
	startIdx := len(prices) - period
	for i := startIdx; i < len(prices); i++ {
		sum += prices[i]
	}
	ema := sum / float64(period)

	for i := startIdx + 1; i < len(prices); i++ {
		ema = (prices[i]-ema)*multiplier + ema
	}

	return ema
}

// BollingerBands holds Bollinger Bands calculation result
type BollingerBands struct {
	Upper    float64
	Middle   float64
	Lower    float64
	Width    float64
	PercentB float64
}

// Bollinger calculates Bollinger Bands
func (ti *TechnicalIndicators) Bollinger(prices []float64, period int, stdDev float64) BollingerBands {
	if len(prices) < period {
		return BollingerBands{}
	}

	var sum float64
	startIdx := len(prices) - period
	for i := startIdx; i < len(prices); i++ {
		sum += prices[i]
	}
	middle := sum / float64(period)

	var variance float64
	for i := startIdx; i < len(prices); i++ {
		diff := prices[i] - middle
		variance += diff * diff
	}
	variance /= float64(period)
	std := math.Sqrt(variance)

	upper := middle + stdDev*std
	lower := middle - stdDev*std

	width := 0.0
	if middle > 0 {
		width = (upper - lower) / middle * 100
	}

	currentPrice := prices[len(prices)-1]
	percentB := 0.0
	if upper != lower {
		percentB = (currentPrice - lower) / (upper - lower) * 100
	}

	return BollingerBands{
		Upper:    upper,
		Middle:   middle,
		Lower:    lower,
		Width:    width,
		PercentB: percentB,
	}
}

// ATR calculates Average True Range from prices
func (ti *TechnicalIndicators) ATRFromPrices(prices []float64, period int) float64 {
	if len(prices) < period+1 {
		return 0
	}

	var trueRanges []float64
	for i := 1; i < len(prices); i++ {
		tr := math.Abs(prices[i] - prices[i-1])
		trueRanges = append(trueRanges, tr)
	}

	if len(trueRanges) < period {
		period = len(trueRanges)
	}

	var sum float64
	for i := len(trueRanges) - period; i < len(trueRanges); i++ {
		sum += trueRanges[i]
	}

	return sum / float64(period)
}

// Momentum calculates price momentum over n periods
func (ti *TechnicalIndicators) Momentum(prices []float64, period int) float64 {
	if len(prices) < period+1 {
		return 0
	}
	current := prices[len(prices)-1]
	past := prices[len(prices)-1-period]
	if past == 0 {
		return 0
	}
	return (current - past) / past * 100
}
