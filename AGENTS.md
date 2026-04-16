# AGENTS.md - Poly-Bot Development Guide

**Last Updated:** 2026-03-13
**Commit:** 6fececa
**Branch:** main

## Project Overview

Poly-Bot is a high-performance Polymarket BTC 5-minute delay arbitrage scanner with a monitoring dashboard. **Go + Python hybrid architecture**:
- **Go**: WebSocket listener, BTC spot price monitoring, strategy execution, REST API server
- **Python**: Order signing and execution via `py-clob-client` SDK
- **React Dashboard**: Real-time monitoring UI (Vite + Tailwind + Recharts)

## Structure

```
poly-scan/
├── cmd/
│   ├── main.go              # Main BTC strategy entry
│   ├── api/main.go          # Dashboard API server (530 lines, WebSocket + REST)
│   ├── btc/main.go          # Alternative BTC entry
│   └── test/main.go         # Integration tests
├── internal/
│   ├── btc/                 # Core BTC strategy module (1200+ lines) ⚡
│   │   ├── strategy.go      # Main trading loop, market scanning (671 lines)
│   │   ├── predictor.go     # Chainlink monitoring & prediction
│   │   ├── spot_monitor.go  # Real-time BTC price (Binance/Coinbase weighted)
│   │   ├── risk_manager.go  # Kelly sizing, exposure limits, drawdown control
│   │   └── indicators.go    # RSI, MACD, Bollinger, ATR, Momentum
│   ├── execution/
│   │   └── engine.go        # Order execution, Go-Python bridge
│   └── polymarket/
│       ├── api.go           # REST API client (Gamma API)
│       └── ws.go            # WebSocket client with heartbeat
├── dashboard/               # React monitoring UI ⚡
│   └── src/
│       ├── App.tsx          # Layout, routing, alert bell
│       ├── api.ts           # API service + WebSocket client
│       ├── types.ts         # TypeScript interfaces (mirror Go structs)
│       └── pages/           # Dashboard, Markets, Trades, Risk, Settings
├── scripts/
│   └── executor.py          # Python order executor (py-clob-client)
├── ecosystem.config.js      # PM2 configuration
└── start.sh                 # Startup script with env vars
```

⚡ = see subdirectory AGENTS.md for detailed docs.

## Where to Look

| Task | Location | Notes |
|------|----------|-------|
| Trading strategy logic | `internal/btc/strategy.go` | Main loop, market scanning, execution |
| Price prediction | `internal/btc/predictor.go` | Chainlink + spot price comparison |
| Risk management | `internal/btc/risk_manager.go` | Kelly formula, exposure limits |
| Technical indicators | `internal/btc/indicators.go` | RSI, MACD, Bollinger, ATR |
| Order execution | `internal/execution/engine.go` | Go → Python bridge |
| Dashboard UI | `dashboard/` | React app, see dashboard/AGENTS.md |
| API server | `cmd/api/main.go` | REST + WebSocket for dashboard |
| Python signer | `scripts/executor.py` | EIP-712 order signing |

## Commands

```bash
# Build
go build -o poly-bot ./cmd/main.go       # Main strategy
go build -o poly-bot-api ./cmd/api/main.go # API server
go build -o test-arb ./cmd/test/          # Integration tests

# Run (simulation - no credentials needed)
./poly-bot

# Run (live trading)
export POLY_PRIVATE_KEY="0x..." POLY_API_KEY="..." POLY_API_SECRET="..." POLY_PASSPHRASE="..."
./poly-bot

# Dashboard API server
export API_PORT=9876  # default
./poly-bot-api

# Dashboard frontend (development)
cd dashboard && npm install && npm run dev

# Process management (PM2)
pm2 start ecosystem.config.js
pm2 logs poly-bot-btc
pm2 restart poly-bot-btc
pm2 stop poly-bot-btc
```

## Go Code Style

### Imports — 3 groups, blank lines between
```go
import (
    // Standard library
    "encoding/json"
    "log"
    "sync"

    // External
    "github.com/gorilla/websocket"

    // Local
    "poly-scan/internal/execution"
    "poly-scan/internal/polymarket"
)
```

### Naming
- **Packages**: lowercase, single word (`btc`, `execution`, `polymarket`)
- **Types**: PascalCase exported, camelCase private
- **Config structs**: `XxxConfig` with `DefaultXxxConfig()` constructor
- **Interfaces**: `-er` suffix (loosely followed)

### Structs + Constructors
```go
type BTCStrategy struct {
    client     *polymarket.Client    // aligned comments
    mu         sync.RWMutex          // mutex for thread safety
}

func NewBTCStrategy(client *polymarket.Client, config BTCMarketConfig) *BTCStrategy {
    return &BTCStrategy{client: client, config: config}
}
```

### Error Handling
- Return errors as last value
- `log.Printf("[COMPONENT] context: %v", err)` for logging
- `log.Fatalf` only in `main()`
- Never suppress errors silently

## Architecture

### Go → Python Bridge
1. Go calls `python3 scripts/executor.py` via `exec.Command`
2. Orders as JSON via stdin
3. Credentials via env vars
4. Result JSON via stdout

### Thread Safety
- `sync.RWMutex` for shared state
- `mu.Lock()` writes, `mu.RLock()` reads
- Always `defer mu.Unlock()`

### BTC Strategy Flow
1. Monitor BTC spot (Binance + Coinbase weighted avg)
2. Track Chainlink oracle price for delay detection
3. Calculate trend over 30s window
4. Predict direction 10s before market close
5. Execute when confidence ≥ 55% and price change ≥ 0.01%

## Configuration

| Config | Default | Description |
|--------|---------|-------------|
| WindowDuration | 5m | Market window |
| PredictBeforeEnd | 30s | Prediction start time |
| MinConfidence | 0.55 | Confidence threshold |
| MaxPositionSize | 50.0 | Max USD per trade |
| MinPriceChangePct | 0.01% | Min price change |
| MaxTokenPrice | 0.60 | Max token price to buy |

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| POLY_PRIVATE_KEY | Yes | Ethereum private key (0x-prefixed) |
| POLY_API_KEY | Yes | Polymarket API key |
| POLY_API_SECRET | Yes | Polymarket API secret |
| POLY_PASSPHRASE | Yes | Polymarket passphrase |
| POLY_SIGNATURE_TYPE | No | 0=EOA, 1=Proxy, 2=Gnosis Safe |
| POLY_FUNDER_ADDRESS | No | Required for Proxy/Gnosis |
| API_PORT | No | Dashboard API port (default 9876) |

## Anti-Patterns (This Project)

- No unit tests exist — only integration test binary in `cmd/test/`
- No CI/CD, no Makefile, no Dockerfile
- `cmd/api/main.go` is 530 lines monolith — single file for entire API server
- No `.env.example` — env vars documented in README only

## Important Files

- `logs/btc-out.log` — PM2 stdout
- `logs/btc-error.log` — PM2 stderr
