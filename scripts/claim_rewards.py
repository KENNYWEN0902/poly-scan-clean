"""
领取 Polymarket 已结算仓位的收益。
通过 Gnosis Safe 代理钱包调用 CTF 合约的 redeemPositions，赎回到 pUSD。
"""
import json
import sys
import requests
from eth_account import Account
from eth_abi import encode
from web3 import Web3

import os

PRIVATE_KEY = os.environ.get("POLY_PRIVATE_KEY", "")
PROXY_WALLET = os.environ.get("POLY_FUNDER_ADDRESS", "")
EOA_ADDRESS = Account.from_key(PRIVATE_KEY).address if PRIVATE_KEY else ""

# Polygon 合约地址
CTF_ADDRESS = "0x4D97DCd97eC945f40cF65F87097ACe5EA0476045"
PUSD_ADDRESS = "0xC011a7E12a19f7B1f670d46F03B03f3342E82DFB"

# RPC (多备选)
RPC_URLS = [
    "https://polygon-mainnet.g.alchemy.com/v2/demo",
    "https://rpc.ankr.com/polygon",
    "https://polygon-bor-rpc.publicnode.com",
    "https://polygon-rpc.com",
]

# redeemPositions(address collateralToken, bytes32 parentCollectionId, bytes32 conditionId, uint[] indexSets)
REDEEM_SELECTOR = Web3.keccak(text="redeemPositions(address,bytes32,bytes32,uint256[])")[:4]

# Gnosis Safe execTransaction 签名
EXEC_TX_ABI = json.loads('[{"inputs":[{"name":"to","type":"address"},{"name":"value","type":"uint256"},{"name":"data","type":"bytes"},{"name":"operation","type":"uint8"},{"name":"safeTxGas","type":"uint256"},{"name":"baseGas","type":"uint256"},{"name":"gasPrice","type":"uint256"},{"name":"gasToken","type":"address"},{"name":"refundReceiver","type":"address"},{"name":"signatures","type":"bytes"}],"name":"execTransaction","outputs":[{"name":"success","type":"bool"}],"stateMutability":"nonpayable","type":"function"}]')

def get_redeemable_positions():
    """查询可领取的仓位"""
    url = f"https://data-api.polymarket.com/positions?user={PROXY_WALLET}"
    resp = requests.get(url, timeout=10)
    positions = resp.json()
    redeemable = [p for p in positions if p.get("redeemable")]
    return redeemable

def build_redeem_calldata(condition_id_hex):
    """构造 CTF redeemPositions 的 calldata"""
    condition_id = bytes.fromhex(condition_id_hex[2:])
    parent_collection_id = b'\x00' * 32
    index_sets = [1, 2]  # 标准二元市场

    encoded_params = encode(
        ['address', 'bytes32', 'bytes32', 'uint256[]'],
        [
            Web3.to_checksum_address(PUSD_ADDRESS),
            parent_collection_id,
            condition_id, 
            index_sets
        ]
    )
    return REDEEM_SELECTOR + encoded_params

def build_safe_tx(w3, to, data):
    """构造 Gnosis Safe 的 execTransaction"""
    safe = w3.eth.contract(address=Web3.to_checksum_address(PROXY_WALLET), abi=EXEC_TX_ABI)
    account = Account.from_key(PRIVATE_KEY)

    # 构建 Safe 交易哈希 (EIP-712)
    # 对于单签 Safe (threshold=1)，签名格式为 r + s + v
    # 使用 pre-validated 签名 (v=1, r=owner_address, s=0)
    owner_padded = account.address.lower()[2:].zfill(64)
    sig = bytes.fromhex(owner_padded + "0" * 64 + "01")

    nonce = w3.eth.call({
        'to': Web3.to_checksum_address(PROXY_WALLET),
        'data': Web3.keccak(text="nonce()")[:4]
    })
    safe_nonce = int.from_bytes(nonce, 'big')
    print(f"  Safe nonce: {safe_nonce}")

    tx = safe.functions.execTransaction(
        Web3.to_checksum_address(to),  # to
        0,                              # value
        data,                           # data  
        0,                              # operation (CALL)
        0,                              # safeTxGas
        0,                              # baseGas
        0,                              # gasPrice
        "0x0000000000000000000000000000000000000000",  # gasToken
        "0x0000000000000000000000000000000000000000",  # refundReceiver
        sig                             # signatures
    ).build_transaction({
        'from': account.address,
        'nonce': w3.eth.get_transaction_count(account.address),
        'gas': 300000,
        'gasPrice': w3.eth.gas_price,
        'chainId': 137
    })

    signed = account.sign_transaction(tx)
    return signed

def main():
    print("=== Polymarket 收益领取工具 ===\n")

    # 查询可领取仓位
    positions = get_redeemable_positions()
    if not positions:
        print("没有可领取的仓位")
        return

    print(f"找到 {len(positions)} 个可领取仓位:")
    for p in positions:
        print(f"  市场: {p['title']}")
        print(f"  结果: {p['outcome']} | 数量: {p['size']} | 当前价值: ${p['currentValue']}")
        print(f"  condition_id: {p['conditionId']}")
        print()

    w3 = None
    for rpc in RPC_URLS:
        try:
            _w3 = Web3(Web3.HTTPProvider(rpc, request_kwargs={'timeout': 5}))
            if _w3.is_connected():
                w3 = _w3
                print(f"已连接 RPC: {rpc}")
                break
        except Exception:
            continue
    if not w3:
        print("无法连接任何 Polygon RPC")
        return

    account = Account.from_key(PRIVATE_KEY)
    balance = w3.eth.get_balance(account.address)
    print(f"EOA ({account.address}) MATIC 余额: {w3.from_wei(balance, 'ether')} MATIC")

    if balance < w3.to_wei(0.005, 'ether'):
        print("警告: MATIC 余额可能不足以支付 gas 费")

    for p in positions:
        condition_id = p['conditionId']
        title = p['title']
        print(f"\n正在领取: {title}")
        print(f"  condition_id: {condition_id}")

        redeem_data = build_redeem_calldata(condition_id)
        print(f"  已构造 redeemPositions calldata ({len(redeem_data)} bytes)")

        try:
            signed_tx = build_safe_tx(w3, CTF_ADDRESS, redeem_data)
            tx_hash = w3.eth.send_raw_transaction(signed_tx.raw_transaction)
            print(f"  已发送交易: {tx_hash.hex()}")
            print(f"  等待确认...")
            receipt = w3.eth.wait_for_transaction_receipt(tx_hash, timeout=60)
            if receipt['status'] == 1:
                print(f"  领取成功! Gas: {receipt['gasUsed']}")
            else:
                print(f"  交易失败! Receipt: {receipt}")
        except Exception as e:
            print(f"  领取失败: {e}")

    print("\n=== 领取流程结束 ===")

if __name__ == '__main__':
    main()
