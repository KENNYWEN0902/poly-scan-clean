#!/bin/bash

# ===================================================================
# Polymarket Trading Bot Startup Script
# ===================================================================
# SECURITY: Load credentials from .env file or environment variables.
# NEVER hardcode private keys in this script!
# ===================================================================

# Clear stale API credentials from previous sessions — always derive fresh ones
unset POLY_API_KEY POLY_API_SECRET POLY_PASSPHRASE

# Load .env file if it exists (create one from .env.example)
if [ -f /root/poly-scan/.env ]; then
    set -a
    source /root/poly-scan/.env
    set +a
fi

# Validate required credentials
if [ -z "$POLY_PRIVATE_KEY" ]; then
    echo "ERROR: POLY_PRIVATE_KEY is not set. Create a .env file or export it."
    echo "  Example: echo 'POLY_PRIVATE_KEY=0x...' > /root/poly-scan/.env"
    exit 1
fi

# Gnosis Safe configuration (override in .env if needed)
export POLY_SIGNATURE_TYPE="${POLY_SIGNATURE_TYPE:-2}"
export POLY_FUNDER_ADDRESS="${POLY_FUNDER_ADDRESS:-}"

# Trading parameters
export POLY_TAKE_PROFIT_PCT="${POLY_TAKE_PROFIT_PCT:-0.50}"  # 50% 止盈
export POLY_STOP_LOSS_PCT="${POLY_STOP_LOSS_PCT:-0.20}"      # 20% 止损

cd /root/poly-scan
./poly-bot
