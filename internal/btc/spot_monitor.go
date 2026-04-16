package btc

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// BinanceTicker represents Binance ticker response
type BinanceTicker struct {
	Symbol string `json:"symbol"`
	Price  string `json:"price"`
}

// PricePoint represents a price at a specific time
type PricePoint struct {
	Price     float64
	Timestamp time.Time
}

// SpotMonitor monitors real-time BTC price from multiple exchanges
type SpotMonitor struct {
	client       *http.Client
	currentPrice float64
	priceHistory []PricePoint
	mu           sync.RWMutex
	stopChan     chan struct{}
}

// NewSpotMonitor creates a new spot price monitor
func NewSpotMonitor() *SpotMonitor {
	return &SpotMonitor{
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		priceHistory: make([]PricePoint, 0, 100),
		stopChan:     make(chan struct{}),
	}
}

// Start begins monitoring BTC price
func (m *SpotMonitor) Start() {
	go m.pollLoop()
}

// Stop stops the monitor
func (m *SpotMonitor) Stop() {
	close(m.stopChan)
}

// pollLoop polls exchanges for price updates
func (m *SpotMonitor) pollLoop() {
	ticker := time.NewTicker(500 * time.Millisecond) // Poll every 500ms
	defer ticker.Stop()

	for {
		select {
		case <-m.stopChan:
			return
		case <-ticker.C:
			m.fetchPrice()
		}
	}
}

// fetchPrice fetches BTC price from Binance
func (m *SpotMonitor) fetchPrice() {
	// Try Binance first
	price, err := m.fetchBinancePrice()
	if err != nil {
		// Fallback to Coinbase
		price, err = m.fetchCoinbasePrice()
		if err != nil {
			log.Printf("[SPOT] Failed to fetch price: %v", err)
			return
		}
	}

	m.mu.Lock()
	m.currentPrice = price
	// Keep last 100 price points (50 seconds at 500ms intervals)
	m.priceHistory = append(m.priceHistory, PricePoint{
		Price:     price,
		Timestamp: time.Now(),
	})
	if len(m.priceHistory) > 100 {
		m.priceHistory = m.priceHistory[1:]
	}
	m.mu.Unlock()
}

// fetchBinancePrice fetches BTC price from Binance
func (m *SpotMonitor) fetchBinancePrice() (float64, error) {
	resp, err := m.client.Get("https://api.binance.com/api/v3/ticker/price?symbol=BTCUSDT")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var ticker BinanceTicker
	if err := json.Unmarshal(body, &ticker); err != nil {
		return 0, err
	}

	var price float64
	fmt.Sscanf(ticker.Price, "%f", &price)
	return price, nil
}

// CoinbaseResponse represents Coinbase price response
type CoinbaseResponse struct {
	Data struct {
		Amount   string `json:"amount"`
		Base     string `json:"base"`
		Currency string `json:"currency"`
	} `json:"data"`
}

// fetchCoinbasePrice fetches BTC price from Coinbase
func (m *SpotMonitor) fetchCoinbasePrice() (float64, error) {
	resp, err := m.client.Get("https://api.coinbase.com/v2/prices/BTC-USD/spot")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var cb CoinbaseResponse
	if err := json.Unmarshal(body, &cb); err != nil {
		return 0, err
	}

	var price float64
	fmt.Sscanf(cb.Data.Amount, "%f", &price)
	return price, nil
}

// GetCurrentPrice returns the current BTC price
func (m *SpotMonitor) GetCurrentPrice() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentPrice
}

// GetPriceHistory returns recent price history
func (m *SpotMonitor) GetPriceHistory() []PricePoint {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]PricePoint, len(m.priceHistory))
	copy(result, m.priceHistory)
	return result
}

// GetPriceChange calculates price change over a duration
func (m *SpotMonitor) GetPriceChange(duration time.Duration) (startPrice, endPrice, changePct float64, valid bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.priceHistory) < 2 {
		return 0, 0, 0, false
	}

	now := time.Now()
	cutoff := now.Add(-duration)

	// Find price at cutoff time
	var startIdx int
	for i, p := range m.priceHistory {
		if p.Timestamp.After(cutoff) {
			startIdx = i
			break
		}
	}

	if startIdx >= len(m.priceHistory)-1 {
		return 0, 0, 0, false
	}

	startPrice = m.priceHistory[startIdx].Price
	endPrice = m.priceHistory[len(m.priceHistory)-1].Price
	if startPrice == 0 {
		return 0, 0, 0, false
	}
	changePct = (endPrice - startPrice) / startPrice * 100

	return startPrice, endPrice, changePct, true
}

// GetTrend analyzes recent price trend over the specified number of seconds
func (m *SpotMonitor) GetTrend(seconds int) (trend string, strength float64) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.priceHistory) < 4 {
		return "neutral", 0
	}

	// Filter price history to the requested time window
	cutoff := time.Now().Add(-time.Duration(seconds) * time.Second)
	var window []PricePoint
	for _, p := range m.priceHistory {
		if p.Timestamp.After(cutoff) {
			window = append(window, p)
		}
	}

	if len(window) < 4 {
		return "neutral", 0
	}

	// Compare first half vs second half of the windowed data
	mid := len(window) / 2
	firstHalf := window[:mid]
	secondHalf := window[mid:]

	var firstAvg, secondAvg float64
	for _, p := range firstHalf {
		firstAvg += p.Price
	}
	firstAvg /= float64(len(firstHalf))

	for _, p := range secondHalf {
		secondAvg += p.Price
	}
	secondAvg /= float64(len(secondHalf))

	if firstAvg == 0 {
		return "neutral", 0
	}

	change := (secondAvg - firstAvg) / firstAvg * 100

	if change > 0.01 {
		strength = change
		trend = "up"
	} else if change < -0.01 {
		strength = -change
		trend = "down"
	} else {
		trend = "neutral"
		strength = 0
	}

	return trend, strength
}
