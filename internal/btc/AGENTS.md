# BTC Strategy Module

**Core module** — 1200+ lines, 5 files. This is where the money is made.

## Overview

Implements the BTC 5-minute delay arbitrage strategy. Monitors real-time spot prices (Binance + Coinbase weighted avg), compares against Chainlink oracle delay, predicts direction, and executes via Polymarket CLOB.

## Structure

```
btc/
├── strategy.go      # Main loop (671 lines) ⚠️ LARGEST FILE
│   ├── BTCStrategy struct — orchestrates everything
│   ├── Run() — main loop, market scanning
│   ├── scanMarkets() — finds active BTC Up/Down windows
│   ├── evaluateMarket() — prediction + execution decision
│   └── executeTrade() — order placement via execution.Engine
├── spot_monitor.go  # Real-time BTC price
│   ├── SpotMonitor — WebSocket to Binance + Coinbase
│   ├── GetPriceHistory() — rolling window of prices
│   └── GetTrend() — trend direction + strength over N seconds
├── predictor.go     # Chainlink delay detection
│   ├── MarketPredictor — compares spot vs oracle
│   └── PredictDirection() — returns UP/DOWN + confidence
├── risk_manager.go  # Position sizing + limits
│   ├── RiskManager — Kelly criterion, exposure tracking
│   ├── CheckTrade() — pre-trade risk gate
│   └── UpdatePosition() — post-trade state update
└── indicators.go    # Technical analysis
    ├── TechnicalIndicators struct
    ├── RSI(), MACD(), Bollinger(), ATR(), Momentum()
    └── All take []float64 prices, return computed values
```

## Key Flows

### Prediction → Execution
1. `strategy.evaluateMarket()` called for each active window
2. `predictor.PredictDirection()` gets confidence + direction
3. `riskManager.CheckTrade()` gates on exposure/drawdown
4. If pass → `execution.Engine.PlaceOrder()` → Python signer

### Price Monitoring
- `SpotMonitor` maintains rolling buffer (30s default)
- Dual-source: Binance (primary) + Coinbase (backup), weighted avg
- Thread-safe via `sync.RWMutex`

## Conventions

- All public functions log with `[BTC]` prefix
- Config via `BTCMarketConfig` with `DefaultBTCMarketConfig()` constructor
- Positions tracked in `map[string]*Position` with separate mutex
- Price history stored as `[]PricePoint{Price, Timestamp}`

## Anti-Patterns

- `strategy.go` is 671 lines — avoid adding more logic here
- Risk config and strategy config are separate structs but loaded together
- No interface abstraction for exchanges — hardcoded Binance/Coinbase

## Gotchas

- `MinConfidence` default is 0.55 (not 0.65 as in README — README is outdated)
- `PredictBeforeEnd` is 30s (not 10s as in README)
- Orderbook state stored in `marketState map[string]map[string]*OrderbookUpdate` — nested maps
- `execTimeMu` separate mutex prevents deadlock with `mu`
