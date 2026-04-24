#!/usr/bin/env python3
"""
Wrap or unwrap Polymarket collateral.

Usage:
  python3 scripts/wrap_pusd.py wrap 100
  python3 scripts/wrap_pusd.py unwrap 25
  python3 scripts/wrap_pusd.py balances
"""

import os
import sys
from decimal import Decimal, ROUND_DOWN

from eth_account import Account
from web3 import Web3

PUSD_ADDRESS = "0xC011a7E12a19f7B1f670d46F03B03f3342E82DFB"
USDCE_ADDRESS = "0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174"
ONRAMP_ADDRESS = "0x93070a847efEf7F70739046A929D47a521F5B8ee"
OFFRAMP_ADDRESS = "0x2957922Eb93258b93368531d39fAcCA3B4dC5854"
DECIMALS = 6

ERC20_ABI = [
    {
        "inputs": [{"name": "account", "type": "address"}],
        "name": "balanceOf",
        "outputs": [{"name": "", "type": "uint256"}],
        "stateMutability": "view",
        "type": "function",
    },
    {
        "inputs": [
            {"name": "spender", "type": "address"},
            {"name": "amount", "type": "uint256"},
        ],
        "name": "approve",
        "outputs": [{"name": "", "type": "bool"}],
        "stateMutability": "nonpayable",
        "type": "function",
    },
]

ONRAMP_ABI = [
    {
        "inputs": [
            {"name": "_asset", "type": "address"},
            {"name": "_to", "type": "address"},
            {"name": "_amount", "type": "uint256"},
        ],
        "name": "wrap",
        "outputs": [],
        "stateMutability": "nonpayable",
        "type": "function",
    }
]

OFFRAMP_ABI = [
    {
        "inputs": [
            {"name": "_asset", "type": "address"},
            {"name": "_to", "type": "address"},
            {"name": "_amount", "type": "uint256"},
        ],
        "name": "unwrap",
        "outputs": [],
        "stateMutability": "nonpayable",
        "type": "function",
    }
]


def parse_amount(value: str) -> int:
    quantized = Decimal(value).quantize(Decimal("0.000001"), rounding=ROUND_DOWN)
    return int(quantized * (10 ** DECIMALS))


def format_amount(value: int) -> str:
    return f"{Decimal(value) / Decimal(10 ** DECIMALS):.6f}"


def get_web3() -> Web3:
    rpc_url = os.environ.get("POLY_RPC_URL") or os.environ.get("POLYGON_RPC_URL") or "https://polygon-bor-rpc.publicnode.com"
    w3 = Web3(Web3.HTTPProvider(rpc_url, request_kwargs={"timeout": 20}))
    if not w3.is_connected():
        raise RuntimeError(f"Failed to connect to Polygon RPC: {rpc_url}")
    return w3


def build_tx(account, w3: Web3) -> dict:
    return {
        "from": account.address,
        "nonce": w3.eth.get_transaction_count(account.address, "pending"),
        "chainId": 137,
    }


def send_and_wait(w3: Web3, account, tx: dict, label: str) -> str:
    if "gasPrice" not in tx:
        tx["gasPrice"] = w3.eth.gas_price
    if "gas" not in tx:
        tx["gas"] = 250000
    signed = account.sign_transaction(tx)
    tx_hash = w3.eth.send_raw_transaction(signed.raw_transaction)
    print(f"[{label}] sent: {tx_hash.hex()}")
    receipt = w3.eth.wait_for_transaction_receipt(tx_hash, timeout=180)
    if receipt["status"] != 1:
        raise RuntimeError(f"{label} reverted on-chain: {tx_hash.hex()}")
    print(f"[{label}] confirmed in block {receipt['blockNumber']}")
    return tx_hash.hex()


def print_balances(w3: Web3, address: str) -> None:
    usdce = w3.eth.contract(address=Web3.to_checksum_address(USDCE_ADDRESS), abi=ERC20_ABI)
    pusd = w3.eth.contract(address=Web3.to_checksum_address(PUSD_ADDRESS), abi=ERC20_ABI)
    usdce_balance = usdce.functions.balanceOf(address).call()
    pusd_balance = pusd.functions.balanceOf(address).call()
    print(f"Wallet: {address}")
    print(f"USDC.e: {format_amount(usdce_balance)}")
    print(f"pUSD:   {format_amount(pusd_balance)}")


def wrap(amount: str) -> None:
    private_key = os.environ.get("POLY_PRIVATE_KEY")
    if not private_key:
        raise RuntimeError("POLY_PRIVATE_KEY is required")

    w3 = get_web3()
    account = Account.from_key(private_key)
    amount_base = parse_amount(amount)

    usdce = w3.eth.contract(address=Web3.to_checksum_address(USDCE_ADDRESS), abi=ERC20_ABI)
    onramp = w3.eth.contract(address=Web3.to_checksum_address(ONRAMP_ADDRESS), abi=ONRAMP_ABI)

    print_balances(w3, account.address)
    send_and_wait(
        w3,
        account,
        usdce.functions.approve(Web3.to_checksum_address(ONRAMP_ADDRESS), amount_base).build_transaction(build_tx(account, w3)),
        "approve-onramp",
    )
    send_and_wait(
        w3,
        account,
        onramp.functions.wrap(
            Web3.to_checksum_address(USDCE_ADDRESS),
            account.address,
            amount_base,
        ).build_transaction(build_tx(account, w3)),
        "wrap-pusd",
    )
    print_balances(w3, account.address)


def unwrap(amount: str) -> None:
    private_key = os.environ.get("POLY_PRIVATE_KEY")
    if not private_key:
        raise RuntimeError("POLY_PRIVATE_KEY is required")

    w3 = get_web3()
    account = Account.from_key(private_key)
    amount_base = parse_amount(amount)

    pusd = w3.eth.contract(address=Web3.to_checksum_address(PUSD_ADDRESS), abi=ERC20_ABI)
    offramp = w3.eth.contract(address=Web3.to_checksum_address(OFFRAMP_ADDRESS), abi=OFFRAMP_ABI)

    print_balances(w3, account.address)
    send_and_wait(
        w3,
        account,
        pusd.functions.approve(Web3.to_checksum_address(OFFRAMP_ADDRESS), amount_base).build_transaction(build_tx(account, w3)),
        "approve-offramp",
    )
    send_and_wait(
        w3,
        account,
        offramp.functions.unwrap(
            Web3.to_checksum_address(USDCE_ADDRESS),
            account.address,
            amount_base,
        ).build_transaction(build_tx(account, w3)),
        "unwrap-pusd",
    )
    print_balances(w3, account.address)


def main() -> int:
    if len(sys.argv) < 2:
        print(__doc__.strip())
        return 1

    command = sys.argv[1].lower()
    try:
        if command == "balances":
            private_key = os.environ.get("POLY_PRIVATE_KEY")
            if not private_key:
                raise RuntimeError("POLY_PRIVATE_KEY is required")
            account = Account.from_key(private_key)
            print_balances(get_web3(), account.address)
            return 0

        if len(sys.argv) < 3:
            raise RuntimeError("Amount is required for wrap/unwrap")

        if command == "wrap":
            wrap(sys.argv[2])
            return 0
        if command == "unwrap":
            unwrap(sys.argv[2])
            return 0
        raise RuntimeError(f"Unknown command: {command}")
    except Exception as exc:
        print(f"Error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
