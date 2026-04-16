package btc

import (
	"math"
	"testing"
)

func TestRSI_Insufficient_Data(t *testing.T) {
	ti := NewTechnicalIndicators()
	rsi := ti.RSI([]float64{100, 101, 102}, 14)
	if rsi != 50 {
		t.Errorf("expected RSI=50 for insufficient data, got %.2f", rsi)
	}
}

func TestRSI_AllUp(t *testing.T) {
	ti := NewTechnicalIndicators()
	prices := make([]float64, 20)
	for i := range prices {
		prices[i] = float64(100 + i)
	}
	rsi := ti.RSI(prices, 14)
	if rsi < 90 {
		t.Errorf("expected RSI > 90 for all-up prices, got %.2f", rsi)
	}
}

func TestRSI_AllDown(t *testing.T) {
	ti := NewTechnicalIndicators()
	prices := make([]float64, 20)
	for i := range prices {
		prices[i] = float64(120 - i)
	}
	rsi := ti.RSI(prices, 14)
	if rsi > 10 {
		t.Errorf("expected RSI < 10 for all-down prices, got %.2f", rsi)
	}
}

func TestRSI_Mixed(t *testing.T) {
	ti := NewTechnicalIndicators()
	prices := []float64{100, 102, 101, 103, 102, 104, 103, 105, 104, 106, 105, 107, 106, 108, 107, 109}
	rsi := ti.RSI(prices, 14)
	// Mixed oscillating prices should give moderate RSI
	if rsi < 30 || rsi > 70 {
		t.Errorf("expected moderate RSI for mixed prices, got %.2f", rsi)
	}
}

func TestRSI_Clamped(t *testing.T) {
	ti := NewTechnicalIndicators()
	// Flat prices
	flat := make([]float64, 20)
	for i := range flat {
		flat[i] = 100
	}
	rsi := ti.RSI(flat, 14)
	if rsi != 50 {
		t.Errorf("expected RSI=50 for flat prices, got %.2f", rsi)
	}
}

func TestMACD_InsufficientData(t *testing.T) {
	ti := NewTechnicalIndicators()
	result := ti.MACD([]float64{1, 2, 3})
	if result.MACD != 0 || result.Signal != 0 || result.Histogram != 0 {
		t.Errorf("expected zero MACD for insufficient data, got %+v", result)
	}
}

func TestMACD_ValidComputation(t *testing.T) {
	ti := NewTechnicalIndicators()
	// Generate enough data for MACD (26+ points)
	prices := make([]float64, 40)
	for i := range prices {
		prices[i] = 100 + float64(i)*0.5
	}
	result := ti.MACD(prices)

	// In an uptrend, MACD should be positive
	if result.MACD <= 0 {
		t.Errorf("expected positive MACD for uptrend, got %.4f", result.MACD)
	}
	// Histogram = MACD - Signal
	expectedHist := result.MACD - result.Signal
	if math.Abs(result.Histogram-expectedHist) > 0.0001 {
		t.Errorf("histogram should equal MACD-Signal: got %.4f, expected %.4f", result.Histogram, expectedHist)
	}
}

func TestMACD_Downtrend(t *testing.T) {
	ti := NewTechnicalIndicators()
	prices := make([]float64, 40)
	for i := range prices {
		prices[i] = 200 - float64(i)*0.5
	}
	result := ti.MACD(prices)
	if result.MACD >= 0 {
		t.Errorf("expected negative MACD for downtrend, got %.4f", result.MACD)
	}
}

func TestEMA_Basic(t *testing.T) {
	ti := NewTechnicalIndicators()
	prices := []float64{10, 11, 12, 13, 14, 15}
	ema := ti.EMA(prices, 3)

	// EMA should be weighted toward recent prices
	if ema < 13 || ema > 15 {
		t.Errorf("expected EMA near recent prices, got %.2f", ema)
	}
}

func TestEMA_InsufficientData(t *testing.T) {
	ti := NewTechnicalIndicators()
	ema := ti.EMA([]float64{42}, 5)
	if ema != 42 {
		t.Errorf("expected last price for insufficient data, got %.2f", ema)
	}
}

func TestEMA_Empty(t *testing.T) {
	ti := NewTechnicalIndicators()
	ema := ti.EMA([]float64{}, 5)
	if ema != 0 {
		t.Errorf("expected 0 for empty prices, got %.2f", ema)
	}
}

func TestBollinger_InsufficientData(t *testing.T) {
	ti := NewTechnicalIndicators()
	bb := ti.Bollinger([]float64{1, 2, 3}, 20, 2.0)
	if bb.Upper != 0 || bb.Middle != 0 {
		t.Errorf("expected zero BB for insufficient data, got Upper=%.2f", bb.Upper)
	}
}

func TestBollinger_Valid(t *testing.T) {
	ti := NewTechnicalIndicators()
	prices := make([]float64, 25)
	for i := range prices {
		prices[i] = 100 + float64(i%5) // oscillating 100-104
	}
	bb := ti.Bollinger(prices, 20, 2.0)

	if bb.Upper <= bb.Middle {
		t.Errorf("upper band should be above middle: upper=%.2f, middle=%.2f", bb.Upper, bb.Middle)
	}
	if bb.Lower >= bb.Middle {
		t.Errorf("lower band should be below middle: lower=%.2f, middle=%.2f", bb.Lower, bb.Middle)
	}
	if bb.Width <= 0 {
		t.Errorf("expected positive width, got %.2f", bb.Width)
	}
	// PercentB should be between 0 and 100 for data within bands
	if bb.PercentB < -50 || bb.PercentB > 150 {
		t.Errorf("expected PercentB in reasonable range, got %.2f", bb.PercentB)
	}
}

func TestBollinger_FlatPrices(t *testing.T) {
	ti := NewTechnicalIndicators()
	prices := make([]float64, 25)
	for i := range prices {
		prices[i] = 100
	}
	bb := ti.Bollinger(prices, 20, 2.0)

	// For flat prices, upper == middle == lower
	if bb.Upper != bb.Lower {
		t.Errorf("flat prices should have equal bands: upper=%.2f, lower=%.2f", bb.Upper, bb.Lower)
	}
}

func TestATRFromPrices_InsufficientData(t *testing.T) {
	ti := NewTechnicalIndicators()
	atr := ti.ATRFromPrices([]float64{100, 101}, 14)
	if atr != 0 {
		t.Errorf("expected ATR=0 for insufficient data, got %.4f", atr)
	}
}

func TestATRFromPrices_Valid(t *testing.T) {
	ti := NewTechnicalIndicators()
	prices := make([]float64, 20)
	for i := range prices {
		if i%2 == 0 {
			prices[i] = 100
		} else {
			prices[i] = 102
		}
	}
	atr := ti.ATRFromPrices(prices, 14)
	if atr <= 0 {
		t.Errorf("expected positive ATR for oscillating prices, got %.4f", atr)
	}
	// Each true range is 2, so ATR should be 2
	if math.Abs(atr-2.0) > 0.01 {
		t.Errorf("expected ATR≈2.0, got %.4f", atr)
	}
}
