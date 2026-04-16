package polymarket

import (
	"encoding/json"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	WSClobAPIURL = "wss://ws-subscriptions-clob.polymarket.com/ws/market"
)

type WSClient struct {
	conn      *websocket.Conn
	mu        sync.Mutex
	callbacks map[string]func(*OrderbookUpdate)
	assets    []string
	done      chan struct{}
}

type WSSubscriptionMsg struct {
	AssetsIds            []string `json:"assets_ids"`
	Type                 string   `json:"type"`
	CustomFeatureEnabled bool     `json:"custom_feature_enabled"`
}

// OrderbookUpdate represents a price update from WebSocket
type OrderbookUpdate struct {
	EventType string  `json:"event_type"`
	AssetID   string  `json:"asset_id"`
	Bids      []Order `json:"bids"`
	Asks      []Order `json:"asks"`
	// Additional fields from price_change events
	BestBid float64 `json:"best_bid"`
	BestAsk float64 `json:"best_ask"`
}

// WSPriceChangeMessage is the actual format from Polymarket WebSocket
type WSPriceChangeMessage struct {
	Market       string          `json:"market"`
	PriceChanges []WSPriceChange `json:"price_changes"`
}

type WSPriceChange struct {
	AssetID string `json:"asset_id"`
	Price   string `json:"price"`
	Size    string `json:"size"`
	Side    string `json:"side"`
	Hash    string `json:"hash"`
	BestBid string `json:"best_bid"`
	BestAsk string `json:"best_ask"`
}

func NewWSClient() *WSClient {
	return &WSClient{
		callbacks: make(map[string]func(*OrderbookUpdate)),
		done:      make(chan struct{}),
	}
}

func (ws *WSClient) Connect() error {
	err := ws.dial()
	if err != nil {
		return err
	}

	go ws.reconnectLoop()
	return nil
}

func (ws *WSClient) dial() error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if ws.conn != nil {
		ws.conn.Close()
	}

	log.Printf("Connecting to Polymarket WS: %s", WSClobAPIURL)
	conn, _, err := websocket.DefaultDialer.Dial(WSClobAPIURL, nil)
	if err != nil {
		return err
	}
	ws.conn = conn

	// If we were already subscribed to assets, re-subscribe immediately
	if len(ws.assets) > 0 {
		msg := WSSubscriptionMsg{
			AssetsIds:            ws.assets,
			Type:                 "market",
			CustomFeatureEnabled: true,
		}
		err := ws.conn.WriteJSON(msg)
		if err != nil {
			log.Printf("Failed to re-subscribe after dial: %v", err)
		} else {
			log.Printf("Re-subscribed to %d assets", len(ws.assets))
		}
	}

	go ws.readLoop(conn)
	go ws.pingLoop(conn)

	return nil
}

func (ws *WSClient) reconnectLoop() {
	for {
		select {
		case <-ws.done:
			return
		case <-time.After(5 * time.Second):
			ws.mu.Lock()
			connActive := ws.conn != nil
			ws.mu.Unlock()

			if !connActive {
				log.Println("WS connection lost, attempting to reconnect...")
				err := ws.dial()
				if err != nil {
					log.Printf("Reconnection failed: %v", err)
				} else {
					log.Println("Reconnection successful")
				}
			}
		}
	}
}

func (ws *WSClient) pingLoop(conn *websocket.Conn) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ws.done:
			return
		case <-ticker.C:
			ws.mu.Lock()
			err := conn.WriteMessage(websocket.TextMessage, []byte("PING"))
			ws.mu.Unlock()
			if err != nil {
				log.Printf("WS ping error: %v", err)
				ws.mu.Lock()
				if ws.conn == conn {
					ws.conn = nil
				}
				ws.mu.Unlock()
				conn.Close()
				return
			}
		}
	}
}

func (ws *WSClient) readLoop(conn *websocket.Conn) {
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("WS read error: %v", err)
			ws.mu.Lock()
			if ws.conn == conn {
				ws.conn = nil
			}
			ws.mu.Unlock()
			conn.Close()
			return
		}

		if string(message) == "PONG" {
			continue
		}

		// Parse the actual Polymarket WebSocket format
		var priceMsg WSPriceChangeMessage
		if err := json.Unmarshal(message, &priceMsg); err == nil && len(priceMsg.PriceChanges) > 0 {
			// Collect callbacks and updates under lock, invoke outside
			type callbackEntry struct {
				cb     func(*OrderbookUpdate)
				update *OrderbookUpdate
			}
			var entries []callbackEntry

			ws.mu.Lock()
			for _, pc := range priceMsg.PriceChanges {
				if cb, exists := ws.callbacks[pc.AssetID]; exists {
					update := &OrderbookUpdate{
						EventType: "price_change",
						AssetID:   pc.AssetID,
					}
					if pc.BestBid != "" {
						bid, _ := strconv.ParseFloat(pc.BestBid, 64)
						update.BestBid = bid
						update.Bids = []Order{{Price: pc.BestBid, Size: "1"}}
					}
					if pc.BestAsk != "" {
						ask, _ := strconv.ParseFloat(pc.BestAsk, 64)
						update.BestAsk = ask
						update.Asks = []Order{{Price: pc.BestAsk, Size: "1"}}
					}
					entries = append(entries, callbackEntry{cb: cb, update: update})
				}
			}
			ws.mu.Unlock()

			for _, e := range entries {
				e.cb(e.update)
			}
			continue
		}

		// Fallback: Try parsing as array of orderbook updates (Initial Snapshot)
		var updates []OrderbookUpdate
		if err := json.Unmarshal(message, &updates); err == nil && len(updates) > 0 {
			type callbackEntry struct {
				cb     func(*OrderbookUpdate)
				update *OrderbookUpdate
			}
			var entries []callbackEntry

			ws.mu.Lock()
			for _, update := range updates {
				update.EventType = "book"
				if cb, exists := ws.callbacks[update.AssetID]; exists {
					u := update
					entries = append(entries, callbackEntry{cb: cb, update: &u})
				}
			}
			ws.mu.Unlock()

			for _, e := range entries {
				e.cb(e.update)
			}
			continue
		}

		// Fallback: Try parsing as single object
		var singleUpdate OrderbookUpdate
		if err := json.Unmarshal(message, &singleUpdate); err == nil {
			if singleUpdate.EventType == "book" && singleUpdate.AssetID != "" {
				ws.mu.Lock()
				cb, exists := ws.callbacks[singleUpdate.AssetID]
				ws.mu.Unlock()
				if exists {
					cb(&singleUpdate)
				}
			}
		}
	}
}

// Subscribe sends a subscription message for specific token IDs
func (ws *WSClient) Subscribe(tokenIDs []string, callback func(*OrderbookUpdate)) error {
	ws.mu.Lock()
	ws.assets = tokenIDs
	for _, id := range tokenIDs {
		ws.callbacks[id] = callback
	}

	if ws.conn == nil {
		ws.mu.Unlock()
		return nil // Reconnect loop will handle it
	}

	msg := WSSubscriptionMsg{
		AssetsIds:            tokenIDs,
		Type:                 "market",
		CustomFeatureEnabled: true,
	}

	err := ws.conn.WriteJSON(msg)
	ws.mu.Unlock()
	return err
}

// Close closes the WebSocket connection
func (ws *WSClient) Close() {
	close(ws.done)
	ws.mu.Lock()
	if ws.conn != nil {
		ws.conn.Close()
		ws.conn = nil
	}
	ws.mu.Unlock()
}
