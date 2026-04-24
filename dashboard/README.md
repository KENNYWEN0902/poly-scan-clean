# PolyBot Dashboard

更新时间：2026-04-24

这是 PolymarketBot 的 React + TypeScript + Vite 监控仪表盘，用于查看策略状态、pUSD 余额、持仓、历史交易、运行状态和风控信息。

## 当前已验证状态

- 首页、策略、持仓、历史、设置页面均可正常打开。
- 左下角账户余额从 `/api/account` 获取，当前显示的是 CLOB collateral/pUSD 可用余额。
- WebSocket 状态由全局连接统一管理，首页不再重复创建连接导致“离线”误报。
- 历史页兼容 `/api/trades` 返回 `[]` 的情况，没有交易时显示“暂无交易记录”。
- 所有 REST 请求都有 12 秒超时，单个接口慢不会拖死整页。

## 关键文件

| 文件 | 作用 |
|------|------|
| `src/App.tsx` | 全局布局、导航、账户余额轮询、WebSocket 状态 |
| `src/api.ts` | REST API、请求超时、WebSocket、订阅机制 |
| `src/pages/Portfolio.tsx` | 首页仪表盘，订阅全局 WebSocket 更新 |
| `src/pages/PnlHistory.tsx` | 交易历史，兼容空数组和缺失字段 |
| `src/pages/Positions.tsx` | 持仓页，账户接口慢时不阻塞页面 |
| `src/pages/Settings.tsx` | 参数配置，账户接口慢时不阻塞页面 |
| `src/types.ts` | Dashboard 与 API 返回结构 |

## 本地开发

```bash
cd /Users/kennywen/Documents/Playground/polymarketbot/dashboard
npm install
npm run dev
```

开发模式默认走 Vite proxy 或相对 `/api`。生产构建会根据当前页面 host 访问：

```text
http://服务器IP:9876/api
ws://服务器IP:9876/ws
```

## 生产构建

```bash
cd /root/poly-scan/dashboard
npm install
npm run build
pm2 restart poly-dashboard --update-env
```

如果已经从本地同步了 `dashboard/dist`，服务器上只需要：

```bash
pm2 restart poly-dashboard --update-env
```

## 验证命令

```bash
curl -s --max-time 5 http://127.0.0.1:9876/api/account
curl -s --max-time 5 http://127.0.0.1:9876/api/trades
pm2 logs poly-bot-api --lines 30
pm2 logs poly-dashboard --lines 30
```

正常表现：

- `/api/account` 返回 `collateral_balance/pusd_balance`。
- `/api/trades` 没有交易时返回 `[]`。
- 前端左下角显示“在线 · 已同步”。
