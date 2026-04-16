package main

import (
	"fmt"
	"log"
	"os"

	"poly-scan/internal/execution"
	"poly-scan/internal/polymarket"
)

func main() {
	fmt.Println("=== 订单执行功能测试 ===")

	creds := &execution.L2Credentials{
		APIKey:        os.Getenv("POLY_API_KEY"),
		APISecret:     os.Getenv("POLY_API_SECRET"),
		Passphrase:    os.Getenv("POLY_PASSPHRASE"),
		PrivateKey:    os.Getenv("POLY_PRIVATE_KEY"),
		SignatureType: "0",
		FunderAddress: "",
	}

	if creds.PrivateKey == "" {
		log.Fatal("请确保已设置 POLY_PRIVATE_KEY 环境变量")
	}

	engine := execution.NewEngine(creds)
	client := polymarket.NewClient()

	// 1. 获取一个市场以获取合法的 token_id
	fmt.Println("\n[1] 正在获取市场...")
	markets, err := client.GetMarkets(1)
	if err != nil || len(markets) == 0 {
		log.Fatalf("无法获取市场: %v", err)
	}
	market := markets[0]
	if len(market.Tokens) < 2 {
		log.Fatal("市场缺乏 token")
	}
	tokenID := market.Tokens[0].TokenID
	fmt.Printf("选取测试市场: %s\n", market.Question)
	fmt.Printf("测试 Token ID: %s\n", tokenID)

	// 2. 测试查询余额
	fmt.Println("\n[2] 测试余额查询...")
	balances, err := engine.CheckBalances([]string{tokenID})
	if err != nil {
		fmt.Printf("查询余额失败: %v\n", err)
	} else {
		fmt.Printf("余额查询成功: %v\n", balances)
	}

	// 3. 测试下单功能 (非常低价以免成交，以验证签名机制)
	fmt.Println("\n[3] 测试发送订单...")
	testOrder := []execution.Order{
		{
			TokenID:   tokenID,
			Price:     0.01,
			Size:      50.0, // 至少 50 份 以便达到多边形的最少要求 ($0.5), 因价很小通常不会成交或返回余额不足
			Side:      "BUY",
			OrderType: "FOK",
		},
	}
	res, err := engine.ExecuteBatch("test_batch", testOrder)
	if err != nil {
		fmt.Printf("下单执行返回错误: %v\n", err)
		if res != nil {
			fmt.Printf("错误详情: %s\n", res.ErrorMessage)
		}
	} else {
		fmt.Printf("下单请求发送成功!\n")
		fmt.Printf("执行结果: Success=%v, ErrorMessage=%s\n", res.Success, res.ErrorMessage)
		for _, o := range res.Orders {
			fmt.Printf(" -> 状态: %s, 错误(如果有): %s\n", o.Status, o.Error)
		}
	}

	// 4. 测试 Claim 请求机制是否抛错
	fmt.Println("\n[4] 测试 Claim 机制...")
	claimRes, err := engine.ClaimPositions([]string{"dummy_condition_id"})
	if err != nil {
		fmt.Printf("Claim 请求失败: %v\n", err)
	} else {
		fmt.Printf("Claim 请求成功响应: %+v\n", claimRes)
	}

	fmt.Println("\n=== 测试结束 ===")
}
