package main

import (
	"math"
	"testing"
	"time"

	"poly-scan/internal/notification"
	"poly-scan/internal/polymarket"
)

func TestBuildPositionInfoUsesOfficialPositionMetrics(t *testing.T) {
	pos := polymarket.Position{
		Asset:        "asset-1",
		ConditionId:  "cond-1",
		Slug:         "btc-close-above-100k",
		Title:        "Will BTC close above 100k?",
		Outcome:      "Yes",
		Size:         100,
		AvgPrice:     0.40,
		CurPrice:     0.55,
		CurrentValue: 55,
		InitialValue: 40,
		CashPnL:      15,
		PercentPnL:   37.5,
	}

	got := buildPositionInfo(pos, 0)

	if got.MarketID != pos.Slug {
		t.Fatalf("expected market ID %q, got %q", pos.Slug, got.MarketID)
	}
	if got.MarketName != pos.Title {
		t.Fatalf("expected market name %q, got %q", pos.Title, got.MarketName)
	}
	if got.CurrentPrice != pos.CurPrice {
		t.Fatalf("expected current price %.2f, got %.2f", pos.CurPrice, got.CurrentPrice)
	}
	if got.CurrentValue != pos.CurrentValue {
		t.Fatalf("expected current value %.2f, got %.2f", pos.CurrentValue, got.CurrentValue)
	}
	if got.PnL != pos.CashPnL {
		t.Fatalf("expected pnl %.2f, got %.2f", pos.CashPnL, got.PnL)
	}
	if got.PnLPct != pos.PercentPnL {
		t.Fatalf("expected pnl pct %.2f, got %.2f", pos.PercentPnL, got.PnLPct)
	}
}

func TestBuildPositionInfoFallsBackToDerivedMetrics(t *testing.T) {
	pos := polymarket.Position{
		Asset:    "asset-2",
		Market:   "legacy-market",
		Outcome:  "No",
		Size:     10,
		AvgPrice: 0.20,
	}

	got := buildPositionInfo(pos, 0.35)

	if got.MarketID != pos.Market {
		t.Fatalf("expected fallback market ID %q, got %q", pos.Market, got.MarketID)
	}
	if got.CurrentValue != 3.5 {
		t.Fatalf("expected current value 3.5, got %.2f", got.CurrentValue)
	}
	if math.Abs(got.PnL-1.5) > 1e-9 {
		t.Fatalf("expected pnl 1.5, got %.2f", got.PnL)
	}
	if math.Abs(got.PnLPct-75) > 1e-9 {
		t.Fatalf("expected pnl pct 75, got %.2f", got.PnLPct)
	}
}

func TestBuildAccountInfoUsesOfficialPortfolioValue(t *testing.T) {
	positions := []PositionInfo{
		{IsActive: true, CurrentValue: 25, Size: 100, CurrentPrice: 0.25},
		{IsActive: true, CurrentValue: 15, Size: 30, CurrentPrice: 0.50},
		{IsActive: false, CurrentValue: 100, Size: 1, CurrentPrice: 100},
	}

	perf := accountPerformanceSnapshot{
		TotalPnL: 30,
		DailyStats: map[string]accountDailyStatSnapshot{
			"2026-04-16": {Date: "2026-04-16", Trades: 1, Wins: 1, PnL: 10, WinRate: 100},
			"2026-04-17": {Date: "2026-04-17", Trades: 2, Wins: 1, Losses: 1, PnL: 20, WinRate: 50},
		},
	}

	got := buildAccountInfo("0xabc", 60, positions, perf)

	if got.PositionsValue != 40 {
		t.Fatalf("expected positions value 40, got %.2f", got.PositionsValue)
	}
	if got.PortfolioValue != 100 {
		t.Fatalf("expected portfolio value 100, got %.2f", got.PortfolioValue)
	}
	if len(got.DailyStats) != 2 || len(got.EquityCurve) != 2 {
		t.Fatalf("expected 2 daily stats and 2 equity points, got %d and %d", len(got.DailyStats), len(got.EquityCurve))
	}
	if got.EquityCurve[0].Value != 80 || got.EquityCurve[1].Value != 100 {
		t.Fatalf("expected equity curve [80 100], got [%.2f %.2f]", got.EquityCurve[0].Value, got.EquityCurve[1].Value)
	}
}

func TestNotificationEventsToAlertsPreservesEventFields(t *testing.T) {
	timestamp := time.Date(2026, 4, 23, 15, 25, 38, 0, time.UTC)
	events := []notification.Event{
		{
			Type:      notification.EventTradeExecuted,
			Timestamp: timestamp,
			MarketID:  "btc-updown-5m-123",
			Message:   "Executed UP trade",
			Level:     notification.LevelInfo,
			Data: map[string]interface{}{
				"size": 5.0,
			},
		},
	}

	alerts := notificationEventsToAlerts(events)
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	if alerts[0].Type != string(notification.EventTradeExecuted) {
		t.Fatalf("expected alert type %q, got %q", notification.EventTradeExecuted, alerts[0].Type)
	}
	if alerts[0].Level != string(notification.LevelInfo) {
		t.Fatalf("expected alert level %q, got %q", notification.LevelInfo, alerts[0].Level)
	}
	if alerts[0].Message != events[0].Message {
		t.Fatalf("expected alert message %q, got %q", events[0].Message, alerts[0].Message)
	}
	if alerts[0].Details == "" {
		t.Fatalf("expected alert details to contain serialized event data")
	}
}

func TestLimitAlertsReturnsMostRecentEntries(t *testing.T) {
	alerts := []AlertInfo{
		{ID: "1", Message: "first"},
		{ID: "2", Message: "second"},
		{ID: "3", Message: "third"},
	}

	limited := limitAlerts(alerts, 2)
	if len(limited) != 2 {
		t.Fatalf("expected 2 alerts, got %d", len(limited))
	}
	if limited[0].ID != "2" || limited[1].ID != "3" {
		t.Fatalf("expected last 2 alerts, got %#v", limited)
	}

	limited[0].Message = "mutated"
	if alerts[1].Message != "second" {
		t.Fatalf("expected limitAlerts to return a copy")
	}
}
