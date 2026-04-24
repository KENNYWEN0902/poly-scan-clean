# PolymarketBot 开发文档

更新时间：2026-04-24

## 唯一维护目录

请以后在这个目录开发和上传服务器：

```text
/Users/kennywen/Documents/Playground/polymarketbot
```

此前的 Codex 临时目录只用于恢复修复记录，不再作为主要项目目录。

## 核心模块

| 模块 | 文件 | 说明 |
|------|------|------|
| API 服务 | `cmd/api/main.go` | Dashboard REST API、WebSocket、账户余额、历史交易 |
| Chainlink RTDS | `internal/btc/chainlink_monitor.go` | RTDS 连接、订阅、PING、重连、fallback |
| 预测器 | `internal/btc/predictor.go` | 价格源校验、RTDS 严格模式 |
| Polymarket API | `internal/polymarket/api.go` | GET 超时、重试、User-Agent |
| PM2 配置 | `ecosystem.config.js` | 向 bot/API 传递环境变量 |
| Dashboard API 客户端 | `dashboard/src/api.ts` | 请求超时、WebSocket、订阅机制 |
| Dashboard Shell | `dashboard/src/App.tsx` | 全局状态、账户轮询、WebSocket 在线状态 |

## 本地前端构建

```bash
cd /Users/kennywen/Documents/Playground/polymarketbot/dashboard
npm install
npm run build
```

## 服务器构建

```bash
cd /root/poly-scan
gofmt -w cmd/api/main.go internal/btc/chainlink_monitor.go internal/btc/predictor.go internal/polymarket/api.go
go build -o poly-bot-api ./cmd/api
go build -o poly-bot ./cmd/main.go

cd /root/poly-scan/dashboard
npm install
npm run build
```

## 服务器重启

```bash
cd /root/poly-scan
pm2 restart poly-bot-api --update-env
pm2 restart poly-bot-btc --update-env
pm2 restart poly-dashboard --update-env
```

## 验证命令

```bash
curl -s --max-time 5 http://127.0.0.1:9876/api/account
curl -s --max-time 5 http://127.0.0.1:9876/api/trades
pm2 logs poly-bot-btc --lines 80
pm2 logs poly-bot-api --lines 50
pm2 logs poly-dashboard --lines 30
```

## 同步原则

- 不上传 `.env`、`.git`、`node_modules`、`.DS_Store`。
- 如果服务器已经有运行数据，不覆盖 `data/` 和 `logs/`。
- Dashboard 可以上传源码后服务器 `npm run build`，也可以直接上传本地 `dashboard/dist`。
- 修改 `ecosystem.config.js` 后重启 PM2 必须加 `--update-env`。
