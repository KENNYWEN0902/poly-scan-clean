# Polymarket BTC 5分钟延迟套利机器人 (Poly-Bot)

> 基于 Go + Python 混合架构的高性能 Polymarket BTC 5分钟延迟套利交易机器人，附带实时监控仪表盘。

## 核心特性

- 🔮 **BTC 价格预测**：利用 Chainlink 预言机与实时现货价格之间的延迟进行套利
- 📊 **实时监控仪表盘**：React 前端，实时展示策略状态、交易历史、PnL 和风险指标
- 🎯 **Kelly 仓位管理**：基于凯利公式动态计算最优仓位大小
- 🛡️ **多层风险控制**：最大敞口、日损失限制、回撤控制
- 📈 **技术指标**：RSI、MACD、布林带、ATR、动量分析
- 🔄 **自动结算**：窗口结束后自动领取收益

## 策略原理

Polymarket 的 BTC Up/Down 市场使用 Chainlink Data Streams 作为结算数据源。由于 Chainlink 价格存在聚合平滑延迟，而 Binance/Coinbase 现货价格是实时的，因此可以在窗口结束前预测最终结算方向。

**执行流程**：
1. 监控实时 BTC 现货价格（Binance + Coinbase 加权平均）
2. 跟踪 Chainlink 预言机价格，检测延迟
3. 在 30 秒时间窗口内计算价格趋势
4. 窗口结束前 10 秒做出方向预测
5. 置信度 ≥ 55% 且价格变化 ≥ 0.01% 时执行交易

## 架构设计

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│  Binance WS │────▶│              │     │  React 仪表盘│
│  Coinbase WS│────▶│  Go 策略引擎  │────▶│  (Vite)     │
│  Chainlink  │────▶│              │     │  Port 3456  │
└─────────────┘     └──────┬───────┘     └──────▲──────┘
                           │                     │
                    ┌──────▼───────┐     ┌──────┴──────┐
                    │  Python 签名  │     │  REST API   │
                    │  (py-clob)   │     │  Port 9876  │
                    └──────────────┘     └─────────────┘
```

- **Go（监听与计算层）**：WebSocket 长连接、实时价格监控、交易信号计算、风险管理
- **Python（执行与签名层）**：EIP-712 签名、Polymarket L2 鉴权（HMAC-SHA256）、`py-clob-client` SDK
- **React 仪表盘**：实时监控 UI（Vite + TypeScript + Tailwind + Recharts）

## 项目结构

```
poly-scan/
├── cmd/
│   ├── main.go                # 主程序入口
│   ├── api/main.go            # 仪表盘 API 服务器（REST + WebSocket）
│   ├── btc/main.go            # BTC 策略独立入口
│   └── test/main.go           # 集成测试
├── internal/
│   ├── btc/                   # 核心策略模块
│   │   ├── strategy.go        # 主交易循环、市场扫描、执行
│   │   ├── predictor.go       # Chainlink 监控与预测
│   │   ├── spot_monitor.go    # 实时 BTC 价格（Binance/Coinbase 加权）
│   │   ├── risk_manager.go    # Kelly 仓位、敞口限制、回撤控制
│   │   ├── performance.go     # PnL 追踪、交易记录
│   │   └── indicators.go      # RSI、MACD、布林带、ATR、动量
│   ├── execution/
│   │   └── engine.go          # 订单执行引擎，Go → Python 桥接
│   └── polymarket/
│       ├── api.go             # Gamma API + REST CLOB 客户端
│       └── ws.go              # WebSocket 客户端与心跳保活
├── dashboard/                 # React 监控仪表盘
│   └── src/
│       ├── App.tsx            # 布局、路由、告警
│       ├── api.ts             # API 服务 + WebSocket 客户端
│       ├── types.ts           # TypeScript 接口定义
│       └── pages/             # Dashboard, Markets, Trades, Risk, Settings
├── scripts/
│   └── executor.py            # Python 订单签名与执行
├── ecosystem.config.js        # PM2 进程配置
├── start.sh                   # 启动脚本
└── data/                      # 运行时数据（不提交）
    ├── performance.json       # 交易绩效数据
    └── positions.json         # 持仓记录
```

## 环境要求

- **Go 1.18+**
- **Python 3.8+** + `py-clob-client`
- **Node.js 18+**（仪表盘）
- 带有 Polygon 链 USDC.e 的 EVM 钱包

## 快速开始

### 1. 编译程序

```bash
go mod tidy
go build -o poly-bot ./cmd/main.go
go build -o poly-bot-api ./cmd/api/main.go
```

### 2. 安装依赖

```bash
# Python 依赖
pip3 install py-clob-client

# 仪表盘依赖
cd dashboard && npm install
```

### 3. 配置环境变量

```bash
# 创建 .env 文件
cat > .env << 'EOF'
POLY_PRIVATE_KEY=0x你的以太坊钱包私钥
POLY_SIGNATURE_TYPE=2
POLY_FUNDER_ADDRESS=0x你的Proxy钱包地址
EOF
```

> ⚠️ API Key/Secret/Passphrase 由私钥自动派生，无需手动配置。

### 4. 运行

#### 方式一：直接运行

```bash
source .env && ./poly-bot
```

#### 方式二：PM2 托管（推荐）

```bash
# 启动所有服务
pm2 start ecosystem.config.js

# 仅启动交易机器人
pm2 start ecosystem.config.js --only poly-bot-btc

# 查看日志
pm2 logs poly-bot-btc

# 重启 / 停止
pm2 restart poly-bot-btc
pm2 stop poly-bot-btc
```

## 监控仪表盘

仪表盘提供五个核心页面：

| 页面 | 功能 |
|------|------|
| **Dashboard** | 策略状态概览、实时 BTC 价格、PnL 曲线、胜率统计 |
| **Markets** | 活跃市场列表、订单簿深度、流动性分析 |
| **Trades** | 交易历史、每笔 PnL、方向筛选、分页 |
| **Risk** | 风险指标、最大回撤、敞口分布、Kelly 参数 |
| **Settings** | 策略参数配置、风控阈值调整 |

### 启动仪表盘

```bash
# API 服务器（默认端口 9876）
pm2 start ecosystem.config.js --only poly-bot-api

# 前端（默认端口 3456）
pm2 start ecosystem.config.js --only poly-dashboard
```

访问 `http://你的服务器IP:3456` 查看仪表盘。

## 配置参数

### BTC 策略配置

| 参数 | 默认值 | 说明 |
|------|--------|------|
| WindowDuration | 5m | 市场窗口时长 |
| PredictBeforeEnd | 30s | 窗口结束前预测启动时间 |
| MinConfidence | 0.55 | 最小置信度阈值 |
| MaxPositionSize | 50.0 | 最大单笔仓位 (USD) |
| MinPriceChangePct | 0.01% | 最小价格变化百分比 |
| MaxTokenPrice | 0.60 | 最大可买入代币价格 |

### 风险配置

| 参数 | 默认值 | 说明 |
|------|--------|------|
| MaxGlobalExposure | 10000 | 最大总敞口 (USD) |
| MaxPositionPerMarket | 500 | 单市场最大仓位 (USD) |
| MaxDailyLoss | 500 | 最大日损失 (USD) |
| MaxDrawdownPct | 0.20 | 最大回撤百分比 |
| KellyFraction | 0.25 | Kelly 分数 (1/4 Kelly) |

## 环境变量

| 变量 | 必需 | 说明 |
|------|------|------|
| POLY_PRIVATE_KEY | ✅ | 以太坊私钥（0x 前缀），API 凭证由此派生 |
| POLY_SIGNATURE_TYPE | ❌ | 签名类型：0=EOA, 1=Proxy, 2=Gnosis Safe（默认 2）|
| POLY_FUNDER_ADDRESS | ❌ | Proxy/Gnosis 钱包地址 |
| API_PORT | ❌ | 仪表盘 API 端口（默认 9876）|

## Go → Python 桥接

Go 通过 `exec.Command` 调用 `python3 scripts/executor.py`：
- 订单数据以 JSON 通过 stdin 传递
- 凭证通过环境变量传递（不落盘）
- 执行结果以 JSON 通过 stdout 返回

## 注意事项

1. **市场结算快**：每个窗口仅 5 分钟，交易后几秒内结算
2. **最小订单量**：Polymarket 最小订单为 5 股
3. **Gnosis Safe**：需设置 `POLY_SIGNATURE_TYPE=2` 和 `POLY_FUNDER_ADDRESS`
4. **安全提醒**：私钥仅存储在 `.env` 文件中，严禁提交到版本控制

## 免责声明

本工具仅供学习与技术交流使用。加密货币预测市场具有极高的波动性，使用本工具进行真实交易可能导致资金损失。**作者对任何使用本代码造成的经济损失不承担任何责任。** 请在充分理解风险并经过长期模拟测试后再投入真实资金。
