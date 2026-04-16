#!/usr/bin/env python3
import argparse
import json
import time
from datetime import datetime, timedelta
from typing import List, Dict, Optional
import requests


class HistoricalDataCollector:
    def __init__(self):
        self.session = requests.Session()
        self.session.headers.update(
            {
                "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
            }
        )

    def fetch_binance_klines(
        self,
        symbol: str = "BTCUSDT",
        interval: str = "1m",
        start_time: int = None,
        end_time: int = None,
        limit: int = 1000,
    ) -> List[Dict]:
        url = "https://api.binance.com/api/v3/klines"
        params = {"symbol": symbol, "interval": interval, "limit": limit}
        if start_time:
            params["startTime"] = start_time
        if end_time:
            params["endTime"] = end_time

        try:
            resp = self.session.get(url, params=params, timeout=10)
            resp.raise_for_status()
            data = resp.json()

            return [
                {
                    "timestamp": int(k[0]),
                    "open": float(k[1]),
                    "high": float(k[2]),
                    "low": float(k[3]),
                    "close": float(k[4]),
                    "volume": float(k[5]),
                }
                for k in data
            ]
        except Exception as e:
            print(f"[ERROR] Failed to fetch Binance klines: {e}")
            return []

    def fetch_coinbase_candles(
        self,
        product: str = "BTC-USD",
        granularity: int = 60,
        start: str = None,
        end: str = None,
    ) -> List[Dict]:
        url = f"https://api.exchange.coinbase.com/products/{product}/candles"
        params = {"granularity": granularity}
        if start:
            params["start"] = start
        if end:
            params["end"] = end

        try:
            resp = self.session.get(url, params=params, timeout=10)
            resp.raise_for_status()
            data = resp.json()

            return [
                {
                    "timestamp": int(c[0]) * 1000,
                    "low": float(c[1]),
                    "high": float(c[2]),
                    "open": float(c[3]),
                    "close": float(c[4]),
                    "volume": float(c[5]),
                }
                for c in data
            ]
        except Exception as e:
            print(f"[ERROR] Failed to fetch Coinbase candles: {e}")
            return []

    def fetch_chainlink_historical(
        self, start_time: datetime, end_time: datetime
    ) -> List[Dict]:
        url = "https://min-api.cryptocompare.com/data/v2/histominute"
        params = {
            "fsym": "BTC",
            "tsym": "USD",
            "limit": 2000,
            "aggregate": 1,
            "toTs": int(end_time.timestamp()),
        }

        try:
            resp = self.session.get(url, params=params, timeout=10)
            resp.raise_for_status()
            data = resp.json()

            if data.get("Response") == "Error":
                print(f"[ERROR] CryptoCompare API error: {data.get('Message')}")
                return []

            candles = data.get("Data", {}).get("Data", [])
            return [
                {
                    "timestamp": c["time"] * 1000,
                    "open": float(c["open"]),
                    "high": float(c["high"]),
                    "low": float(c["low"]),
                    "close": float(c["close"]),
                    "volume": float(c["volumefrom"]),
                }
                for c in candles
                if c["time"] * 1000 >= int(start_time.timestamp() * 1000)
            ]
        except Exception as e:
            print(f"[ERROR] Failed to fetch CryptoCompare data: {e}")
            return []

    def generate_market_windows(
        self, start_time: datetime, end_time: datetime, window_minutes: int = 5
    ) -> List[Dict]:
        windows = []
        current = start_time.replace(second=0, microsecond=0)
        current = current.replace(minute=(current.minute // 5) * 5)

        while current < end_time:
            window_end = current + timedelta(minutes=window_minutes)
            windows.append(
                {
                    "start": current.isoformat(),
                    "end": window_end.isoformat(),
                    "start_ts": int(current.timestamp() * 1000),
                    "end_ts": int(window_end.timestamp() * 1000),
                }
            )
            current = window_end

        return windows

    def collect_window_data(self, window: Dict) -> Optional[Dict]:
        start_ts = window["start_ts"]
        end_ts = window["end_ts"]

        print(f"[INFO] Collecting data for window {window['start']} to {window['end']}")

        binance_data = self.fetch_binance_klines(
            start_time=start_ts, end_time=end_ts, limit=1000
        )

        if not binance_data:
            print(f"[WARN] No Binance data for window")
            return None

        chainlink_data = self.fetch_chainlink_historical(
            datetime.fromtimestamp(start_ts / 1000),
            datetime.fromtimestamp(end_ts / 1000),
        )

        if not chainlink_data:
            print(f"[WARN] No Chainlink data for window")
            return None

        spot_start = binance_data[0]["close"] if binance_data else None
        spot_end = binance_data[-1]["close"] if binance_data else None

        chainlink_start = chainlink_data[0]["close"] if chainlink_data else None
        chainlink_end = chainlink_data[-1]["close"] if chainlink_data else None

        if not all([spot_start, spot_end, chainlink_start, chainlink_end]):
            print(f"[WARN] Missing price data for window")
            return None

        actual_direction = "UP" if chainlink_end >= chainlink_start else "DOWN"

        spot_change_pct = ((spot_end - spot_start) / spot_start) * 100
        chainlink_change_pct = (
            (chainlink_end - chainlink_start) / chainlink_start
        ) * 100

        return {
            "window_start": window["start"],
            "window_end": window["end"],
            "spot_start": spot_start,
            "spot_end": spot_end,
            "chainlink_start": chainlink_start,
            "chainlink_end": chainlink_end,
            "actual_direction": actual_direction,
            "spot_change_pct": round(spot_change_pct, 4),
            "chainlink_change_pct": round(chainlink_change_pct, 4),
            "spot_data": [
                {"timestamp": d["timestamp"], "price": d["close"]} for d in binance_data
            ],
            "chainlink_data": [
                {"timestamp": d["timestamp"], "price": d["close"]}
                for d in chainlink_data
            ],
        }

    def collect_data(self, days: int = 7, output_file: str = None) -> List[Dict]:
        end_time = datetime.utcnow()
        start_time = end_time - timedelta(days=days)

        print(f"[INFO] Collecting historical data from {start_time} to {end_time}")

        windows = self.generate_market_windows(start_time, end_time)
        print(f"[INFO] Generated {len(windows)} market windows")

        collected_data = []
        for i, window in enumerate(windows):
            print(f"[INFO] Processing window {i + 1}/{len(windows)}")

            window_data = self.collect_window_data(window)
            if window_data:
                collected_data.append(window_data)

            time.sleep(0.5)

            if (i + 1) % 10 == 0 and output_file:
                self.save_progress(collected_data, output_file)
                print(f"[INFO] Saved progress: {len(collected_data)} windows collected")

        print(f"[INFO] Collection complete. Total windows: {len(collected_data)}")
        return collected_data

    def save_progress(self, data: List[Dict], output_file: str):
        try:
            with open(output_file, "w") as f:
                json.dump(data, f, indent=2)
        except Exception as e:
            print(f"[ERROR] Failed to save progress: {e}")


def main():
    parser = argparse.ArgumentParser(
        description="Collect historical price data for backtesting"
    )
    parser.add_argument(
        "--days", type=int, default=7, help="Number of days to collect (default: 7)"
    )
    parser.add_argument(
        "--output",
        type=str,
        default="data/historical_windows.json",
        help="Output file path (default: data/historical_windows.json)",
    )

    args = parser.parse_args()

    collector = HistoricalDataCollector()
    data = collector.collect_data(days=args.days, output_file=args.output)

    collector.save_progress(data, args.output)
    print(f"[SUCCESS] Data saved to {args.output}")
    print(f"[SUMMARY] Total windows collected: {len(data)}")


if __name__ == "__main__":
    main()
