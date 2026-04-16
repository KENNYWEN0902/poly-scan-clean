package btc

import (
	"math"
	"testing"
)

func newTestPredictor(config PredictorConfig) *MarketPredictor {
	return &MarketPredictor{config: config}
}

// === calculateEnhancedConfidence tests ===

func TestEnhancedConfidence_StrongMovement(t *testing.T) {
	p := newTestPredictor(DefaultPredictorConfig())
	conf := p.calculateEnhancedConfidence(0.20, "up", 0.8, "up", 0.5, 0.55)
	// 0.20% change → +0.10, trend >0.7 → +0.06, agreement → +0.05 = +0.21, capped at +0.20
	expected := 0.55 + 0.20
	if math.Abs(conf-expected) > 0.01 {
		t.Errorf("Expected ~%.2f, got %.2f", expected, conf)
	}
}

func TestEnhancedConfidence_MinThreshold(t *testing.T) {
	p := newTestPredictor(DefaultPredictorConfig())
	conf := p.calculateEnhancedConfidence(0.07, "up", 0.2, "down", 0.1, 0.55)
	// 0.07% → +0.04, trend <0.5 → 0, no agreement → 0 = +0.04
	expected := 0.59
	if math.Abs(conf-expected) > 0.01 {
		t.Errorf("Expected ~%.2f, got %.2f", expected, conf)
	}
}

func TestEnhancedConfidence_CappedAt90(t *testing.T) {
	p := newTestPredictor(DefaultPredictorConfig())
	conf := p.calculateEnhancedConfidence(0.20, "up", 0.9, "up", 0.9, 0.85)
	if conf > 0.90 {
		t.Errorf("Confidence should be capped at 0.90, got %.2f", conf)
	}
}

func TestEnhancedConfidence_ZeroChange(t *testing.T) {
	p := newTestPredictor(DefaultPredictorConfig())
	conf := p.calculateEnhancedConfidence(0.0, "neutral", 0.0, "neutral", 0.0, 0.50)
	// No bonus at all
	if math.Abs(conf-0.50) > 0.01 {
		t.Errorf("Expected 0.50, got %.2f", conf)
	}
}

func TestEnhancedConfidence_MediumMovement(t *testing.T) {
	p := newTestPredictor(DefaultPredictorConfig())
	conf := p.calculateEnhancedConfidence(0.12, "down", 0.6, "down", 0.4, 0.55)
	// 0.12% → +0.07, trend 0.5-0.7 → +0.03, agreement → +0.05 = +0.15
	expected := 0.70
	if math.Abs(conf-expected) > 0.01 {
		t.Errorf("Expected ~%.2f, got %.2f", expected, conf)
	}
}

func TestEnhancedConfidence_DisagreementDoesNotAdd(t *testing.T) {
	p := newTestPredictor(DefaultPredictorConfig())
	conf := p.calculateEnhancedConfidence(0.10, "up", 0.6, "down", 0.6, 0.55)
	// 0.10% → +0.07, trend 0.5-0.7 → +0.03, no agreement (opposite) → 0 = +0.10
	expected := 0.65
	if math.Abs(conf-expected) > 0.01 {
		t.Errorf("Expected ~%.2f, got %.2f", expected, conf)
	}
}

// === determineDirection tests ===

func TestDetermineDirection_StrongUp(t *testing.T) {
	p := newTestPredictor(DefaultPredictorConfig())
	dir := p.determineDirection(0.05, 0.02, "up", 0.8)
	if dir != "up" {
		t.Errorf("Expected up, got %s", dir)
	}
}

func TestDetermineDirection_StrongDown(t *testing.T) {
	p := newTestPredictor(DefaultPredictorConfig())
	dir := p.determineDirection(-0.05, -0.02, "down", 0.8)
	if dir != "down" {
		t.Errorf("Expected down, got %s", dir)
	}
}

func TestDetermineDirection_ConflictFavorsWindowChange(t *testing.T) {
	p := newTestPredictor(DefaultPredictorConfig())
	// Window says up (weight 4), momentum says down (weight 1.5), trend says down (weight ~0.8)
	dir := p.determineDirection(0.05, -0.02, "down", 0.8)
	if dir != "up" {
		t.Errorf("Window change should dominate, expected up, got %s", dir)
	}
}

func TestDetermineDirection_TieBreak(t *testing.T) {
	p := newTestPredictor(DefaultPredictorConfig())
	// Zero changes → default to windowChange direction
	dir := p.determineDirection(0.001, 0.001, "neutral", 0.0)
	if dir != "up" {
		t.Errorf("Expected up (default for positive change), got %s", dir)
	}
}

func TestDetermineDirection_NegativeTieBreak(t *testing.T) {
	p := newTestPredictor(DefaultPredictorConfig())
	dir := p.determineDirection(-0.001, -0.001, "neutral", 0.0)
	if dir != "down" {
		t.Errorf("Expected down (default for negative change), got %s", dir)
	}
}

// === calculateConfidence (legacy) tests ===

func TestCalculateConfidence_Base(t *testing.T) {
	p := newTestPredictor(DefaultPredictorConfig())
	conf := p.calculateConfidence(0.0, 0.0, "neutral", 0.0)
	if math.Abs(conf-0.50) > 0.01 {
		t.Errorf("Base confidence should be 0.50, got %.2f", conf)
	}
}

func TestCalculateConfidence_HighAll(t *testing.T) {
	p := newTestPredictor(DefaultPredictorConfig())
	conf := p.calculateConfidence(0.06, 0.03, "up", 0.8)
	// base 0.5 + window 0.15 + momentum 0.20 + trend 0.10 = 0.95, capped 0.90
	if conf > 0.90 {
		t.Errorf("Confidence should be capped at 0.90, got %.2f", conf)
	}
}

func TestCalculateConfidence_MediumWindow(t *testing.T) {
	p := newTestPredictor(DefaultPredictorConfig())
	conf := p.calculateConfidence(0.03, 0.0, "neutral", 0.0)
	// base 0.5 + window 0.10 = 0.60
	expected := 0.60
	if math.Abs(conf-expected) > 0.01 {
		t.Errorf("Expected ~%.2f, got %.2f", expected, conf)
	}
}

// === ShouldTrade tests ===

func TestShouldTrade_Normal(t *testing.T) {
	p := newTestPredictor(DefaultPredictorConfig())
	ok, dir, _ := p.ShouldTrade("up", 0.50, 0.50)
	if !ok {
		t.Error("Should trade when token price is within acceptable range")
	}
	if dir != "up" {
		t.Errorf("Direction should be up, got %s", dir)
	}
}

func TestShouldTrade_TokenTooExpensive(t *testing.T) {
	p := newTestPredictor(DefaultPredictorConfig())
	ok, dir, reason := p.ShouldTrade("up", 0.85, 0.15)
	if !ok {
		t.Error("Should trade contrarian when UP too expensive but DOWN is cheap")
	}
	if dir != "down" {
		t.Errorf("Should flip to down direction, got %s", dir)
	}
	if reason == "" {
		t.Error("Should provide a reason")
	}
}

func TestShouldTrade_TokenTooLow(t *testing.T) {
	p := newTestPredictor(DefaultPredictorConfig())
	ok, _, _ := p.ShouldTrade("up", 0.01, 0.99)
	if ok {
		t.Error("Should NOT trade when token price is below 0.02 (market settled)")
	}
}

func TestShouldTrade_MarketSettled(t *testing.T) {
	p := newTestPredictor(DefaultPredictorConfig())
	ok, _, reason := p.ShouldTrade("up", 0.99, 0.99)
	if ok {
		t.Error("Should NOT trade when both token prices are extreme (settled market)")
	}
	if reason == "" {
		t.Error("Should provide reason")
	}
}

func TestShouldTrade_DownDirection(t *testing.T) {
	p := newTestPredictor(DefaultPredictorConfig())
	ok, dir, _ := p.ShouldTrade("down", 0.70, 0.30)
	if !ok {
		t.Error("Should trade when DOWN token price is $0.30 (under max)")
	}
	if dir != "down" {
		t.Errorf("Direction should be down, got %s", dir)
	}
}

func TestShouldTrade_DownTokenTooExpensive(t *testing.T) {
	p := newTestPredictor(DefaultPredictorConfig())
	ok, dir, _ := p.ShouldTrade("down", 0.20, 0.80)
	if !ok {
		t.Error("Should trade contrarian when DOWN too expensive but UP is cheap")
	}
	if dir != "up" {
		t.Errorf("Should flip to up direction, got %s", dir)
	}
}

func TestShouldTrade_AtMaxPrice(t *testing.T) {
	p := newTestPredictor(DefaultPredictorConfig())
	ok, _, _ := p.ShouldTrade("up", 0.62, 0.38)
	if !ok {
		t.Error("Should trade when token price equals MaxTokenPrice exactly")
	}
}
