package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"poly-scan/internal/btc"
)

// PriceDataPoint represents a single price observation
type PriceDataPoint struct {
	Timestamp int64   `json:"timestamp"`
	Price     float64 `json:"price"`
	Source    string  `json:"source"`
}

// MarketWindow represents a 5-minute trading window
type MarketWindow struct {
	Start           time.Time        `json:"start"`
	End             time.Time        `json:"end"`
	ChainlinkStart  float64          `json:"chainlink_start"`
	ChainlinkEnd    float64          `json:"chainlink_end"`
	ChainlinkData   []PriceDataPoint `json:"chainlink_data"`
	SpotData        []PriceDataPoint `json:"spot_data"`
	ActualDirection string           `json:"actual_direction"` // "UP" or "DOWN"
}

// DataCollector collects historical price data for backtesting
type DataCollector struct {
	windows    []MarketWindow
	outputFile string
}

// NewDataCollector creates a new data collector
func NewDataCollector(outputFile string) *DataCollector {
	return &DataCollector{
		windows:    make([]MarketWindow, 0),
		outputFile: outputFile,
	}
}

// CollectHistoricalData collects data for the specified time range
func (dc *DataCollector) CollectHistoricalData(startTime, endTime time.Time) error {
	log.Printf("[DATA] 开始收集历史数据: %s 到 %s", startTime.Format("2006-01-02 15:04"), endTime.Format("2006-01-02 15:04"))

	// Generate 5-minute windows
	current := startTime
	for current.Before(endTime) {
		windowEnd := current.Add(5 * time.Minute)
		if windowEnd.After(endTime) {
			windowEnd = endTime
		}

		window := MarketWindow{
			Start: current,
			End:   windowEnd,
		}

		// Collect Chainlink data for this window
		chainlinkData, err := dc.fetchChainlinkData(current, windowEnd)
		if err != nil {
			log.Printf("[DATA] 警告: 获取Chainlink数据失败 %s: %v", current.Format("15:04"), err)
		} else {
			window.ChainlinkData = chainlinkData
			if len(chainlinkData) > 0 {
				window.ChainlinkStart = chainlinkData[0].Price
				window.ChainlinkEnd = chainlinkData[len(chainlinkData)-1].Price
			}
		}

		// Collect spot price data
		spotData, err := dc.fetchSpotData(current, windowEnd)
		if err != nil {
			log.Printf("[DATA] 警告: 获取现货数据失败 %s: %v", current.Format("15:04"), err)
		} else {
			window.SpotData = spotData
		}

		// Determine actual settlement direction
		if window.ChainlinkStart > 0 && window.ChainlinkEnd > 0 {
			if window.ChainlinkEnd >= window.ChainlinkStart {
				window.ActualDirection = "UP"
			} else {
				window.ActualDirection = "DOWN"
			}
		}

		dc.windows = append(dc.windows, window)
		log.Printf("[DATA] 已收集窗口 %s - Chainlink: %.2f -> %.2f (%s)",
			current.Format("15:04"), window.ChainlinkStart, window.ChainlinkEnd, window.ActualDirection)

		current = windowEnd
		time.Sleep(100 * time.Millisecond) // Rate limiting
	}

	return dc.saveToFile()
}

// fetchChainlinkData fetches Chainlink oracle data
func (dc *DataCollector) fetchChainlinkData(start, end time.Time) ([]PriceDataPoint, error) {
	// Try multiple sources for Chainlink data

	// Source 1: Chainlink Data Streams API (if available)
	data, err := dc.fetchFromChainlinkAPI(start, end)
	if err == nil && len(data) > 0 {
		return data, nil
	}

	// Source 2: CoinGecko (as proxy for oracle-like data)
	data, err = dc.fetchFromCoinGecko(start, end)
	if err == nil && len(data) > 0 {
		return data, nil
	}

	// Source 3: CryptoCompare
	data, err = dc.fetchFromCryptoCompare(start, end)
	if err == nil && len(data) > 0 {
		return data, nil
	}

	return nil, fmt.Errorf("所有Chainlink数据源均失败")
}

// fetchFromChainlinkAPI attempts to fetch from Chainlink
func (dc *DataCollector) fetchFromChainlinkAPI(start, end time.Time) ([]PriceDataPoint, error) {
	return dc.fetchFromCoinGecko(start, end)
}

// fetchFromCoinGecko fetches BTC price from CoinGecko
func (dc *DataCollector) fetchFromCoinGecko(start, end time.Time) ([]PriceDataPoint, error) {
	url := fmt.Sprintf("https://api.coingecko.com/api/v3/coins/bitcoin/market_chart/range?vs_currency=usd&from=%d&to=%d",
		start.Unix(), end.Unix())

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("CoinGecko API返回状态码: %d", resp.StatusCode)
	}

	var result struct {
		Prices [][]float64 `json:"prices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	data := make([]PriceDataPoint, 0, len(result.Prices))
	for _, p := range result.Prices {
		if len(p) >= 2 {
			data = append(data, PriceDataPoint{
				Timestamp: int64(p[0]) / 1000, // Convert ms to seconds
				Price:     p[1],
				Source:    "coingecko",
			})
		}
	}

	return data, nil
}

// fetchFromCryptoCompare fetches BTC price from CryptoCompare
func (dc *DataCollector) fetchFromCryptoCompare(start, end time.Time) ([]PriceDataPoint, error) {
	// CryptoCompare free API
	url := fmt.Sprintf("https://min-api.cryptocompare.com/data/v2/histominute?fsym=BTC&tsym=USD&limit=5&toTs=%d",
		end.Unix())

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("CryptoCompare API返回状态码: %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			Data []struct {
				Time  int64   `json:"time"`
				Close float64 `json:"close"`
			} `json:"Data"`
		} `json:"Data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	data := make([]PriceDataPoint, 0, len(result.Data.Data))
	for _, p := range result.Data.Data {
		data = append(data, PriceDataPoint{
			Timestamp: p.Time,
			Price:     p.Close,
			Source:    "cryptocompare",
		})
	}

	return data, nil
}

// fetchSpotData fetches spot price data from Binance and Coinbase
func (dc *DataCollector) fetchSpotData(start, end time.Time) ([]PriceDataPoint, error) {
	data := make([]PriceDataPoint, 0)

	// Try Binance
	binanceData, err := dc.fetchFromBinance(start, end)
	if err == nil {
		data = append(data, binanceData...)
	}

	// Try Coinbase
	coinbaseData, err := dc.fetchFromCoinbase(start, end)
	if err == nil {
		data = append(data, coinbaseData...)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("无法获取现货价格数据")
	}

	return data, nil
}

// fetchFromBinance fetches from Binance API
func (dc *DataCollector) fetchFromBinance(start, end time.Time) ([]PriceDataPoint, error) {
	// Binance klines API (1 minute intervals)
	url := fmt.Sprintf("https://api.binance.com/api/v3/klines?symbol=BTCUSDT&interval=1m&startTime=%d&endTime=%d&limit=5",
		start.UnixMilli(), end.UnixMilli())

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Binance API返回状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var klines [][]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&klines); err != nil {
		return nil, err
	}

	data := make([]PriceDataPoint, 0, len(klines))
	for _, k := range klines {
		if len(k) >= 5 {
			timestamp := int64(k[0].(float64)) / 1000
			price := k[4].(string) // Close price
			var priceFloat float64
			fmt.Sscanf(price, "%f", &priceFloat)
			data = append(data, PriceDataPoint{
				Timestamp: timestamp,
				Price:     priceFloat,
				Source:    "binance",
			})
		}
	}

	return data, nil
}

// fetchFromCoinbase fetches from Coinbase API
func (dc *DataCollector) fetchFromCoinbase(start, end time.Time) ([]PriceDataPoint, error) {
	// Coinbase candles API
	url := fmt.Sprintf("https://api.exchange.coinbase.com/products/BTC-USD/candles?granularity=60&start=%s&end=%s",
		start.Format(time.RFC3339), end.Format(time.RFC3339))

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Coinbase API返回状态码: %d", resp.StatusCode)
	}

	var candles [][]float64
	if err := json.NewDecoder(resp.Body).Decode(&candles); err != nil {
		return nil, err
	}

	data := make([]PriceDataPoint, 0, len(candles))
	for _, c := range candles {
		if len(c) >= 5 {
			data = append(data, PriceDataPoint{
				Timestamp: int64(c[0]),
				Price:     c[4], // Close price
				Source:    "coinbase",
			})
		}
	}

	return data, nil
}

// saveToFile saves collected data to JSON file
func (dc *DataCollector) saveToFile() error {
	file, err := os.Create(dc.outputFile)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(dc.windows); err != nil {
		return err
	}

	log.Printf("[DATA] 数据已保存到: %s (共 %d 个窗口)", dc.outputFile, len(dc.windows))
	return nil
}

// LoadWindows loads previously collected windows from file
func LoadWindows(filename string) ([]MarketWindow, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var windows []MarketWindow
	if err := json.NewDecoder(file).Decode(&windows); err != nil {
		return nil, err
	}

	return windows, nil
}

func main() {
	log.Println("[DATA] BTC历史数据收集工具")
	log.Println("[DATA] ====================")

	// Default: collect last 7 days of data
	endTime := time.Now()
	startTime := endTime.Add(-7 * 24 * time.Hour)

	// Allow custom date range via command line
	if len(os.Args) >= 3 {
		var err error
		startTime, err = time.Parse("2006-01-02", os.Args[1])
		if err != nil {
			log.Fatalf("[DATA] 无效的开始日期格式，使用 YYYY-MM-DD: %v", err)
		}
		endTime, err = time.Parse("2006-01-02", os.Args[2])
		if err != nil {
			log.Fatalf("[DATA] 无效的结束日期格式，使用 YYYY-MM-DD: %v", err)
		}
	}

	outputFile := "data/historical_windows.json"
	if len(os.Args) >= 4 {
		outputFile = os.Args[3]
	}

	collector := NewDataCollector(outputFile)
	if err := collector.CollectHistoricalData(startTime, endTime); err != nil {
		log.Fatalf("[DATA] 数据收集失败: %v", err)
	}

	log.Println("[DATA] 数据收集完成!")
	log.Printf("[DATA] 使用命令运行回测: go run cmd/backtest/main.go %s", outputFile)
}

// Ensure btc package types are available for backtest
func init() {
	_ = btc.PricePoint{} // Verify btc package is importable
}
