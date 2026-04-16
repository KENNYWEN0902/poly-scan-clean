#!/usr/bin/env python3
"""
Recover lost positions from Polymarket and save to positions.json
This will query your wallet for open positions and create the positions file.
"""
import os
import sys
import json
from datetime import datetime, timedelta

sys.path.insert(0, '/root/poly-scan')

from py_clob_client.client import ClobClient
from py_clob_client.clob_types import ApiCreds

def main():
    print("=== Position Recovery Tool ===\n")
    
    # Get credentials
    private_key = os.environ.get("POLY_PRIVATE_KEY")
    api_key = os.environ.get("POLY_API_KEY")
    api_secret = os.environ.get("POLY_API_SECRET")
    api_passphrase = os.environ.get("POLY_PASSPHRASE")
    signature_type = int(os.environ.get("POLY_SIGNATURE_TYPE", "2"))
    funder_address = os.environ.get("POLY_FUNDER_ADDRESS")
    
    if not all([private_key, api_key, api_secret, api_passphrase]):
        print("Error: Missing credentials", file=sys.stderr)
        return 1
    
    # Initialize client
    print("Connecting to Polymarket...", file=sys.stderr)
    client = ClobClient(
        host="https://clob.polymarket.com",
        key=private_key,
        chain_id=137,
        signature_type=signature_type,
        funder=funder_address if signature_type in [1, 2] else None
    )
    
    client.set_api_creds(ApiCreds(
        api_key=api_key,
        api_secret=api_secret,
        api_passphrase=api_passphrase
    ))
    
    print("Fetching open orders...", file=sys.stderr)
    
    # Get open orders (these are positions that were placed)
    try:
        orders = client.get_orders()
        print(f"Found {len(orders)} orders", file=sys.stderr)
        
        # Convert to positions format
        positions = {}
        now = datetime.utcnow()
        
        for order in orders:
            market_id = order.get('market', 'unknown')
            token_id = order.get('asset_id', '')
            side = 'UP' if 'up' in token_id.lower() else 'DOWN'
            price = float(order.get('price', 0))
            size = float(order.get('original_size', 0))
            
            # Estimate open time (use order creation time if available)
            open_time = now - timedelta(hours=1)  # Default: 1 hour ago
            
            position = {
                "MarketID": market_id,
                "TokenID": token_id,
                "Side": side,
                "EntryPrice": price,
                "Size": size,
                "OpenTime": open_time.isoformat() + "Z",
                "IsActive": True,
                "CloseReason": ""
            }
            
            positions[market_id] = position
        
        # Save to file
        os.makedirs('data', exist_ok=True)
        with open('data/positions.json', 'w') as f:
            json.dump(positions, f, indent=2)
        
        print(f"\n✅ Recovered {len(positions)} positions")
        print(f"Saved to: data/positions.json")
        print("\nRestart the bot to load these positions:")
        print("  pm2 restart poly-bot-btc")
        
        return 0
        
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        import traceback
        traceback.print_exc(file=sys.stderr)
        return 1

if __name__ == "__main__":
    sys.exit(main())
