#!/usr/bin/env python3
"""
策略实时监控脚本
监控 BTC 策略的置信度和交易状态
当置信度接近阈值时发送通知
"""

import json
import time
import subprocess
from datetime import datetime
from pathlib import Path

# 配置
LOG_FILE = "/root/poly-scan/logs/btc-out.log"
POSITIONS_FILE = "/root/poly-scan/data/positions.json"
CONFIDENCE_THRESHOLD = 0.65
NOTIFY_THRESHOLD = 0.60  # 置信度达到此值时通知
CHECK_INTERVAL = 10  # 检查间隔（秒）


class StrategyMonitor:
    def __init__(self):
        self.last_position_count = 0
        self.high_confidence_count = 0
        self.last_notification_time = 0

    def get_latest_confidence(self):
        """从日志获取最新的置信度"""
        try:
            result = subprocess.run(
                ["tail", "-n", "5", LOG_FILE], capture_output=True, text=True
            )
            lines = result.stdout.strip().split("\n")

            for line in reversed(lines):
                if "confidence=" in line and "threshold=0.65" in line:
                    # 解析置信度
                    parts = line.split("confidence=")
                    if len(parts) > 1:
                        conf_str = parts[1].split()[0]
                        return float(conf_str)
            return None
        except Exception as e:
            print(f"[{datetime.now().strftime('%H:%M:%S')}] 读取日志错误: {e}")
            return None

    def get_position_count(self):
        """获取当前持仓数量"""
        try:
            if Path(POSITIONS_FILE).exists():
                with open(POSITIONS_FILE, "r") as f:
                    positions = json.load(f)
                    return len(positions)
            return 0
        except:
            return 0

    def check_strategy_health(self):
        """检查策略健康状态"""
        try:
            # 检查 PM2 状态
            result = subprocess.run(
                ["pm2", "status", "poly-bot-btc", "--no-color"],
                capture_output=True,
                text=True,
            )
            if "online" in result.stdout:
                return True, "运行中"
            else:
                return False, "未运行"
        except:
            return False, "检查失败"

    def notify(self, message):
        """发送通知（打印到控制台）"""
        timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        print(f"\n{'=' * 60}")
        print(f"[通知] {timestamp}")
        print(f"{message}")
        print(f"{'=' * 60}\n")

    def run(self):
        """主监控循环"""
        print(f"[{datetime.now().strftime('%H:%M:%S')}] 策略监控启动")
        print(f"置信度阈值: {CONFIDENCE_THRESHOLD}")
        print(f"通知阈值: {NOTIFY_THRESHOLD}")
        print(f"检查间隔: {CHECK_INTERVAL}秒")
        print("-" * 60)

        while True:
            try:
                # 检查策略健康
                healthy, status = self.check_strategy_health()
                if not healthy:
                    self.notify(f"⚠️ 策略状态异常: {status}")

                # 获取最新置信度
                confidence = self.get_latest_confidence()

                if confidence is not None:
                    # 检查是否接近交易阈值
                    if (
                        confidence >= NOTIFY_THRESHOLD
                        and confidence < CONFIDENCE_THRESHOLD
                    ):
                        self.high_confidence_count += 1
                        if self.high_confidence_count >= 3:  # 连续3次高置信度才通知
                            self.notify(
                                f"📊 高置信度机会!\n"
                                f"当前置信度: {confidence:.2%}\n"
                                f"距离交易阈值: {(CONFIDENCE_THRESHOLD - confidence):.2%}"
                            )
                            self.high_confidence_count = 0
                    else:
                        self.high_confidence_count = 0

                    # 检查是否触发交易
                    if confidence >= CONFIDENCE_THRESHOLD:
                        current_time = time.time()
                        if (
                            current_time - self.last_notification_time > 300
                        ):  # 5分钟内只通知一次
                            self.notify(
                                f"🚀 交易信号触发!\n"
                                f"置信度: {confidence:.2%} >= 阈值 {CONFIDENCE_THRESHOLD:.2%}"
                            )
                            self.last_notification_time = current_time

                # 检查新交易
                current_positions = self.get_position_count()
                if current_positions > self.last_position_count:
                    self.notify(f"✅ 新交易执行! 总持仓: {current_positions}")
                self.last_position_count = current_positions

                # 每30秒打印一次状态
                if int(time.time()) % 30 == 0 and confidence is not None:
                    print(
                        f"[{datetime.now().strftime('%H:%M:%S')}] 置信度: {confidence:.2%} | 持仓: {current_positions}"
                    )

                time.sleep(CHECK_INTERVAL)

            except KeyboardInterrupt:
                print(f"\n[{datetime.now().strftime('%H:%M:%S')}] 监控已停止")
                break
            except Exception as e:
                print(f"[{datetime.now().strftime('%H:%M:%S')}] 错误: {e}")
                time.sleep(CHECK_INTERVAL)


if __name__ == "__main__":
    monitor = StrategyMonitor()
    monitor.run()
