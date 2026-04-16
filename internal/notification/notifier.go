package notification

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// Level represents notification severity
type Level string

const (
	LevelInfo    Level = "info"
	LevelWarning Level = "warning"
	LevelError   Level = "error"
	LevelSuccess Level = "success"
)

// EventType represents the type of event
type EventType string

const (
	EventTradeExecuted   EventType = "trade_executed"
	EventTradeSettled    EventType = "trade_settled"
	EventClaimSuccess    EventType = "claim_success"
	EventClaimFailed     EventType = "claim_failed"
	EventRiskWarning     EventType = "risk_warning"
	EventCooldownEntered EventType = "cooldown_entered"
	EventDailyLimitHit   EventType = "daily_limit_hit"
	EventSystemError     EventType = "system_error"
)

// Event represents a notification event
type Event struct {
	Type      EventType              `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	MarketID  string                 `json:"market_id,omitempty"`
	Message   string                 `json:"message"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Level     Level                  `json:"level"`
}

// Config holds notification configuration
type Config struct {
	Enabled        bool   `json:"enabled"`
	WebhookURL     string `json:"webhook_url"`
	TelegramToken  string `json:"telegram_token"`
	TelegramChatID string `json:"telegram_chat_id"`
	MinLevel       Level  `json:"min_level"`
	RateLimit      int    `json:"rate_limit"`  // Max notifications per minute per type
	QuietHours     string `json:"quiet_hours"` // e.g., "22:00-08:00"
}

// DefaultConfig returns default notification configuration
func DefaultConfig() Config {
	return Config{
		Enabled:   true,
		MinLevel:  LevelInfo,
		RateLimit: 5,
	}
}

// Notifier handles sending notifications
type Notifier struct {
	config Config
	mu     sync.RWMutex

	// Rate limiting
	eventCounts  map[EventType]int
	lastReset    time.Time
	recentEvents []Event

	// HTTP client
	client *http.Client
}

// NewNotifier creates a new notifier
func NewNotifier(config Config) *Notifier {
	return &Notifier{
		config:       config,
		eventCounts:  make(map[EventType]int),
		recentEvents: make([]Event, 0, 100),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		lastReset: time.Now(),
	}
}

// Notify sends a notification
func (n *Notifier) Notify(event Event) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.config.Enabled {
		return nil
	}

	// Check minimum level
	if !n.shouldNotify(event) {
		return nil
	}

	// Check rate limit
	if !n.checkRateLimit(event.Type) {
		log.Printf("[NOTIFICATION] Rate limited: %s", event.Type)
		return nil
	}

	// Check quiet hours
	if n.isQuietHours() {
		// Only send errors during quiet hours
		if event.Level != LevelError {
			return nil
		}
	}

	// Set timestamp
	event.Timestamp = time.Now()

	// Store event
	n.recentEvents = append(n.recentEvents, event)
	if len(n.recentEvents) > 100 {
		n.recentEvents = n.recentEvents[1:]
	}

	// Persist events to file for API access
	n.saveEventsToFile()

	// Send to all configured channels
	var errors []error

	if n.config.WebhookURL != "" {
		if err := n.sendWebhook(event); err != nil {
			errors = append(errors, fmt.Errorf("webhook: %v", err))
		}
	}

	if n.config.TelegramToken != "" && n.config.TelegramChatID != "" {
		if err := n.sendTelegram(event); err != nil {
			errors = append(errors, fmt.Errorf("telegram: %v", err))
		}
	}

	// Always log
	n.logEvent(event)

	if len(errors) > 0 {
		return fmt.Errorf("notification errors: %v", errors)
	}
	return nil
}

// SetWebhookURL updates the webhook URL
func (n *Notifier) SetWebhookURL(url string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.config.WebhookURL = url
}

// SetTelegram updates Telegram configuration
func (n *Notifier) SetTelegram(token, chatID string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.config.TelegramToken = token
	n.config.TelegramChatID = chatID
}

// SetEnabled enables or disables notifications
func (n *Notifier) SetEnabled(enabled bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.config.Enabled = enabled
}

// GetRecentEvents returns recent notification events
func (n *Notifier) GetRecentEvents(limit int) []Event {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if limit <= 0 || limit > len(n.recentEvents) {
		limit = len(n.recentEvents)
	}

	result := make([]Event, limit)
	copy(result, n.recentEvents[len(n.recentEvents)-limit:])
	return result
}

// GetConfig returns current configuration
func (n *Notifier) GetConfig() Config {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.config
}

// Internal methods

func (n *Notifier) shouldNotify(event Event) bool {
	levels := map[Level]int{
		LevelInfo:    0,
		LevelSuccess: 0,
		LevelWarning: 1,
		LevelError:   2,
	}

	minLevel, ok := levels[n.config.MinLevel]
	if !ok {
		minLevel = 0
	}

	eventLevel, ok := levels[event.Level]
	if !ok {
		eventLevel = 0
	}

	return eventLevel >= minLevel
}

func (n *Notifier) checkRateLimit(eventType EventType) bool {
	// Reset counters every minute
	if time.Since(n.lastReset) > time.Minute {
		n.eventCounts = make(map[EventType]int)
		n.lastReset = time.Now()
	}

	count := n.eventCounts[eventType]
	if count >= n.config.RateLimit {
		return false
	}

	n.eventCounts[eventType]++
	return true
}

func (n *Notifier) isQuietHours() bool {
	if n.config.QuietHours == "" {
		return false
	}

	// Parse quiet hours (format: "HH:MM-HH:MM")
	var startHour, startMin, endHour, endMin int
	_, err := fmt.Sscanf(n.config.QuietHours, "%d:%d-%d:%d", &startHour, &startMin, &endHour, &endMin)
	if err != nil {
		return false
	}

	now := time.Now()
	currentMins := now.Hour()*60 + now.Minute()
	startMins := startHour*60 + startMin
	endMins := endHour*60 + endMin

	// Handle overnight quiet hours (e.g., 22:00-08:00)
	if startMins > endMins {
		return currentMins >= startMins || currentMins < endMins
	}

	return currentMins >= startMins && currentMins < endMins
}

func (n *Notifier) sendWebhook(event Event) error {
	payload := map[string]interface{}{
		"type":      event.Type,
		"timestamp": event.Timestamp,
		"market_id": event.MarketID,
		"message":   event.Message,
		"level":     event.Level,
		"data":      event.Data,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %v", err)
	}

	req, err := http.NewRequest("POST", n.config.WebhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	log.Printf("[NOTIFICATION] Webhook sent: %s", event.Type)
	return nil
}

func (n *Notifier) sendTelegram(event Event) error {
	// Format message with emoji based on level
	var emoji string
	switch event.Level {
	case LevelSuccess:
		emoji = "✅"
	case LevelWarning:
		emoji = "⚠️"
	case LevelError:
		emoji = "❌"
	default:
		emoji = "ℹ️"
	}

	text := fmt.Sprintf("%s *%s*\n\n%s", emoji, event.Type, event.Message)
	if event.MarketID != "" {
		text += fmt.Sprintf("\n\nMarket: `%s`", event.MarketID)
	}

	payload := map[string]interface{}{
		"chat_id":    n.config.TelegramChatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal telegram payload: %v", err)
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.config.TelegramToken)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create telegram request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("telegram returned status %d", resp.StatusCode)
	}

	log.Printf("[NOTIFICATION] Telegram sent: %s", event.Type)
	return nil
}

func (n *Notifier) logEvent(event Event) {
	var prefix string
	switch event.Level {
	case LevelSuccess:
		prefix = "✅"
	case LevelWarning:
		prefix = "⚠️"
	case LevelError:
		prefix = "❌"
	default:
		prefix = "ℹ️"
	}

	log.Printf("[NOTIFICATION] %s [%s] %s", prefix, event.Type, event.Message)
}

func (n *Notifier) saveEventsToFile() {
	jsonData, err := json.MarshalIndent(n.recentEvents, "", "  ")
	if err != nil {
		log.Printf("[NOTIFICATION] Failed to marshal events: %v", err)
		return
	}

	os.MkdirAll("data", 0755)
	if err := os.WriteFile("data/notifications.json", jsonData, 0644); err != nil {
		log.Printf("[NOTIFICATION] Failed to save events: %v", err)
	}
}

// Helper functions to create common events

// NewTradeExecutedEvent creates a trade executed event
func NewTradeExecutedEvent(marketID, direction string, size, price, confidence float64) Event {
	return Event{
		Type:     EventTradeExecuted,
		Level:    LevelInfo,
		MarketID: marketID,
		Message:  fmt.Sprintf("Executed %s trade: %.2f shares @ $%.4f (%.1f%% confidence)", direction, size, price, confidence*100),
		Data: map[string]interface{}{
			"direction":  direction,
			"size":       size,
			"price":      price,
			"confidence": confidence,
		},
	}
}

// NewTradeSettledEvent creates a trade settled event
func NewTradeSettledEvent(marketID string, pnl float64, won bool) Event {
	level := LevelSuccess
	if !won {
		level = LevelWarning
	}
	return Event{
		Type:     EventTradeSettled,
		Level:    level,
		MarketID: marketID,
		Message:  fmt.Sprintf("Trade settled: PnL $%.2f", pnl),
		Data: map[string]interface{}{
			"pnl": pnl,
			"won": won,
		},
	}
}

// NewClaimSuccessEvent creates a claim success event
func NewClaimSuccessEvent(marketID string, amount float64) Event {
	return Event{
		Type:     EventClaimSuccess,
		Level:    LevelSuccess,
		MarketID: marketID,
		Message:  fmt.Sprintf("Successfully claimed $%.2f from market", amount),
		Data: map[string]interface{}{
			"amount": amount,
		},
	}
}

// NewClaimFailedEvent creates a claim failed event
func NewClaimFailedEvent(marketID, reason string) Event {
	return Event{
		Type:     EventClaimFailed,
		Level:    LevelError,
		MarketID: marketID,
		Message:  fmt.Sprintf("Claim failed: %s", reason),
		Data: map[string]interface{}{
			"reason": reason,
		},
	}
}

// NewRiskWarningEvent creates a risk warning event
func NewRiskWarningEvent(reason string, data map[string]interface{}) Event {
	return Event{
		Type:    EventRiskWarning,
		Level:   LevelWarning,
		Message: reason,
		Data:    data,
	}
}

// NewCooldownEvent creates a cooldown event
func NewCooldownEvent(duration time.Duration, reason string) Event {
	return Event{
		Type:    EventCooldownEntered,
		Level:   LevelWarning,
		Message: fmt.Sprintf("Entering cooldown for %v: %s", duration, reason),
		Data: map[string]interface{}{
			"duration": duration.String(),
			"reason":   reason,
		},
	}
}

// NewDailyLimitEvent creates a daily limit event
func NewDailyLimitEvent(limitType string, value interface{}) Event {
	return Event{
		Type:    EventDailyLimitHit,
		Level:   LevelWarning,
		Message: fmt.Sprintf("Daily %s limit reached: %v", limitType, value),
		Data: map[string]interface{}{
			"limit_type": limitType,
			"value":      value,
		},
	}
}

// NewSystemErrorEvent creates a system error event
func NewSystemErrorEvent(component, message string, err error) Event {
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	return Event{
		Type:    EventSystemError,
		Level:   LevelError,
		Message: fmt.Sprintf("[%s] %s: %s", component, message, errMsg),
		Data: map[string]interface{}{
			"component": component,
			"error":     errMsg,
		},
	}
}
