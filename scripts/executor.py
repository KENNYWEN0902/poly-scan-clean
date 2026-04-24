#!/usr/bin/env python3
"""
Polymarket Order Executor
Executes orders via py-clob-client-v2 with batch support and error handling.
"""

import sys
import json
import os
import time
import random
from typing import List, Dict, Any, Optional
from dataclasses import dataclass
from enum import Enum

from py_clob_client_v2 import (
    AssetType,
    ApiCreds,
    BalanceAllowanceParams,
    BuilderConfig,
    ClobClient,
    OrderArgs,
    OrderType,
)

# Rate limiting configuration
MIN_REQUEST_INTERVAL = 0.5  # Minimum seconds between API requests
MAX_RETRIES = 3  # Maximum retry attempts for 429 errors
BASE_RETRY_DELAY = 2.0  # Base delay for exponential backoff

MIN_PYTHON = (3, 9, 10)
DEFAULT_CLOB_HOST = "https://clob.polymarket.com"
PUSD_ADDRESS = "0xC011a7E12a19f7B1f670d46F03B03f3342E82DFB"
CTF_ADDRESS = "0x4D97DCd97eC945f40cF65F87097ACe5EA0476045"


class OrderStatus(Enum):
    PENDING = "pending"
    FILLED = "filled"
    PARTIAL = "partial"
    REJECTED = "rejected"
    CANCELLED = "cancelled"


@dataclass
class OrderResult:
    order_id: str
    token_id: str
    side: str
    price: float
    size: float
    filled_size: float
    avg_price: float
    status: OrderStatus
    error: Optional[str] = None


@dataclass
class BatchResult:
    batch_id: str
    orders: List[OrderResult]
    total_cost: float
    total_filled: float
    success: bool
    error_message: Optional[str] = None


class PolymarketExecutor:
    def __init__(self, host: Optional[str] = None, chain_id: int = 137):
        self.host = (host or os.environ.get("POLY_CLOB_HOST") or DEFAULT_CLOB_HOST).rstrip("/")
        self.chain_id = chain_id
        self.client: Optional[ClobClient] = None
        self.connect_error = ""

    def connect(self) -> bool:
        if sys.version_info < MIN_PYTHON:
            version = ".".join(str(part) for part in MIN_PYTHON)
            self.connect_error = (
                f"py-clob-client-v2 requires Python {version}+ "
                f"(current: {sys.version_info.major}.{sys.version_info.minor}.{sys.version_info.micro})"
            )
            print(f"Error: {self.connect_error}", file=sys.stderr)
            return False

        private_key = os.environ.get("POLY_PRIVATE_KEY")
        try:
            signature_type = int(os.environ.get("POLY_SIGNATURE_TYPE", "0"))
        except ValueError:
            self.connect_error = "POLY_SIGNATURE_TYPE must be an int"
            print("Error: POLY_SIGNATURE_TYPE must be an int", file=sys.stderr)
            return False

        funder_address = os.environ.get("POLY_FUNDER_ADDRESS", "")
        builder_code = os.environ.get("POLY_BUILDER_CODE", "").strip()

        if not private_key:
            self.connect_error = "POLY_PRIVATE_KEY is required"
            print("Error: POLY_PRIVATE_KEY is required.", file=sys.stderr)
            return False

        client_kwargs = {
            "host": self.host,
            "key": private_key,
            "chain_id": self.chain_id,
            "signature_type": signature_type,
        }

        if signature_type in [1, 2] and funder_address:
            client_kwargs["funder"] = funder_address
        if builder_code:
            client_kwargs["builder_config"] = BuilderConfig(builder_code=builder_code)
        self.client = ClobClient(**client_kwargs)

        # Use pre-configured API creds from env if available (faster, no on-chain call)
        api_key = os.environ.get("POLY_API_KEY", "")
        api_secret = os.environ.get("POLY_API_SECRET", "")
        passphrase = os.environ.get("POLY_PASSPHRASE", "")

        if api_key and api_secret and passphrase:
            self.client.set_api_creds(ApiCreds(
                api_key=api_key,
                api_secret=api_secret,
                api_passphrase=passphrase,
            ))
            print(
                f"[DEBUG] Using pre-configured API creds from env against {self.host}.",
                file=sys.stderr,
            )
            return True

        # V2 keeps the same L1/L2 auth flow; the new SDK exposes a combined helper.
        try:
            creds = self.client.create_or_derive_api_key()
            if creds and getattr(creds, "api_key", None):
                self.client.set_api_creds(creds)
                print(f"[DEBUG] Created/derived API creds successfully for {self.host}.", file=sys.stderr)
                return True
        except Exception as e:
            self.connect_error = f"Failed to create or derive API key: {e}"
            print(f"Error: {self.connect_error}", file=sys.stderr)
            return False

        self.connect_error = "create_or_derive_api_key returned empty creds"
        print(f"Error: {self.connect_error}", file=sys.stderr)
        return False

    def _api_call_with_retry(self, func, *args, **kwargs):
        last_error = None

        for attempt in range(MAX_RETRIES):
            try:
                time.sleep(MIN_REQUEST_INTERVAL + random.uniform(0, 0.2))
                result = func(*args, **kwargs)
                return result
            except Exception as e:
                error_str = str(e)
                if (
                    "429" in error_str
                    or "rate limit" in error_str.lower()
                    or "cloudflare" in error_str.lower()
                ):
                    last_error = e
                    delay = BASE_RETRY_DELAY * (2**attempt) + random.uniform(0, 1)
                    print(
                        f"[RETRY] Rate limited, waiting {delay:.1f}s before retry {attempt + 1}/{MAX_RETRIES}",
                        file=sys.stderr,
                    )
                    time.sleep(delay)
                else:
                    raise

        raise last_error

    def execute_order(self, order: Dict[str, Any]) -> OrderResult:
        token_id = order["tokenID"]
        side = order["side"]
        try:
            price = float(order["price"])
            size = float(order["size"])
        except (ValueError, TypeError) as e:
            return OrderResult(
                order_id="",
                token_id=token_id,
                side=side,
                price=0,
                size=0,
                filled_size=0,
                avg_price=0,
                status=OrderStatus.REJECTED,
                error=f"Invalid price or size: {e}",
            )
        order_type = order.get("type", "FOK")

        # Polymarket requires minimum 5 shares per order
        MIN_ORDER_SIZE = 5.0
        if size < MIN_ORDER_SIZE:
            return OrderResult(
                order_id="",
                token_id=token_id,
                side=side,
                price=price,
                size=size,
                filled_size=0,
                avg_price=0,
                status=OrderStatus.REJECTED,
                error=f"Order size {size} below minimum {MIN_ORDER_SIZE} shares",
            )

        print(f"Executing: {side} {size} of {token_id} @ ${price}", file=sys.stderr)

        try:
            order_args = OrderArgs(price=price, size=size, side=side, token_id=token_id)

            signed_order = self._api_call_with_retry(
                self.client.create_order, order_args
            )

            order_type_enum = OrderType.FOK
            if order_type == "GTC":
                order_type_enum = OrderType.GTC
            elif order_type == "GTD":
                order_type_enum = OrderType.GTD
                if "expiration" not in order:
                    print(
                        f"Warning: GTD order without expiration, defaulting to GTC",
                        file=sys.stderr,
                    )
                    order_type_enum = OrderType.GTC

            resp = self._api_call_with_retry(
                self.client.post_order, signed_order, order_type_enum
            )
            resp_log = {
                k: v for k, v in resp.items() if k not in ("signature", "salt", "maker")
            }
            print(f"Order Response: {resp_log}", file=sys.stderr)

            if "error" in resp:
                return OrderResult(
                    order_id="",
                    token_id=token_id,
                    side=side,
                    price=price,
                    size=size,
                    filled_size=0,
                    avg_price=0,
                    status=OrderStatus.REJECTED,
                    error=resp["error"],
                )

            # For GTC orders, the order is placed on the book
            # Response will have status='live' or 'matched'
            order_id = resp.get("orderID", "")
            order_status = resp.get("status", "")
            print(f"[DEBUG] post_order response: status={order_status}, orderID={order_id}, "
                  f"takingAmount={resp.get('takingAmount','')}, makingAmount={resp.get('makingAmount','')}, "
                  f"keys={list(resp.keys())}", file=sys.stderr)

            # Check if order was accepted
            if order_status == "live":
                # GTC order placed successfully on the book
                return OrderResult(
                    order_id=order_id,
                    token_id=token_id,
                    side=side,
                    price=price,
                    size=size,
                    filled_size=0,  # Not filled yet, just placed
                    avg_price=price,
                    status=OrderStatus.PENDING,  # Order is live on the book
                )

            if order_status == "matched":
                # Order was matched/filled immediately
                # Parse actual filled amount from response
                # SELL: makingAmount = tokens sold (= fill size)
                # BUY: takingAmount = tokens received (= fill size)
                taking_amount = resp.get("takingAmount", "")
                filled_size = size  # Default: assume full fill
                if side == "SELL":
                    fill_source = resp.get("makingAmount", "")
                else:
                    fill_source = taking_amount
                if fill_source:
                    try:
                        raw_val = float(fill_source)
                        # Values in base units (6 decimals) if >= 1000
                        # Some responses may already be in human-readable format
                        if raw_val >= 1000:
                            filled_size = raw_val / 1e6
                        else:
                            filled_size = raw_val
                        print(f"[DEBUG] Parsed fill amount ({side}): raw={raw_val}, filled={filled_size}", file=sys.stderr)
                    except (ValueError, TypeError):
                        filled_size = size

                # Parse actual average price from makingAmount/takingAmount
                making_amount = resp.get("makingAmount", "")
                actual_avg_price = price
                if making_amount and taking_amount:
                    try:
                        raw_making = float(making_amount)
                        raw_taking = float(taking_amount)
                        if side == "SELL" and raw_making > 0:
                            # SELL: makingAmount=tokens, takingAmount=collateral → price = collateral/tokens
                            actual_avg_price = raw_taking / raw_making
                        elif raw_taking > 0:
                            # BUY: makingAmount=collateral, takingAmount=tokens → price = collateral/tokens
                            actual_avg_price = raw_making / raw_taking
                        print(f"[DEBUG] Parsed avg price ({side}): making={raw_making}, taking={raw_taking}, price={actual_avg_price}", file=sys.stderr)
                    except (ValueError, TypeError, ZeroDivisionError):
                        actual_avg_price = price

                return OrderResult(
                    order_id=order_id,
                    token_id=token_id,
                    side=side,
                    price=price,
                    size=size,
                    filled_size=filled_size,
                    avg_price=actual_avg_price,
                    status=OrderStatus.FILLED,
                )

            # Fallback parsing
            filled_size = (
                size
                if order_type_enum == OrderType.FOK
                else float(resp.get("amount", "0")) / 1e6
            )
            avg_price = float(resp.get("price", str(price)))

            status = OrderStatus.FILLED
            if filled_size < size:
                status = (
                    OrderStatus.PARTIAL if filled_size > 0 else OrderStatus.REJECTED
                )

            return OrderResult(
                order_id=resp.get("id", ""),
                token_id=token_id,
                side=side,
                price=price,
                size=size,
                filled_size=filled_size,
                avg_price=avg_price,
                status=status,
            )

        except Exception as e:
            print(f"Exception during execution: {e}", file=sys.stderr)
            return OrderResult(
                order_id="",
                token_id=token_id,
                side=side,
                price=price,
                size=size,
                filled_size=0,
                avg_price=0,
                status=OrderStatus.REJECTED,
                error=str(e),
            )

    def execute_batch(
        self, orders: List[Dict[str, Any]], batch_id: str = ""
    ) -> BatchResult:
        results: List[OrderResult] = []
        total_cost = 0.0
        total_filled = 0.0
        all_success = True
        error_messages = []

        # Refresh balance allowance cache (best-effort, not fatal).
        # This syncs on-chain pUSD allowance to the CLOB cache.
        try:
            self.client.update_balance_allowance(
                BalanceAllowanceParams(asset_type=AssetType.COLLATERAL)
            )
            print("[DEBUG] Balance allowance cache refreshed", file=sys.stderr)
        except Exception as e:
            # Log warning but continue — on-chain allowance may already be set
            # Orders will fail at CLOB with a clear error if allowance is truly missing
            print(f"[WARN] Allowance refresh failed (continuing): {e}", file=sys.stderr)

        for i, order in enumerate(orders):
            result = self.execute_order(order)
            results.append(result)

            if result.status == OrderStatus.FILLED:
                total_cost += result.filled_size * result.avg_price
                total_filled += result.filled_size
            elif result.status == OrderStatus.PENDING:
                # GTC order placed on book - don't count as filled yet
                # Only track as success, actual fill happens asynchronously
                pass
            elif result.status == OrderStatus.PARTIAL:
                total_cost += result.filled_size * result.avg_price
                total_filled += result.filled_size
                all_success = False
                error_messages.append(f"Partial fill on order {i}")
            else:
                all_success = False
                if result.error:
                    error_messages.append(result.error)

        return BatchResult(
            batch_id=batch_id,
            orders=results,
            total_cost=total_cost,
            total_filled=total_filled,
            success=all_success,
            error_message="; ".join(error_messages) if error_messages else None,
        )

    def execute_batch_atomic(
        self, orders: List[Dict[str, Any]], batch_id: str = ""
    ) -> BatchResult:
        results: List[OrderResult] = []
        total_cost = 0.0
        total_filled = 0.0

        signed_orders = []
        for order in orders:
            try:
                order_args = OrderArgs(
                    price=float(order["price"]),
                    size=float(order["size"]),
                    side=order["side"],
                    token_id=order["tokenID"],
                )
                signed_order = self.client.create_order(order_args)
                signed_orders.append((order, signed_order))
            except Exception as e:
                print(f"Failed to sign order: {e}", file=sys.stderr)
                return BatchResult(
                    batch_id=batch_id,
                    orders=[],
                    total_cost=0,
                    total_filled=0,
                    success=False,
                    error_message=f"Failed to sign order: {e}",
                )

        for order, signed_order in signed_orders:
            try:
                resp = self.client.post_order(signed_order, OrderType.FOK)
                print(f"Order Response: {resp}", file=sys.stderr)

                if "error" in resp:
                    return BatchResult(
                        batch_id=batch_id,
                        orders=results,
                        total_cost=total_cost,
                        total_filled=total_filled,
                        success=False,
                        error_message=f"Order rejected: {resp['error']}",
                    )

                order_status = resp.get("status", "")
                order_id = resp.get("orderID", resp.get("id", ""))

                if order_status == "matched":
                    if order["side"] == "SELL":
                        # SELL: makingAmount = tokens sold (= fill size)
                        raw_val = float(resp.get("makingAmount", "0"))
                    else:
                        # BUY: takingAmount = tokens received (= fill size)
                        raw_val = float(resp.get("takingAmount", "0"))
                    filled_size = raw_val / 1e6 if raw_val >= 1000 else raw_val
                elif order_status == "live":
                    filled_size = 0
                else:
                    filled_size = float(resp.get("amount", "0")) / 1e6

                # Compute avg_price from making/taking amounts when available
                making_amount = resp.get("makingAmount", "")
                taking_amount = resp.get("takingAmount", "")
                if making_amount and taking_amount:
                    try:
                        raw_making = float(making_amount)
                        raw_taking = float(taking_amount)
                        if order["side"] == "SELL" and raw_making > 0:
                            # SELL: makingAmount=tokens, takingAmount=collateral → price = collateral/tokens
                            avg_price = raw_taking / raw_making
                        elif raw_taking > 0:
                            # BUY: makingAmount=collateral, takingAmount=tokens → price = collateral/tokens
                            avg_price = raw_making / raw_taking
                        else:
                            avg_price = float(resp.get("price", str(order["price"])))
                    except (ValueError, TypeError, ZeroDivisionError):
                        avg_price = float(resp.get("price", str(order["price"])))
                else:
                    avg_price = float(resp.get("price", str(order["price"])))

                status = OrderStatus.FILLED
                if order_status == "live":
                    status = OrderStatus.PENDING
                elif filled_size < float(order["size"]):
                    status = (
                        OrderStatus.PARTIAL if filled_size > 0 else OrderStatus.REJECTED
                    )

                result = OrderResult(
                    order_id=order_id,
                    token_id=order["tokenID"],
                    side=order["side"],
                    price=float(order["price"]),
                    size=float(order["size"]),
                    filled_size=filled_size,
                    avg_price=avg_price,
                    status=status,
                )

                results.append(result)
                total_cost += filled_size * avg_price
                total_filled += filled_size

            except Exception as e:
                print(f"Exception during execution: {e}", file=sys.stderr)
                return BatchResult(
                    batch_id=batch_id,
                    orders=results,
                    total_cost=total_cost,
                    total_filled=total_filled,
                    success=False,
                    error_message=str(e),
                )

        return BatchResult(
            batch_id=batch_id,
            orders=results,
            total_cost=total_cost,
            total_filled=total_filled,
            success=True,
        )

    def claim_position(self, condition_id: str) -> Dict[str, Any]:
        """通过 Polymarket Relayer 免 gas 领取已结算仓位（Safe 钱包），或回退到链上交易。"""
        sig_type = int(os.environ.get("POLY_SIGNATURE_TYPE", "0"))
        funder = os.environ.get("POLY_FUNDER_ADDRESS", "")

        # Safe (sig_type=2) or Proxy (sig_type=1): use gasless relayer
        if sig_type in (1, 2) and funder:
            result = self._claim_position_gasless(condition_id, sig_type)
            if result.get("success"):
                return result
            # If gasless fails due to rate limit or relayer issue, fall back to on-chain
            err = result.get("error", "")
            if "rate" not in err.lower() and "quota" not in err.lower():
                return result
            print(f"[CLAIM] Gasless rate limited, falling back to on-chain", file=sys.stderr)

        return self._claim_position_onchain(condition_id)

    def _claim_position_gasless(self, condition_id: str, sig_type: int) -> Dict[str, Any]:
        """通过 Polymarket Relayer 免 gas 领取（无需 MATIC）。
        使用 polymarket-apis 构建 Safe 交易体，通过 Relayer API Key 提交。
        BTC 5-min 二元市场为 non-neg_risk，使用 CTF 的 redeemPositions。
        """
        try:
            import requests as http_requests
            from polymarket_apis import PolymarketGaslessWeb3Client
            from json import dumps

            print(f"[CLAIM] Gasless redeem via relayer: {condition_id}", file=sys.stderr)

            private_key = os.environ.get("POLY_PRIVATE_KEY", "")
            relayer_api_key = os.environ.get("POLY_RELAYER_API_KEY", "")

            gasless = PolymarketGaslessWeb3Client(
                private_key=private_key,
                signature_type=sig_type,
            )

            # 检查链上条件是否已 resolve（payoutDenominator > 0）
            from web3 import Web3
            from eth_abi import encode as abi_encode
            w3 = Web3(Web3.HTTPProvider("https://polygon-bor-rpc.publicnode.com", request_kwargs={"timeout": 5}))
            CTF = Web3.to_checksum_address("0x4D97DCd97eC945f40cF65F87097ACe5EA0476045")
            cid_bytes = bytes.fromhex(condition_id[2:] if condition_id.startswith("0x") else condition_id)
            denom_sel = Web3.keccak(text="payoutDenominator(bytes32)")[:4]
            result = w3.eth.call({"to": CTF, "data": "0x" + (denom_sel + abi_encode(["bytes32"], [cid_bytes])).hex()})
            if int.from_bytes(result, "big") == 0:
                print(f"[CLAIM] Condition not yet resolved on-chain, skipping: {condition_id[:30]}...", file=sys.stderr)
                return {"success": False, "condition_id": condition_id, "error": "condition not resolved on-chain"}

            # 判断是否为 neg_risk 市场并编码 calldata
            neg_risk = self._is_neg_risk_market(condition_id)
            if neg_risk:
                # NegRiskAdapter: redeemPositions(bytes32, uint256[])
                # 使用最大金额让合约自行裁剪
                data = gasless._encode_redeem_neg_risk(condition_id, [10**18, 10**18])
                to = gasless.neg_risk_adapter_address
            else:
                # CTF: redeemPositions(address, bytes32, bytes32, uint256[])
                data = gasless._encode_redeem(condition_id)
                to = gasless.conditional_tokens_address

            # 构建 Safe 中继交易体（签名 + payload）
            body = gasless._build_safe_relay_transaction(to, data, "redeem")

            # 提交方式：优先 Relayer API Key，其次共享签名服务器
            if relayer_api_key:
                eoa = gasless.get_base_address()
                headers = {
                    "Content-Type": "application/json",
                    "RELAYER_API_KEY": relayer_api_key,
                    "RELAYER_API_KEY_ADDRESS": eoa,
                }
                resp = http_requests.post(
                    "https://relayer-v2.polymarket.com/submit",
                    data=dumps(body), headers=headers, timeout=30,
                )
                resp.raise_for_status()
                result = resp.json()
                state = result.get("state", "")
                tx_hash = result.get("transactionHash", "")

                if state == "STATE_FAILED":
                    print(f"[CLAIM] Relayer STATE_FAILED for {condition_id[:30]}...", file=sys.stderr)
                    return {"success": False, "condition_id": condition_id, "error": "relayer transaction failed"}

                if tx_hash:
                    receipt = w3.eth.wait_for_transaction_receipt(tx_hash, timeout=120)
                    if receipt["status"] == 1:
                        print(f"[CLAIM] ✅ Gasless redeem success! TX: {tx_hash}", file=sys.stderr)
                        return {"success": True, "condition_id": condition_id, "tx_hash": tx_hash, "method": "relayer_api_key"}
                    else:
                        return {"success": False, "condition_id": condition_id, "error": "relayer tx reverted on-chain"}

                print(f"[CLAIM] Relayer accepted: txID={result.get('transactionID', 'N/A')}", file=sys.stderr)
                return {"success": True, "condition_id": condition_id, "tx_id": result.get("transactionID"), "method": "relayer_api_key"}

            # 无 Relayer Key：使用 polymarket-apis 共享签名服务器
            if neg_risk:
                receipt = gasless.redeem_position(condition_id=condition_id, amounts=[10**12, 10**12], neg_risk=True)
            else:
                receipt = gasless.redeem_position(condition_id=condition_id, amounts=[10**12, 10**12], neg_risk=False)

            if receipt.status == 1:
                tx_hash = receipt.transaction_hash.hex() if hasattr(receipt.transaction_hash, "hex") else str(receipt.transaction_hash)
                print(f"[CLAIM] ✅ Gasless redeem success! TX: {tx_hash}", file=sys.stderr)
                return {"success": True, "condition_id": condition_id, "tx_hash": tx_hash, "method": "shared_server"}
            else:
                return {"success": False, "condition_id": condition_id, "error": "gasless transaction reverted"}

        except Exception as e:
            err_msg = str(e)
            print(f"[CLAIM] Gasless failed: {err_msg}", file=sys.stderr)
            return {"success": False, "condition_id": condition_id, "error": err_msg}

    def _is_neg_risk_market(self, condition_id: str) -> bool:
        """查询 CLOB API 判断市场是否为 neg_risk 类型。"""
        try:
            import requests as http_requests
            resp = http_requests.get(f"https://clob.polymarket.com/markets/{condition_id}", timeout=5)
            if resp.status_code == 200:
                return resp.json().get("neg_risk", False)
        except Exception:
            pass
        return False

    def _claim_position_onchain(self, condition_id: str) -> Dict[str, Any]:
        """通过链上交易领取（需要 MATIC gas）。"""
        try:
            from web3 import Web3
            from eth_account import Account
            from eth_abi import encode as abi_encode

            CTF = CTF_ADDRESS
            collateral = PUSD_ADDRESS
            custom_rpc = os.environ.get("POLY_RPC_URL", "")
            RPCS = ([custom_rpc] if custom_rpc else []) + [
                "https://polygon-rpc.com",
                "https://rpc.ankr.com/polygon",
                "https://polygon-bor-rpc.publicnode.com",
            ]

            print(f"[CLAIM] On-chain redeem: {condition_id}", file=sys.stderr)

            private_key = os.environ.get("POLY_PRIVATE_KEY")
            funder = os.environ.get("POLY_FUNDER_ADDRESS", "")

            w3 = None
            for rpc in RPCS:
                try:
                    _w3 = Web3(Web3.HTTPProvider(rpc, request_kwargs={"timeout": 5}))
                    if _w3.is_connected():
                        w3 = _w3
                        break
                except Exception:
                    continue
            if not w3:
                return {"success": False, "condition_id": condition_id, "error": "RPC connection failed"}

            account = Account.from_key(private_key)

            matic_balance = w3.eth.get_balance(account.address)
            gas_price = w3.eth.gas_price
            estimated_cost = 300000 * gas_price
            if matic_balance < estimated_cost:
                matic_ether = matic_balance / 1e18
                cost_ether = estimated_cost / 1e18
                print(f"[CLAIM] Insufficient MATIC: have {matic_ether:.6f}, need ~{cost_ether:.6f}", file=sys.stderr)
                return {"success": False, "condition_id": condition_id,
                        "error": f"insufficient MATIC for gas: have {matic_ether:.6f}, need ~{cost_ether:.6f}"}

            selector = Web3.keccak(text="redeemPositions(address,bytes32,bytes32,uint256[])")[:4]
            cid_hex = condition_id[2:] if condition_id.startswith("0x") else condition_id
            cid_bytes = bytes.fromhex(cid_hex)
            params = abi_encode(
                ["address", "bytes32", "bytes32", "uint256[]"],
                [Web3.to_checksum_address(collateral), b"\x00" * 32, cid_bytes, [1, 2]],
            )
            redeem_data = selector + params

            nonce = w3.eth.get_transaction_count(account.address, 'pending')

            if funder:
                safe_abi = json.loads(
                    '[{"inputs":[{"name":"to","type":"address"},{"name":"value","type":"uint256"},{"name":"data","type":"bytes"},{"name":"operation","type":"uint8"},{"name":"safeTxGas","type":"uint256"},{"name":"baseGas","type":"uint256"},{"name":"gasPrice","type":"uint256"},{"name":"gasToken","type":"address"},{"name":"refundReceiver","type":"address"},{"name":"signatures","type":"bytes"}],"name":"execTransaction","outputs":[{"name":"success","type":"bool"}],"stateMutability":"nonpayable","type":"function"}]'
                )
                safe = w3.eth.contract(address=Web3.to_checksum_address(funder), abi=safe_abi)
                owner_padded = account.address.lower()[2:].zfill(64)
                sig = bytes.fromhex(owner_padded + "0" * 64 + "01")
                zero = "0x0000000000000000000000000000000000000000"
                tx = safe.functions.execTransaction(
                    Web3.to_checksum_address(CTF), 0, redeem_data, 0, 0, 0, 0, zero, zero, sig,
                ).build_transaction({"from": account.address, "nonce": nonce, "gas": 300000, "gasPrice": gas_price, "chainId": 137})
            else:
                tx = {"to": Web3.to_checksum_address(CTF), "data": redeem_data, "from": account.address,
                      "nonce": nonce, "gas": 200000, "gasPrice": gas_price, "chainId": 137, "value": 0}

            signed = account.sign_transaction(tx)
            tx_hash = w3.eth.send_raw_transaction(signed.raw_transaction)
            print(f"[CLAIM] TX sent: {tx_hash.hex()}", file=sys.stderr)
            receipt = w3.eth.wait_for_transaction_receipt(tx_hash, timeout=60)

            if receipt["status"] == 1:
                print(f"[CLAIM] Redeemed! Gas used: {receipt['gasUsed']}", file=sys.stderr)
                return {"success": True, "condition_id": condition_id, "tx_hash": tx_hash.hex()}
            else:
                return {"success": False, "condition_id": condition_id, "error": "transaction reverted"}

        except Exception as e:
            print(f"[CLAIM ERROR] {e}", file=sys.stderr)
            return {"success": False, "condition_id": condition_id, "error": str(e)}

    def claim_batch(self, condition_ids: List[str]) -> Dict[str, Any]:
        """批量领取多个已结算仓位。"""
        import time as _time
        results = []
        total_claimed = 0
        for i, condition_id in enumerate(condition_ids):
            if i > 0:
                _time.sleep(3)  # 等待 nonce 更新
            claim_result = self.claim_position(condition_id)
            results.append(claim_result)
            if claim_result.get("success"):
                total_claimed += 1
        return {
            "success": total_claimed > 0,
            "total_claimed": total_claimed,
            "results": results,
        }

    def auto_redeem(self) -> Dict[str, Any]:
        """自动扫描并领取所有已结算仓位。"""
        import requests as http_requests
        from eth_account import Account

        private_key = os.environ.get("POLY_PRIVATE_KEY", "")
        funder = os.environ.get("POLY_FUNDER_ADDRESS", "")
        eoa = Account.from_key(private_key).address
        wallets = [eoa]
        if funder:
            wallets.append(funder)

        all_redeemable = []
        for addr in wallets:
            try:
                url = f"https://data-api.polymarket.com/positions?user={addr.lower()}"
                resp = http_requests.get(url, timeout=10)
                for p in resp.json():
                    if p.get("redeemable"):
                        all_redeemable.append(p)
            except Exception as e:
                print(f"[AUTO_REDEEM] Query {addr} failed: {e}", file=sys.stderr)

        if not all_redeemable:
            return {
                "success": True,
                "total_claimed": 0,
                "message": "no redeemable positions",
            }

        print(
            f"[AUTO_REDEEM] Found {len(all_redeemable)} redeemable positions",
            file=sys.stderr,
        )
        condition_ids = list(set(p["conditionId"] for p in all_redeemable))
        return self.claim_batch(condition_ids)

    def check_balances(self, token_ids: List[str]) -> Dict[str, float]:
        """Check balances for multiple token IDs."""
        balances = {}
        try:
            signature_type = int(os.environ.get("POLY_SIGNATURE_TYPE", "0"))
        except ValueError:
            print(
                "Error: POLY_SIGNATURE_TYPE must be an integer (0, 1, or 2)",
                file=sys.stderr,
            )
            return balances

        for token_id in token_ids:
            try:
                params = BalanceAllowanceParams(
                    asset_type=AssetType.CONDITIONAL,
                    token_id=token_id,
                    signature_type=signature_type,
                )
                result = self.client.get_balance_allowance(params)
                # Polymarket tokens usually use 6 decimals.
                balance = float(result.get("balance", "0")) / 1e6
                balances[token_id] = balance
                print(
                    f"[BALANCE] Token {token_id[:16]}... balance: {balance}",
                    file=sys.stderr,
                )
            except Exception as e:
                print(
                    f"[BALANCE ERROR] Failed to get balance for {token_id[:16]}...: {e}",
                    file=sys.stderr,
                )
                balances[token_id] = 0.0

        return balances

    def check_collateral_balance(self) -> float:
        """Check the available pUSD balance tracked by the CLOB."""
        try:
            signature_type = int(os.environ.get("POLY_SIGNATURE_TYPE", "0"))
        except ValueError:
            return 0.0

        try:
            params = BalanceAllowanceParams(
                asset_type=AssetType.COLLATERAL,
                signature_type=signature_type,
            )
            result = self.client.get_balance_allowance(params)
            # pUSD uses 6 decimals.
            balance = float(result.get("balance", "0")) / 1e6
            print(f"[BALANCE] pUSD balance: ${balance:.2f}", file=sys.stderr)
            return balance
        except Exception as e:
            print(f"[BALANCE ERROR] Failed to get pUSD balance: {e}", file=sys.stderr)
            return 0.0

    def check_usdc_balance(self) -> float:
        """Backward-compatible alias for legacy callers."""
        return self.check_collateral_balance()


def main():
    executor = PolymarketExecutor()

    if not executor.connect():
        print(
            json.dumps(
                {
                    "success": False,
                    "error_message": executor.connect_error or "Failed to connect",
                    "orders": [],
                    "total_cost": 0,
                    "total_filled": 0,
                }
            )
        )
        sys.exit(1)

    input_data = sys.stdin.read()
    try:
        data = json.loads(input_data)
    except json.JSONDecodeError as e:
        print(
            json.dumps(
                {
                    "success": False,
                    "error_message": f"Invalid JSON input: {e}",
                    "orders": [],
                    "total_cost": 0,
                    "total_filled": 0,
                }
            )
        )
        sys.exit(1)

    # Check if this is a claim request
    if isinstance(data, dict) and data.get("action") == "claim":
        condition_ids = data.get("condition_ids", [])
        if not condition_ids:
            print(json.dumps({"success": False, "error": "No condition_ids provided"}))
            sys.exit(1)
        result = executor.claim_batch(condition_ids)
        print(json.dumps(result, indent=2))
        if not result["success"]:
            sys.exit(1)
        return

    # Check if this is an auto redeem request
    if isinstance(data, dict) and data.get("action") == "auto_redeem":
        result = executor.auto_redeem()
        print(json.dumps(result, indent=2))
        return

    # Cancel all open orders
    if isinstance(data, dict) and data.get("action") == "cancel_all":
        try:
            resp = executor.client.cancel_all()
            print(json.dumps({"success": True, "result": str(resp)}, indent=2))
        except Exception as e:
            print(json.dumps({"success": False, "error": str(e)}, indent=2))
        return

    # Check if this is a balance check request
    if isinstance(data, dict) and data.get("action") == "check_balances":
        token_ids = data.get("token_ids", [])
        if not token_ids:
            print(json.dumps({"success": False, "error": "No token_ids provided"}))
            sys.exit(1)
        balances = executor.check_balances(token_ids)
        print(json.dumps({"success": True, "balances": balances}, indent=2))
        return

    # Check if this is a collateral balance request
    if isinstance(data, dict) and data.get("action") in {"check_collateral_balance", "check_usdc_balance"}:
        balance = executor.check_collateral_balance()
        print(json.dumps({"success": True, "balance": balance}, indent=2))
        return

    # Regular order execution
    orders = data if isinstance(data, list) else [data]
    batch_id = os.environ.get("BATCH_ID", "")
    result = executor.execute_batch(orders, batch_id)

    output = {
        "batch_id": result.batch_id,
        "success": result.success,
        "total_cost": result.total_cost,
        "total_filled": result.total_filled,
        "orders": [
            {
                "token_id": r.token_id,
                "side": r.side,
                "price": r.price,
                "size": r.size,
                "filled_size": r.filled_size,
                "avg_price": r.avg_price,
                "status": r.status.value,
                "error": r.error,
            }
            for r in result.orders
        ],
        "error_message": result.error_message,
    }

    print(json.dumps(output, indent=2))

    if not result.success:
        sys.exit(1)


if __name__ == "__main__":
    main()
