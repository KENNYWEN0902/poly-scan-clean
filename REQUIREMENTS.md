# PolymarketBot 需求文档

更新时间：2026-04-24

## 项目目录要求

- 正式本地项目目录：`/Users/kennywen/Documents/Playground/polymarketbot`
- 临时修复目录：`/Users/kennywen/Documents/Codex/2026-04-21-ubantu-22-04-lts-bot-github/migration-polymarketbot`
- 服务器默认项目目录：`/root/poly-scan`

后续维护以正式本地项目目录为准，临时修复目录只作为历史来源，不再作为主要开发目录。

## 运行目标

- 连接 Polymarket RTDS 获取 BTC/USD Chainlink 价格。
- 使用 Binance/Coinbase 现货价格辅助预测 BTC 5 分钟 Up/Down 市场。
- 使用 pUSD/CLOB collateral 作为可用余额来源。
- Dashboard 能稳定展示账户、策略、持仓、历史、设置和 WebSocket 在线状态。

## 功能需求

- RTDS 订阅必须使用当前 Polymarket 协议：`action=subscribe`、`topic=crypto_prices_chainlink`、`filters={"symbol":"btc/usd"}`。
- RTDS WebSocket 必须保活，断线后自动重连。
- `/api/account` 必须优先查询 CLOB collateral/pUSD 余额。
- `/api/account` 必须有短超时和缓存，不能拖死页面加载。
- `/api/trades` 没有交易时必须返回空数组 `[]`。
- Dashboard 任意单个 API 慢或失败时，页面主体仍应显示。
- Dashboard WebSocket 连接应由全局 App 管理，页面组件只订阅事件。

## 配置需求

- `POLY_PRIVATE_KEY`：服务器 `.env` 中配置，不提交到仓库。
- `POLY_SIGNATURE_TYPE`：通常为 `2`。
- `POLY_FUNDER_ADDRESS`：使用 Polymarket 个人资料页展示的地址。
- `POLY_REQUIRE_CHAINLINK_RTDS`：可选，设为 `1` 时预测必须使用 RTDS。
- `POLY_DASHBOARD_FETCH_GENERIC_MARKETS`：可选，设为 `1` 时 Dashboard API 拉取 Gamma 通用市场。

## 验收标准

- `pm2 list` 中 `poly-bot-api`、`poly-bot-btc`、`poly-dashboard` 均为 `online`。
- `pm2 logs poly-bot-btc` 出现 `RTDS BTC/USD price established`。
- `curl -s --max-time 5 http://127.0.0.1:9876/api/account` 返回 pUSD/collateral 余额。
- `curl -s --max-time 5 http://127.0.0.1:9876/api/trades` 没交易时返回 `[]`。
- Dashboard 左下角显示正确余额和“在线 · 已同步”。
- 历史页没有交易时显示“暂无交易记录”，不是空白页或无限加载。
