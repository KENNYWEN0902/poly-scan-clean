# polymarketbot 本地配置与服务器运行教程

更新时间：2026-04-24

适合场景：

- 你已经把项目下载到本地 Mac
- 你想先在本地整理配置
- 然后再上传到 Ubuntu 22.04 服务器运行

---

## 你的本地项目位置

我已经帮你下载到了这个目录：

```text
/Users/kennywen/Documents/Playground/polymarketbot
```

> 重要：`/Users/kennywen/Documents/Playground/polymarketbot` 现在是正式项目目录。此前临时修复目录 `/Users/kennywen/Documents/Codex/2026-04-21-ubantu-22-04-lts-bot-github/migration-polymarketbot` 中的修复已经同步回正式项目，后续请只从 Playground 目录上传服务器。

## 2026-04-24 已同步修复

本轮已验证并同步这些运行修复：

- ChainlinkMonitor 可连接 Polymarket RTDS，并显示 `RTDS BTC/USD price established`。
- `/api/account` 可读取 CLOB collateral/pUSD 余额，并使用短超时与缓存避免页面卡死。
- Dashboard 历史页在没有交易时正常显示空状态。
- Dashboard 所有页面都能打开，左下角余额和 WebSocket 在线状态正常。
- `/api/trades` 没有交易时返回 `[]`，这是正常结果。

服务器替换代码后，建议执行：

```bash
cd /root/poly-scan
gofmt -w cmd/api/main.go internal/btc/chainlink_monitor.go internal/btc/predictor.go internal/polymarket/api.go
go build -o poly-bot-api ./cmd/api
go build -o poly-bot ./cmd/main.go

cd /root/poly-scan/dashboard
npm install
npm run build

cd /root/poly-scan
pm2 restart poly-bot-api --update-env
pm2 restart poly-bot-btc --update-env
pm2 restart poly-dashboard --update-env

curl -s --max-time 5 http://127.0.0.1:9876/api/account
curl -s --max-time 5 http://127.0.0.1:9876/api/trades
```

---

## 先说最推荐的做法

建议你这样操作：

1. 本地只修改“非敏感参数”
2. 本地不要保存真实私钥
3. 上传到服务器后，再创建真正的 `.env`
4. 编译和运行都在 Ubuntu 服务器上完成

原因很简单：

- 你的 Mac `Documents` 目录可能会被同步或备份
- 私钥如果先保存在本地项目里，风险更高
- 这个 bot 最终运行环境是 Ubuntu，不是本地 Mac

---

## 这个 bot 最关键的 3 个参数

如果你现在走的是“浏览器钱包登录 Polymarket”的路线，那么通常是：

```env
POLY_PRIVATE_KEY=0x你的浏览器钱包私钥
POLY_SIGNATURE_TYPE=2
POLY_FUNDER_ADDRESS=0x你在Polymarket个人资料页看到的地址
```

说明：

- `POLY_PRIVATE_KEY`
  - 这是你 Rabby / MetaMask 钱包私钥
  - 建议只在服务器上填写

- `POLY_SIGNATURE_TYPE=2`
  - 这是你当前最适合的新手配置

- `POLY_FUNDER_ADDRESS`
  - 用 Polymarket 个人资料页中的地址
  - 不是充值弹窗里的收款地址

---

## 本地应该改什么

### 推荐：本地只准备一个模板文件

你可以使用我给你准备的：

```text
env.server.template
```

这个文件适合你先把“非敏感配置”填好，例如：

- `POLY_SIGNATURE_TYPE=2`
- `POLY_FUNDER_ADDRESS=你的个人资料页地址`
- `POLY_TRADE_AMOUNT=5`
- `API_PORT=9876`

但请不要在本地填写真实：

- `POLY_PRIVATE_KEY`
- `POLY_API_SECRET`
- `POLY_PASSPHRASE`

### 本地还可以改什么

你还可以在本地做这些事情：

- 看项目文件结构
- 阅读 `README.md`
- 修改你自己的说明文件
- 准备上传到服务器的文件夹结构

### 本地不建议做什么

- 不建议在本地 Mac 上真实运行交易
- 不建议把真实私钥写进项目目录后再上传
- 不建议改 `ecosystem.config.js`，除非你确定服务器不会用 `/root/poly-scan`

---

## 本地配置步骤

### 第 1 步：打开项目目录

如果你想在终端里进入项目目录：

```bash
cd /Users/kennywen/Documents/Playground/polymarketbot
```

### 第 2 步：查看我给你的模板

```bash
cat env.server.template
```

### 第 3 步：本地填写非敏感部分

你可以先打开模板：

```bash
nano env.server.template
```

把它改成类似这样：

```env
POLY_PRIVATE_KEY=0xREPLACE_ON_SERVER_ONLY
POLY_SIGNATURE_TYPE=2
POLY_FUNDER_ADDRESS=0x你个人资料页里的地址
POLY_TAKE_PROFIT_PCT=0.50
POLY_STOP_LOSS_PCT=0.30
POLY_TRADE_AMOUNT=5
API_PORT=9876
```

这里最重要的是：

- 私钥先保留占位符
- `POLY_FUNDER_ADDRESS` 可以先填
- 交易金额建议先从 `5` 开始

---

## 上传到服务器前，你有两种方式

## 方式 A：整个文件夹直接上传

适合完全新手，最简单。

假设你的服务器 IP 是：

```text
1.2.3.4
```

登录用户是：

```text
root
```

那你可以在 Mac 终端执行：

```bash
scp -r /Users/kennywen/Documents/Playground/polymarketbot root@1.2.3.4:/root/poly-scan
```

注意：

- 这条命令会把整个目录上传到服务器
- 服务器上最终路径会变成 `/root/poly-scan`

如果你服务器不是 root，而是 `ubuntu` 用户：

```bash
scp -r /Users/kennywen/Documents/Playground/polymarketbot ubuntu@1.2.3.4:/home/ubuntu/poly-scan
```

之后再登录服务器：

```bash
ssh ubuntu@1.2.3.4
sudo -i
mv /home/ubuntu/poly-scan /root/poly-scan
```

## 方式 B：用图形化工具上传

如果你不想记命令，也可以用：

- FileZilla
- Cyberduck
- FinalShell

上传目标建议仍然是：

```text
/root/poly-scan
```

这样你就不用改项目里的 PM2 路径配置。

---

## 上传到服务器后，如何配置

### 第 1 步：登录服务器

```bash
ssh root@你的服务器IP
```

### 第 2 步：进入项目目录

```bash
cd /root/poly-scan
```

### 第 3 步：从模板生成真正的 `.env`

如果你已经把 `env.server.template` 一起上传到了服务器，那么可以执行：

```bash
cp env.server.template .env
nano .env
```

### 第 4 步：在服务器里补上真实私钥

把：

```env
POLY_PRIVATE_KEY=0xREPLACE_ON_SERVER_ONLY
```

改成：

```env
POLY_PRIVATE_KEY=0x你的真实钱包私钥
```

同时确认下面两项已经正确：

```env
POLY_SIGNATURE_TYPE=2
POLY_FUNDER_ADDRESS=0x你个人资料页中的地址
```

### 第 5 步：最终示例 `.env`

```env
POLY_PRIVATE_KEY=0x你的真实钱包私钥
POLY_SIGNATURE_TYPE=2
POLY_FUNDER_ADDRESS=0x你个人资料页中的地址
POLY_TAKE_PROFIT_PCT=0.50
POLY_STOP_LOSS_PCT=0.30
POLY_TRADE_AMOUNT=5
API_PORT=9876
```

---

## 服务器如何安装依赖

下面步骤是在 Ubuntu 22.04 服务器里执行。

### 第 1 步：更新系统

```bash
apt update
apt upgrade -y
```

### 第 2 步：安装基础依赖

```bash
apt install -y git curl ca-certificates gnupg lsb-release build-essential python3 python3-pip python3-venv nginx golang-go
```

### 第 3 步：安装 Node.js 22

```bash
curl -fsSL https://deb.nodesource.com/setup_22.x | bash -
apt install -y nodejs
node -v
npm -v
```

### 第 4 步：安装 PM2 和 serve

```bash
npm install -g pm2 serve
pm2 -v
serve -v
```

### 第 5 步：安装 Python 依赖

```bash
cd /root/poly-scan
python3 -m pip install --upgrade pip
python3 -m pip install py-clob-client-v2 requests web3 eth-account eth-abi
```

> 从 2026-04-28 约 11:00 UTC 起，Polymarket CLOB V2 正式接管生产环境，旧 `py-clob-client` 将不再兼容。联调阶段如果你想提前对接 V2，可在 `.env` 里增加 `POLY_CLOB_HOST=https://clob-v2.polymarket.com`。另外，V2 的交易抵押资产已经切到 `pUSD`，下单前要先把 `USDC.e` 包装成 `pUSD`。

---

## 服务器如何编译

### 第 1 步：编译后端

```bash
cd /root/poly-scan
go mod tidy
go build -o poly-bot ./cmd/main.go
go build -o poly-bot-api ./cmd/api/main.go
```

### 第 2 步：构建 dashboard

```bash
cd /root/poly-scan/dashboard
npm install
npm run build
cd /root/poly-scan
```

### 第 3 步：创建运行目录

```bash
mkdir -p /root/poly-scan/logs
mkdir -p /root/poly-scan/data
```

---

## 服务器如何先做安全测试

在真正跑实盘前，建议先做两个测试。

### 测试 1：只测 API

```bash
cd /root/poly-scan
./poly-bot-api
```

另开一个 SSH 窗口执行：

```bash
curl http://127.0.0.1:9876/api/strategy
```

如果返回 JSON，说明 API 正常。

### 测试 2：测试主 bot

```bash
cd /root/poly-scan
./poly-bot
```

如果程序能启动、没有立刻报错退出，说明主程序基础上是通的。

---

## 服务器如何正式运行

## 方式一：直接运行

适合临时测试。

```bash
cd /root/poly-scan
./poly-bot
```

如果要启动 API：

```bash
cd /root/poly-scan
./poly-bot-api
```

## 方式二：用 PM2 托管

更推荐。

### 启动 API

```bash
cd /root/poly-scan
pm2 start ecosystem.config.js --only poly-bot-api
```

### 启动前端 dashboard

```bash
cd /root/poly-scan
pm2 start ecosystem.config.js --only poly-dashboard
```

### 启动 bot

```bash
cd /root/poly-scan
pm2 start ecosystem.config.js --only poly-bot-btc
```

### 查看状态

```bash
pm2 status
```

### 查看日志

```bash
pm2 logs poly-bot-btc --lines 50
pm2 logs poly-bot-api --lines 50
pm2 logs poly-dashboard --lines 50
```

### 设置开机自启

```bash
pm2 save
pm2 startup systemd
```

然后把终端输出的那条命令复制执行一次。

---

## 如果你想通过浏览器访问 dashboard

你还需要 Nginx 反向代理。

### 新建 Nginx 配置

```bash
nano /etc/nginx/sites-available/poly-bot
```

粘贴：

```nginx
server {
    listen 80;
    server_name _;

    location / {
        proxy_pass http://127.0.0.1:3457;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /api/ {
        proxy_pass http://127.0.0.1:9876;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /ws {
        proxy_pass http://127.0.0.1:9876/ws;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

启用配置：

```bash
ln -sf /etc/nginx/sites-available/poly-bot /etc/nginx/sites-enabled/poly-bot
rm -f /etc/nginx/sites-enabled/default
nginx -t
systemctl restart nginx
```

浏览器访问：

```text
http://你的服务器IP
```

---

## 最后给你的实操建议

对你现在最稳的流程是：

1. 本地先看懂项目结构
2. 本地只改非敏感配置
3. 上传到服务器路径 `/root/poly-scan`
4. 服务器里创建真正的 `.env`
5. 先小额测试
6. 确认日志正常后再长期运行

如果你只记一句话：

- `POLY_FUNDER_ADDRESS` 用个人资料页地址
- 充值用充值弹窗地址
- 私钥只在服务器上填写
