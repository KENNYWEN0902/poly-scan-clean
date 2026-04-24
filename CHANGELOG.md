# PolymarketBot 修改记录

## 2026-04-24

- 统一正式项目目录为 `/Users/kennywen/Documents/Playground/polymarketbot`。
- 修复 ChainlinkMonitor 连接 Polymarket RTDS：改用当前订阅协议、添加 PING 保活、断线持续重连。
- 增加 `POLY_REQUIRE_CHAINLINK_RTDS`，支持强制只使用 RTDS Chainlink 价格源。
- 修复 Dashboard pUSD 余额显示：`/api/account` 优先查询 CLOB collateral，并增加超时、缓存和 fallback。
- 修复历史页空白/无限加载：`/api/trades` 无交易时返回 `[]`，前端兼容空数组和缺失字段。
- 修复所有页面被慢接口拖死：前端请求增加 12 秒超时，页面主体不再依赖账户接口完成。
- 修复左下角 WebSocket 状态误报离线：首页改为订阅全局连接，不再重复创建 WebSocket。
- 增加 API 通知事件转 Dashboard alerts 的能力，为网页运行日志展示做准备。
- 增加 `REQUIREMENTS.md`、`DEVELOPMENT.md`、`CHANGELOG.md` 及对应 HTML 文档。

## 2026-04-23

- 服务器验证 pUSD 余额可正常显示为 `$64.98`。
- 服务器验证 RTDS 日志出现 `Connected to Polymarket RTDS` 和 `RTDS BTC/USD price established`。
- 服务器验证 Dashboard 历史页、首页、持仓页、策略页、设置页均可打开。

## 2026-04-22

- 补充 CLOB V2/pUSD 迁移教程。
- 明确 `POLY_FUNDER_ADDRESS` 应使用 Polymarket 个人资料页展示的地址。
