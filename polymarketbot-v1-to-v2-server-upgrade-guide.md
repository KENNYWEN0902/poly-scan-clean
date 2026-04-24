# PolymarketBot 服务器从 V1 升级到 V2 的详细教程

更新时间：2026-04-24

这是一份给“代码和服务器小白”准备的教程。你可以把它理解成：

1. 先在服务器备份现状
2. 只修改必须改的配置
3. 只上传必须更新的文件
4. 安装 V2 依赖
5. 检查 `pUSD`
6. 做一次连接测试
7. 重启机器人
8. 到官方切换日再做最后一次小改动

这份教程默认你的情况是：

- 你已经有一台服务器，并且 V1 版本已经在运行
- 你的项目目录在服务器上大概率是 `/root/poly-scan`
- 你的本地 Mac 项目目录是 `/Users/kennywen/Documents/Playground/polymarketbot`
- 你不想覆盖整个服务器项目目录
- 你不想重建服务器上的 `.env`
- 你有自己的策略文件，不希望被我这次迁移影响

如果你的目录不是 `/root/poly-scan`，把教程里所有 `/root/poly-scan` 换成你的真实目录即可。

> 2026-04-24 补充：正式本地项目目录已经统一为 `/Users/kennywen/Documents/Playground/polymarketbot`。此前在临时 Codex 目录里修复的 RTDS、pUSD 余额、Dashboard 历史页、API 超时缓存和 WebSocket 状态问题，已经同步回正式项目。后续上传服务器时请以 Playground 目录为准。

---

## 先说最重要的结论

### 1. 服务器上的 `.env` 不需要整体重建

如果你服务器上的 `.env` 之前能跑 V1，这次不要整体覆盖它。

你只需要手动检查这几项：

- `POLY_PRIVATE_KEY`
  - 一般不改

- `POLY_SIGNATURE_TYPE`
  - 一般不改

- `POLY_FUNDER_ADDRESS`
  - 一般不改

- `POLY_CLOB_HOST`
  - 如果你现在就要提前试跑 V2，就加：
    - `POLY_CLOB_HOST=https://clob-v2.polymarket.com`
  - 到 `2026-04-28 19:00` 北京时间左右官方正式切换后：
    - 删除这一行，或者改成 `https://clob.polymarket.com`

- `POLY_RPC_URL`
  - 可选，但推荐
  - 如果你有更稳定的 Polygon RPC，可以加：
    - `POLY_RPC_URL=https://polygon-mainnet.g.alchemy.com/v2/你的KEY`

- `POLY_BUILDER_CODE`
  - 只有你自己明确有 builder code 时才填
  - 没有就不要加

### 2. 你的策略文件这次不要动

下面这 3 个文件是你的策略逻辑：

- `internal/btc/predictor.go`
- `internal/btc/risk_manager.go`
- `internal/btc/strategy.go`

这次服务器升级不要上传它们，也不要覆盖它们。

我已经把迁移兼容点放在别的地方了，所以这次升级不依赖改你的策略文件。

### 3. 这次最小升级必须更新哪些文件

如果你现在的目标只是“让机器人能走 V2 下单和 pUSD 路线”，最小需要更新：

- `scripts/executor.py`
- `ecosystem.config.js`

如果你还要同步 2026-04-24 已验证的 RTDS 与 Dashboard 稳定性修复，还需要更新：

- `cmd/api/main.go`
- `cmd/api/main_test.go`
- `internal/btc/chainlink_monitor.go`
- `internal/btc/predictor.go`
- `internal/polymarket/api.go`
- `dashboard/src/App.tsx`
- `dashboard/src/api.ts`
- `dashboard/src/pages/PnlHistory.tsx`
- `dashboard/src/pages/Portfolio.tsx`
- `dashboard/src/pages/Positions.tsx`
- `dashboard/src/pages/Settings.tsx`

推荐一起更新：

- `scripts/recover_positions.py`
- `scripts/ctf_redeem.py`
- `scripts/claim_rewards.py`
- `scripts/wrap_pusd.py`

如果你还想让 API / Dashboard 也显示 `pUSD`，再额外更新：

- `cmd/api/main.go`
- `dashboard/src/App.tsx`
- `dashboard/src/pages/Positions.tsx`
- `dashboard/src/pages/Settings.tsx`
- `dashboard/src/types.ts`

### 4. 为什么 `ecosystem.config.js` 也建议更新

这是新手最容易忽略的一点。

很多人以为只改 `.env` 就够了，但你现在是用 PM2 跑服务。如果服务器上的 `ecosystem.config.js` 还是旧版，它可能不会把新的：

- `POLY_CLOB_HOST`
- `POLY_RPC_URL`
- `POLY_BUILDER_CODE`

传递给 `poly-bot-btc` 进程。结果就是：

- 你明明改了 `.env`
- 但机器人进程实际没有读到新变量
- 最后表现成“改了配置却没生效”

所以这份教程里把 `ecosystem.config.js` 放在“最小升级建议一起更新”里。

---

## 官方切换时间，你要记住这个时间

Polymarket 官方文档目前写的是：

- CLOB V2 正式切换时间：`2026-04-28 约 11:00 UTC`
- 北京时间约等于：`2026-04-28 19:00`

官方还说明：

- 切换时会有大约 1 小时停机
- 所有挂单会被清空
- 旧的 V1 客户端 / 旧 SDK 集成会失效

所以你的实际策略是：

- `2026-04-22` 到 `2026-04-28 19:00` 北京时间之前
  - V1 还能继续跑
  - 你也可以提前试跑 V2，但此时要连 `https://clob-v2.polymarket.com`

- `2026-04-28 19:00` 北京时间之后
  - 必须使用 V2
  - 不再继续用 V1

---

## 这份教程到底会做什么，不会做什么

### 会做什么

- 只更新 V2 必要文件
- 不覆盖整个项目目录
- 不重建你的 `.env`
- 不碰你的策略文件
- 给你一套可以直接复制的命令
- 给你一套出错后的回滚方案

### 不会做什么

- 不会删除你服务器上的旧项目目录
- 不会自动改你的策略参数
- 不会替你决定交易策略
- 不会把你的服务器整个重装

---

## 先认识 4 个词，小白一定要先看

### 1. 本地 Mac

就是你手上的电脑。

你本地项目目录是：

```text
/Users/kennywen/Documents/Playground/polymarketbot
```

### 2. 服务器

就是你现在跑 V1 机器人的那台远程 Linux 机器。

教程里默认项目目录是：

```text
/root/poly-scan
```

### 3. 上传文件

“上传”就是把 Mac 上的某一个文件，复制到服务器同名位置。

我们这次不做“整个文件夹覆盖”，而是做“逐个文件上传”。

### 4. PM2

PM2 是你用来启动和守护进程的工具。

你现在大概率已经用它在跑这些服务：

- `poly-bot-btc`
- `poly-bot-api`
- `poly-dashboard`

---

## 升级前，你先准备这 5 样东西

在真正开始之前，请你先确认：

1. 你知道服务器 IP
2. 你能正常 `ssh` 登录服务器
3. 你知道服务器上的项目目录
4. 你知道你是不是在用 PM2
5. 你有服务器当前 `.env` 的权限

如果你不确定服务器项目目录是不是 `/root/poly-scan`，先登录服务器后执行：

```bash
pwd
ls
pm2 list
```

如果你发现项目根本不在 `/root/poly-scan`，比如在 `/home/ubuntu/poly-scan`，那后面所有命令都要把路径换掉。

---

## 升级总流程，一张图先看懂

```text
Mac 本地准备更新文件
        ↓
登录服务器确认目录
        ↓
备份服务器现状
        ↓
检查并补 `.env` 的必要字段
        ↓
从 Mac 上传少量指定文件
        ↓
服务器安装 py-clob-client-v2
        ↓
检查 pUSD 是否已经准备好
        ↓
测试 executor.py 是否能连上 V2
        ↓
重启 PM2 服务
        ↓
看日志确认没报错
        ↓
等到 2026-04-28 19:00 北京时间再做最后切换
```

---

## 第 0 步：先确认你现在到底在改哪台机器

这一步很重要，因为新手最容易把命令敲错地方。

### 你现在会在两个终端之间来回切换

#### 终端 A：Mac 本地终端

这个终端负责：

- 查看本地代码
- 上传文件到服务器

#### 终端 B：服务器终端

这个终端负责：

- 安装备份
- 修改服务器 `.env`
- 安装依赖
- 重启进程

### 如何登录服务器

在 Mac 本地终端执行：

```bash
ssh root@你的服务器IP
```

如果你不是 `root` 用户，比如你用的是 `ubuntu`，那就改成：

```bash
ssh ubuntu@你的服务器IP
```

登录成功后，先执行：

```bash
whoami
pwd
ls /root
pm2 list
```

你应该至少能看见以下其中一部分信息：

- 当前用户是谁
- 当前目录在哪
- `/root/poly-scan` 是否存在
- PM2 里有哪些进程在跑

如果 `pm2: command not found`，说明你的 PM2 不在当前 PATH，需要先找到你的 Node/PM2 安装方式，这种情况先不要继续做升级。

---

## 第 1 步：先做备份，这是最重要的一步

进入服务器项目上一级目录：

```bash
cd /root
```

执行备份：

```bash
cp -a poly-scan poly-scan.backup-$(date +%F-%H%M%S)
cp /root/poly-scan/.env /root/poly-scan/.env.backup-$(date +%F-%H%M%S)
```

这两条命令做了两件事：

- 备份整个项目目录
- 单独再备份一份 `.env`

### 如何确认备份成功

执行：

```bash
ls -ld /root/poly-scan.backup-*
ls -l /root/poly-scan/.env.backup-*
```

如果能看到带时间戳的新目录和新文件，说明备份成功。

### 如果项目目录不是 `/root/poly-scan`

例如你的实际目录是 `/home/ubuntu/poly-scan`，那命令要改成：

```bash
cd /home/ubuntu
cp -a poly-scan poly-scan.backup-$(date +%F-%H%M%S)
cp /home/ubuntu/poly-scan/.env /home/ubuntu/poly-scan/.env.backup-$(date +%F-%H%M%S)
```

---

## 第 2 步：先确认服务器 `.env` 当前长什么样

进入项目目录：

```bash
cd /root/poly-scan
```

查看所有 `POLY_` 开头的配置：

```bash
grep -n '^POLY_' .env
```

你要重点看这几项是否已经存在：

- `POLY_PRIVATE_KEY`
- `POLY_SIGNATURE_TYPE`
- `POLY_FUNDER_ADDRESS`
- `POLY_CLOB_HOST`
- `POLY_RPC_URL`
- `POLY_BUILDER_CODE`
- `POLY_API_KEY`
- `POLY_API_SECRET`
- `POLY_PASSPHRASE`

### 你应该怎么判断要不要改

#### 一般不用改的

如果下面 3 项已经是正确值，保持不动：

```env
POLY_PRIVATE_KEY=...
POLY_SIGNATURE_TYPE=...
POLY_FUNDER_ADDRESS=...
```

#### 你现在就想试跑 V2

你需要有这一行：

```env
POLY_CLOB_HOST=https://clob-v2.polymarket.com
```

#### 你不想现在试跑 V2，只想先把文件升级好

那你可以先不加 `POLY_CLOB_HOST`，等接近官方切换时间再加或再改。

#### 你有更稳定的 Polygon RPC

推荐加：

```env
POLY_RPC_URL=https://polygon-mainnet.g.alchemy.com/v2/你的KEY
```

#### 你没有 builder code

不要乱填 `POLY_BUILDER_CODE`。

### 旧 API 三件套怎么处理

如果你看到这些旧值：

```env
POLY_API_KEY=...
POLY_API_SECRET=...
POLY_PASSPHRASE=...
```

建议改成注释：

```env
# POLY_API_KEY=
# POLY_API_SECRET=
# POLY_PASSPHRASE=
```

这样做的原因是：

- V2 更推荐通过新 SDK 自动创建或派生 API key
- 旧值留着不一定立刻报错，但容易让你后面排查问题时搞混

### 如何编辑 `.env`

执行：

```bash
nano .env
```

如果你没用过 `nano`，只记住这 4 个操作：

- 方向键：移动光标
- `Ctrl + O`：保存
- 回车：确认保存文件名
- `Ctrl + X`：退出编辑器

### 编辑时注意 3 件事

1. 不要把同一个变量写两遍  
   例如不要同时出现两行：
   `POLY_CLOB_HOST=...`

2. 如果原来已经有这一行，就直接改那一行，不要新增重复行

3. 如果原来没有这一行，就加到文件最后面即可

### 一个最常见的推荐写法

如果你现在就要提前试跑 V2，`.env` 建议至少长这样：

```env
POLY_PRIVATE_KEY=0x你的私钥
POLY_SIGNATURE_TYPE=2
POLY_FUNDER_ADDRESS=0x你的funder地址
POLY_CLOB_HOST=https://clob-v2.polymarket.com
# POLY_RPC_URL=https://polygon-mainnet.g.alchemy.com/v2/你的KEY
# POLY_BUILDER_CODE=0x...
# POLY_API_KEY=
# POLY_API_SECRET=
# POLY_PASSPHRASE=
```

这里最关键的是：

- 前三项保持你原来的正确值
- `POLY_CLOB_HOST` 指向 `clob-v2`
- 旧 API 三件套不要再依赖

---

## 第 3 步：从 Mac 本地上传“最小升级文件”

这一步在 Mac 本地终端执行，不是在服务器终端执行。

你的本地项目目录是：

```text
/Users/kennywen/Documents/Playground/polymarketbot
```

### 最小升级，建议先上传这 6 个文件

```bash
scp /Users/kennywen/Documents/Playground/polymarketbot/scripts/executor.py \
root@你的服务器IP:/root/poly-scan/scripts/executor.py

scp /Users/kennywen/Documents/Playground/polymarketbot/ecosystem.config.js \
root@你的服务器IP:/root/poly-scan/ecosystem.config.js

scp /Users/kennywen/Documents/Playground/polymarketbot/scripts/recover_positions.py \
root@你的服务器IP:/root/poly-scan/scripts/recover_positions.py

scp /Users/kennywen/Documents/Playground/polymarketbot/scripts/ctf_redeem.py \
root@你的服务器IP:/root/poly-scan/scripts/ctf_redeem.py

scp /Users/kennywen/Documents/Playground/polymarketbot/scripts/claim_rewards.py \
root@你的服务器IP:/root/poly-scan/scripts/claim_rewards.py

scp /Users/kennywen/Documents/Playground/polymarketbot/scripts/wrap_pusd.py \
root@你的服务器IP:/root/poly-scan/scripts/wrap_pusd.py
```

### 这 6 个文件分别是做什么的

- `scripts/executor.py`
  - 最重要
  - 它负责真正和 Polymarket CLOB 交互
  - 这次 V2 迁移的核心就在这里

- `ecosystem.config.js`
  - 负责把 `.env` 里的新变量传给 PM2 进程

- `scripts/recover_positions.py`
  - 跟恢复仓位相关

- `scripts/ctf_redeem.py`
  - 跟赎回结算后仓位相关

- `scripts/claim_rewards.py`
  - 跟奖励领取相关

- `scripts/wrap_pusd.py`
  - 跟 `pUSD` 包装/解包相关

### 明确不要上传什么

下面这些文件这次不要上传：

```text
internal/btc/predictor.go
internal/btc/risk_manager.go
internal/btc/strategy.go
.env
```

原因：

- 前 3 个是你的策略文件
- `.env` 是服务器自己的配置文件
- 这次升级完全没必要覆盖它们

### 如果你服务器登录用户不是 root

例如你是 `ubuntu` 用户，命令改成：

```bash
scp /Users/kennywen/Documents/Playground/polymarketbot/scripts/executor.py \
ubuntu@你的服务器IP:/home/ubuntu/poly-scan/scripts/executor.py
```

其他文件同理替换。

### 如果你想一次复制多条命令

你可以把上面那 6 条一起复制到 Mac 终端执行，系统会逐个上传。

如果其中一条失败，不影响其他已经成功的文件。

---

## 第 4 步：如果你还想连 API / Dashboard 一起升级

只有在你需要下面这些效果时，才做这一步：

- 页面上显示 `pUSD`
- API 返回 `collateral_balance`
- 仪表盘文案不再继续写旧 `USDC`

如果你现在只关心机器人能不能交易，这一步可以先跳过。

### 额外上传的文件

在 Mac 本地终端执行：

```bash
scp /Users/kennywen/Documents/Playground/polymarketbot/cmd/api/main.go \
root@你的服务器IP:/root/poly-scan/cmd/api/main.go

scp /Users/kennywen/Documents/Playground/polymarketbot/dashboard/src/App.tsx \
root@你的服务器IP:/root/poly-scan/dashboard/src/App.tsx

scp /Users/kennywen/Documents/Playground/polymarketbot/dashboard/src/pages/Positions.tsx \
root@你的服务器IP:/root/poly-scan/dashboard/src/pages/Positions.tsx

scp /Users/kennywen/Documents/Playground/polymarketbot/dashboard/src/pages/Settings.tsx \
root@你的服务器IP:/root/poly-scan/dashboard/src/pages/Settings.tsx

scp /Users/kennywen/Documents/Playground/polymarketbot/dashboard/src/types.ts \
root@你的服务器IP:/root/poly-scan/dashboard/src/types.ts
```

---

## 第 5 步：回到服务器，检查文件是否真的到了

切回服务器终端，执行：

```bash
cd /root/poly-scan
ls -l scripts/executor.py
ls -l ecosystem.config.js
ls -l scripts/recover_positions.py
ls -l scripts/ctf_redeem.py
ls -l scripts/claim_rewards.py
ls -l scripts/wrap_pusd.py
```

如果你还更新了 API / Dashboard，再执行：

```bash
ls -l cmd/api/main.go
ls -l dashboard/src/App.tsx
ls -l dashboard/src/pages/Positions.tsx
ls -l dashboard/src/pages/Settings.tsx
ls -l dashboard/src/types.ts
```

### 怎么判断上传成功

如果 `ls -l` 能正常显示文件大小和时间，说明文件已经上传到位。

如果你看到：

```text
No such file or directory
```

说明：

- 要么你上传时路径写错了
- 要么服务器上的项目目录不是 `/root/poly-scan`

先不要继续，先把路径核对清楚。

---

## 第 6 步：安装 V2 Python 依赖

这一步在服务器执行。

先进入项目目录：

```bash
cd /root/poly-scan
```

先看 Python 版本：

```bash
python3 --version
```

你需要看到 `Python 3.9.10` 或更高版本。

例如：

- `Python 3.10.12`
- `Python 3.11.x`

都可以。

### 再升级 pip

```bash
python3 -m pip install --upgrade pip
```

### 卸载旧包

```bash
python3 -m pip uninstall -y py-clob-client
```

如果系统提示没装过这个包，也没关系。

### 安装新包

```bash
python3 -m pip install py-clob-client-v2 requests web3 eth-account eth-abi
```

### 做一次最简单的导入测试

```bash
python3 - <<'PY'
from py_clob_client_v2 import ClobClient
print("py-clob-client-v2 import ok")
PY
```

如果输出：

```text
py-clob-client-v2 import ok
```

说明新 SDK 已经装好。

### 如果安装时报错怎么办

#### 报错 1：`python3: command not found`

说明服务器上 Python 3 没装好，先不要继续本教程。

#### 报错 2：`No module named pip`

可以尝试：

```bash
python3 -m ensurepip --upgrade
python3 -m pip install --upgrade pip
```

#### 报错 3：Python 版本太低

如果你看到类似：

```text
py-clob-client-v2 requires Python 3.9.10+
```

那你必须先升级服务器 Python 版本，否则不能继续用 V2。

---

## 第 7 步：先判断你是不是 Safe / Proxy 路线

这一步很重要，因为它决定你怎么准备 `pUSD`。

在服务器执行：

```bash
cd /root/poly-scan
grep -n '^POLY_SIGNATURE_TYPE=' .env
grep -n '^POLY_FUNDER_ADDRESS=' .env
```

### 如果你看到

```env
POLY_SIGNATURE_TYPE=0
```

通常说明你更接近普通 EOA 钱包模式。

### 如果你看到

```env
POLY_SIGNATURE_TYPE=2
```

通常说明你走的是 Proxy / Safe / funder 相关模式。

---

## 第 8 步：准备 `pUSD`

V2 最关键的一点是：

- 下单抵押资产是 `pUSD`
- 不是只有 `USDC.e` 就够

### 先看余额

在服务器执行：

```bash
cd /root/poly-scan
set -a
source .env
set +a

python3 scripts/wrap_pusd.py balances
```

### 情况 A：你是普通 EOA 钱包

如果你确认你不是 Safe / Proxy 模式，而是普通钱包，你可以尝试：

```bash
python3 scripts/wrap_pusd.py wrap 100
python3 scripts/wrap_pusd.py balances
```

这表示尝试把 `100` 的 `USDC.e` 包装成 `pUSD`。

### 情况 B：你是 `POLY_SIGNATURE_TYPE=2`

如果你是：

```env
POLY_SIGNATURE_TYPE=2
POLY_FUNDER_ADDRESS=0x...
```

那我更推荐你用真正出资的钱包去 Polymarket 前端准备 `pUSD`，原因是：

- 服务器脚本是按当前私钥地址去发交易
- 但你真正的资金地址很多时候是 `POLY_FUNDER_ADDRESS`
- 如果这两者不是同一个地址，直接在服务器上 `wrap` 容易和真实出资地址不一致

最稳的办法：

1. 找到真正出资的钱包
2. 用这个钱包登录 Polymarket 前端
3. 在前端完成资金准备
4. 确认这个出资地址里已经有 `pUSD`

### 你要知道什么叫“准备好 pUSD”

不是看见钱包里有钱就算准备好。

真正准备好，指的是：

- 你的实际出资地址里有 `pUSD`
- 不是只有 `USDC.e`

---

## 第 9 步：先做一次最关键的连通性测试

这一步是整个升级里最关键的检查。

在服务器执行：

```bash
cd /root/poly-scan
set -a
source .env
set +a

echo '{"action":"check_collateral_balance"}' | python3 scripts/executor.py
```

### 什么结果算成功

你希望看到类似：

```json
{"success": true, "balance": 123.45}
```

这里的 `balance` 表示当前钱包在 CLOB 可用的 collateral 余额，也就是 V2 下单可用余额。

### 如果失败，不要启动机器人

先看报错内容。

常见错误如下：

#### `ModuleNotFoundError: py_clob_client_v2`

说明新 SDK 没安装好，回到“第 6 步”重新安装。

#### `Failed to create or derive API key`

说明：

- 私钥不对
- `POLY_SIGNATURE_TYPE` 不对
- `POLY_FUNDER_ADDRESS` 不对
- 或者旧 API 三件套干扰了新 SDK

先回头检查 `.env`。

#### `insufficient collateral`

说明你已经连上了，但可用抵押物不够，通常是 `pUSD` 不够。

#### `Failed to get pUSD balance`

说明链上余额查询或抵押资产状态有问题，先检查 RPC 和资金地址。

---

## 第 10 步：你到底要不要重新编译

这一点我专门给你写清楚。

### 情况 A：你只更新了 Python 执行层

如果你只上传了：

- `scripts/executor.py`
- `ecosystem.config.js`
- `scripts/recover_positions.py`
- `scripts/ctf_redeem.py`
- `scripts/claim_rewards.py`
- `scripts/wrap_pusd.py`

那么：

- `poly-bot` 二进制可以先不重新编译
- 因为真正连 Polymarket 的核心执行逻辑在 Python 脚本里

### 情况 B：你还更新了 `cmd/api/main.go`

那你需要重新编译 API：

```bash
cd /root/poly-scan
go build -o poly-bot-api ./cmd/api/main.go
```

如果服务器没有 `go` 命令，这一步先不要做，先维持旧 API。

### 情况 C：你还更新了 dashboard 文件

那你需要重新构建前端：

```bash
cd /root/poly-scan/dashboard
npm install
npm run build
```

如果你不关心页面上的 `pUSD` 文案，这一步可以以后再做。

---

## 第 11 步：如何重启 PM2，才会真的读到新配置

很多人会在这一步出问题。

如果你改过 `.env`，又更新过 `ecosystem.config.js`，建议你用下面方式重启。

### 只重启交易机器人

```bash
cd /root/poly-scan
pm2 restart ecosystem.config.js --only poly-bot-btc --update-env
```

### 如果你还更新了 API

```bash
pm2 restart ecosystem.config.js --only poly-bot-api --update-env
```

### 如果你还更新了 dashboard

```bash
pm2 restart ecosystem.config.js --only poly-dashboard --update-env
```

### 最后保存 PM2 当前状态

```bash
pm2 save
```

### 为什么不直接写 `pm2 restart poly-bot-btc`

因为你这次升级里有新环境变量，最稳妥的方式是让 PM2 重新从新版 `ecosystem.config.js` 加载。

---

## 第 12 步：看日志，确认机器人真的已经走 V2

先看交易机器人日志：

```bash
pm2 logs poly-bot-btc --lines 100
```

如果你还更新了 API：

```bash
pm2 logs poly-bot-api --lines 100
```

如果你还更新了 dashboard：

```bash
pm2 logs poly-dashboard --lines 100
```

### 你要重点找哪些报错

- `ModuleNotFoundError: py_clob_client_v2`
- `Failed to create or derive API key`
- `Failed to get pUSD balance`
- `insufficient collateral`
- `py-clob-client-v2 requires Python 3.9.10+`

### 什么情况说明大概率正常

你没有看到上面的错误，并且：

- 机器人没有反复重启
- `check_collateral_balance` 能成功
- 钱包里也确实有 `pUSD`

这时基本就说明 V2 路线已经通了。

---

## 第 13 步：如果你现在就想跑 V2，该怎么做

你可以现在就跑 V2，但你要明白“现在跑 V2”和“官方正式切换”不是一回事。

### 现在立刻试跑 V2 的条件

你至少要满足下面 4 条：

1. `.env` 里有：
   `POLY_CLOB_HOST=https://clob-v2.polymarket.com`

2. `scripts/executor.py` 已经是新版

3. `py-clob-client-v2` 已装好

4. 你的实际出资地址里已经有 `pUSD`

### 现在跑 V2 后，是否以后就完全不用管了

不是。

你现在提前切到 V2，确实可以避免以后再做一次“代码层面的 V1 -> V2 迁移”，但到官方切换当天你还是要关注：

- 官方停机窗口
- 挂单清空
- `POLY_CLOB_HOST` 是否要从 `clob-v2` 改回正式主地址

---

## 第 14 步：到 2026-04-28 19:00 北京时间当天，你要做什么

官方切换时间约为：

- `2026-04-28 11:00 UTC`
- 北京时间 `2026-04-28 19:00`

### 如果你在切换前已经用的是

```env
POLY_CLOB_HOST=https://clob-v2.polymarket.com
```

那么切换当天按下面做：

#### 1. 先停机器人

```bash
pm2 stop poly-bot-btc
```

如果你还有 API / dashboard，也可以一起停：

```bash
pm2 stop poly-bot-api
pm2 stop poly-dashboard
```

#### 2. 等官方停机窗口结束

官方文档说大约有 1 小时停机。

#### 3. 修改 `.env`

执行：

```bash
cd /root/poly-scan
nano .env
```

把：

```env
POLY_CLOB_HOST=https://clob-v2.polymarket.com
```

改成下面二选一。

方案 1：直接删掉这一行  
方案 2：显式改成：

```env
POLY_CLOB_HOST=https://clob.polymarket.com
```

#### 4. 保存并退出 `nano`

- `Ctrl + O`
- 回车
- `Ctrl + X`

#### 5. 重启机器人

```bash
cd /root/poly-scan
pm2 restart ecosystem.config.js --only poly-bot-btc --update-env
pm2 save
```

如果你还有 API / dashboard，再分别重启：

```bash
pm2 restart ecosystem.config.js --only poly-bot-api --update-env
pm2 restart ecosystem.config.js --only poly-dashboard --update-env
pm2 save
```

---

## 如果升级失败，怎么快速回滚

如果你升级后机器人跑不起来，不要慌，你前面已经做过备份。

### 最简单的回滚法

#### 1. 先停进程

```bash
pm2 stop poly-bot-btc
pm2 stop poly-bot-api
pm2 stop poly-dashboard
```

#### 2. 回到项目上级目录

```bash
cd /root
```

#### 3. 把当前坏掉的目录先改名

```bash
mv poly-scan poly-scan.failed-$(date +%F-%H%M%S)
```

#### 4. 把备份目录恢复回来

把下面命令里的时间戳替换成你自己的备份目录名：

```bash
cp -a poly-scan.backup-2026-04-22-120000 poly-scan
```

#### 5. 把 `.env` 备份恢复

同样把时间戳换成你自己的：

```bash
cp /root/poly-scan/.env.backup-2026-04-22-120000 /root/poly-scan/.env
```

#### 6. 重新启动原服务

```bash
cd /root/poly-scan
pm2 restart ecosystem.config.js --only poly-bot-btc --update-env
pm2 restart ecosystem.config.js --only poly-bot-api --update-env
pm2 restart ecosystem.config.js --only poly-dashboard --update-env
pm2 save
```

### 更轻一点的回滚法

如果只是某一个文件传错了，你也可以只把那个文件从备份目录里拷回去，不一定非要整目录回滚。

---

## 最常见的 10 个坑

### 1. 把 `.env` 整个覆盖了

不要上传本地 `.env` 去覆盖服务器 `.env`。

### 2. 把策略文件也传上去了

这次不要上传：

- `internal/btc/predictor.go`
- `internal/btc/risk_manager.go`
- `internal/btc/strategy.go`

### 3. `.env` 里写了两个 `POLY_CLOB_HOST`

同一个变量不要出现两次。

### 4. 改了 `.env`，但没更新 `ecosystem.config.js`

这样 PM2 进程可能读不到新变量。

### 5. 更新了文件，但没有 `pm2 restart ... --update-env`

这会导致服务还在用旧环境。

### 6. 只有 `USDC.e`，没有 `pUSD`

V2 要看 `pUSD`。

### 7. 直接在 Safe / Proxy 模式下乱跑 `wrap_pusd.py`

先确认实际出资地址，再决定在哪里准备 `pUSD`。

### 8. 还没做 `check_collateral_balance` 就直接重启机器人

建议先测通，再启动。

### 9. 服务器 Python 版本太低

`py-clob-client-v2` 要求 `>= 3.9.10`。

### 10. 切换当天忘了官方会清空挂单

这是官方行为，不是你的程序坏了。

---

## 给你的最稳执行顺序

如果你现在是第一次做服务器升级，我建议你严格按这个顺序做：

1. 登录服务器，确认真实项目路径
2. 备份整个项目和 `.env`
3. 查看 `.env`
4. 只手动加 `POLY_CLOB_HOST`，可选加 `POLY_RPC_URL`
5. 不动私钥、不动签名类型、不动 funder 地址
6. 从 Mac 只上传那 6 个最小升级文件
7. 不上传 `internal/btc/*`
8. 在服务器安装 `py-clob-client-v2`
9. 检查 `pUSD` 是否已经准备好
10. 执行 `check_collateral_balance`
11. 成功后再重启 `poly-bot-btc`
12. 看日志确认没有报错
13. 到 `2026-04-28 19:00` 北京时间再把 `POLY_CLOB_HOST` 从 `clob-v2` 切回正式入口

---

## 给你一个最简短的“必须记住版”

如果你只记住 6 件事，请记住这 6 件：

1. 不要覆盖服务器 `.env`
2. 不要上传你的策略文件
3. 一定要更新 `scripts/executor.py`
4. 用 PM2 的话，最好一起更新 `ecosystem.config.js`
5. V2 下单要看 `pUSD`，不是只看 `USDC.e`
6. 切换当天还要把 `POLY_CLOB_HOST` 从 `clob-v2` 处理回正式入口

---

## 官方参考

- [Polymarket Changelog](https://docs.polymarket.com/changelog)
- [Migrating to CLOB V2](https://docs.polymarket.com/v2-migration)
- [Polymarket USD (pUSD)](https://docs.polymarket.com/concepts/pusd)
- [Contracts](https://docs.polymarket.com/resources/contracts)
- [py-clob-client-v2](https://github.com/Polymarket/py-clob-client-v2)
