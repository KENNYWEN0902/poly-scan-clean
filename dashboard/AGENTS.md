# Dashboard — React Monitoring UI

## Overview

Real-time monitoring dashboard for the BTC arbitrage bot. React 19 + Vite 7 + Tailwind 4 + Recharts. Connects to Go API server (`cmd/api/main.go`) via REST + WebSocket.

## Tech Stack

- React 19, React Router 7
- Vite 7 (with `@vitejs/plugin-react`)
- Tailwind CSS 4 (via `@tailwindcss/vite` plugin)
- Recharts 3 (charts), Lucide (icons), date-fns
- TypeScript 5.9

## Structure

```
dashboard/src/
├── App.tsx         # Layout shell: sidebar nav + header + routes
├── api.ts          # ApiService class — REST fetch + WebSocket with reconnect
├── types.ts        # TypeScript interfaces (mirror Go API structs, snake_case JSON)
├── index.css       # Tailwind imports + custom animations
├── main.tsx        # React entry
├── assets/         # Static assets
└── pages/
    ├── Dashboard.tsx   # Main overview: strategy state, indicators, charts
    ├── Markets.tsx     # Active BTC Up/Down markets
    ├── Trades.tsx      # Trade history table
    ├── Risk.tsx        # Risk metrics, exposure, drawdown
    └── Settings.tsx    # Strategy config display/edit
```

## Data Flow

```
Go API (:9876)  →  REST /api/*    →  ApiService.fetch()  →  React state
              ↘  WebSocket /ws   →  ApiService.ws.onmessage →  real-time updates
```

### API Service (`api.ts`)
- `ApiService` class — singleton, exported as `api`
- REST methods: `getDashboard()`, `getMarkets()`, `getTrades()`, etc.
- WebSocket: auto-reconnect with exponential backoff (max 10 attempts)
- Subscribe pattern: `subscribe(eventType, callback)`

### Types (`types.ts`)
- **snake_case** JSON fields (matches Go `json:"snake_case"` tags)
- Key interfaces: `DashboardData`, `StrategyState`, `TechnicalIndicators`, `TradeInfo`, `MarketInfo`

## Commands

```bash
cd dashboard
npm install
npm run dev        # Dev server (Vite HMR)
npm run build      # Production build (tsc + vite build)
npm run preview    # Preview production build
npm run lint       # ESLint
```

## Conventions

- Page components in `src/pages/`, one per route
- Sidebar nav defined as `navItems` array in `App.tsx`
- Dark theme throughout: `bg-[#0f172a]`, `bg-[#1e293b]`, `border-[#334155]`
- Responsive not implemented — fixed sidebar layout
- Alert bell in header, dropdown with recent alerts

## Anti-Patterns

- No state management library — all state in component `useState`
- No error boundaries
- No loading states or skeleton screens
- WebSocket reconnect uses simple backoff, no jitter
- `types.ts` duplicates Go structs — must be kept in sync manually

## Gotchas

- API proxy: Vite dev server proxies `/api` and `/ws` to Go server on port 9876
- Check `vite.config.ts` for proxy configuration
- Dashboard won't work without API server running
- `node_modules/` and `dist/` are present — not in `.gitignore` for `dist/`
