package main

import (
	"fmt"
	"log"

	"poly-scan/internal/btc"
	"poly-scan/internal/polymarket"
)

func main() {
	fmt.Println("=== Poly-Scan Integration Test ===")
	fmt.Println()

	client := polymarket.NewClient()

	fmt.Println("Fetching markets...")
	markets, err := client.GetMarkets(20)
	if err != nil {
		log.Fatalf("Failed to fetch markets: %v", err)
	}
	fmt.Printf("Fetched %d markets\n\n", len(markets))

	for _, market := range markets {
		if len(market.Tokens) < 2 {
			continue
		}

		ob, err := client.GetOrderbook(market.Tokens[0].TokenID)
		if err != nil {
			continue
		}

		sumBestAsks := 0.0
		validMarket := true
		if len(ob.Asks) == 0 {
			validMarket = false
		} else {
			var price float64
			fmt.Sscanf(ob.Asks[0].Price, "%f", &price)
			sumBestAsks += price
		}

		ob2, err := client.GetOrderbook(market.Tokens[1].TokenID)
		if err != nil || len(ob2.Asks) == 0 {
			validMarket = false
		} else {
			var price float64
			fmt.Sscanf(ob2.Asks[0].Price, "%f", &price)
			sumBestAsks += price
		}

		if !validMarket {
			continue
		}

		if sumBestAsks < 0.985 {
			fmt.Printf("🔍 Market: %s | Sum asks: %.4f\n", market.Question, sumBestAsks)
		}
	}

	fmt.Println("\n=== Testing Technical Indicators ===")
	indicators := btc.NewTechnicalIndicators()
	prices := []float64{100, 101, 102, 101, 100, 99, 100, 101, 102, 103, 104, 103, 102, 101, 100, 101, 102, 103, 104, 105, 104, 103, 102, 101, 100, 99, 100, 101}
	rsi := indicators.RSI(prices, 14)
	macd := indicators.MACD(prices)
	bollinger := indicators.Bollinger(prices, 20, 2.0)
	fmt.Printf("  RSI(14): %.1f\n", rsi)
	fmt.Printf("  MACD Hist: %.4f\n", macd.Histogram)
	fmt.Printf("  Bollinger %%B: %.1f%%\n", bollinger.PercentB)

	fmt.Println("\n=== Test Complete ===")
}
