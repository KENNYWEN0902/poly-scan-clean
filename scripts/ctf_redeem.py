#!/usr/bin/env python3
"""
CTF Contract Redeem - Properly redeem winning positions via smart contract
Based on: https://github.com/Polymarket/conditional-token-examples-py
"""
import os
import sys
from web3 import Web3
from eth_account import Account

# CTF Contract on Polygon
CTF_ADDRESS = "0x4D97DCd97eC945f40cF65F87097ACe5EA0476045"
USDC_ADDRESS = "0x2791Bca1f2de4661ED88A30C99A7a9449Aa84174"  # USDC.e on Polygon
HASH_ZERO = "0x0000000000000000000000000000000000000000000000000000000000000000"

# CTF ABI (simplified - only redeemPositions)
CTF_ABI = [
    {
        "inputs": [
            {"name": "collateralToken", "type": "address"},
            {"name": "parentCollectionId", "type": "bytes32"},
            {"name": "conditionId", "type": "bytes32"},
            {"name": "indexSets", "type": "uint256[]"}
        ],
        "name": "redeemPositions",
        "outputs": [],
        "stateMutability": "nonpayable",
        "type": "function"
    }
]

def redeem_position(condition_id: str, private_key: str, rpc_url: str = "https://polygon-rpc.com"):
    """
    Redeem a winning position from CTF contract
    
    Args:
        condition_id: The market's condition ID
        private_key: Your Ethereum private key
        rpc_url: Polygon RPC endpoint
    """
    # Connect to Polygon
    w3 = Web3(Web3.HTTPProvider(rpc_url))
    if not w3.is_connected():
        raise Exception("Failed to connect to Polygon")
    
    # Load account
    account = Account.from_key(private_key)
    
    # Load CTF contract
    ctf = w3.eth.contract(address=CTF_ADDRESS, abi=CTF_ABI)
    
    # Build transaction
    txn = ctf.functions.redeemPositions(
        USDC_ADDRESS,
        HASH_ZERO,
        condition_id,
        [1, 2]  # Redeem both outcomes (only winning pays)
    ).build_transaction({
        'from': account.address,
        'nonce': w3.eth.get_transaction_count(account.address),
        'gas': 200000,
        'gasPrice': w3.eth.gas_price
    })
    
    # Sign and send
    signed_txn = account.sign_transaction(txn)
    tx_hash = w3.eth.send_raw_transaction(signed_txn.rawTransaction)
    
    print(f"Transaction sent: {tx_hash.hex()}")
    
    # Wait for receipt
    receipt = w3.eth.wait_for_transaction_receipt(tx_hash)
    
    if receipt['status'] == 1:
        print(f"✅ Redeem successful!")
        return {"success": True, "tx_hash": tx_hash.hex()}
    else:
        print(f"❌ Redeem failed")
        return {"success": False, "tx_hash": tx_hash.hex()}

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python3 ctf_redeem.py <condition_id>")
        sys.exit(1)
    
    condition_id = sys.argv[1]
    private_key = os.environ.get("POLY_PRIVATE_KEY")
    
    if not private_key:
        print("Error: POLY_PRIVATE_KEY not set")
        sys.exit(1)
    
    result = redeem_position(condition_id, private_key)
    print(result)
