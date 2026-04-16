package cost

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

type Provider string

const (
	ProviderOpenAI     Provider = "openai"
	ProviderAnthropic  Provider = "anthropic"
	ProviderPolymarket Provider = "polymarket"
	ProviderChainlink  Provider = "chainlink"
)

type UsageRecord struct {
	Timestamp    time.Time `json:"timestamp"`
	Provider     Provider  `json:"provider"`
	Model        string    `json:"model,omitempty"`
	Operation    string    `json:"operation"`
	InputTokens  int       `json:"input_tokens,omitempty"`
	OutputTokens int       `json:"output_tokens,omitempty"`
	Cost         float64   `json:"cost"`
	Success      bool      `json:"success"`
}

type DailyUsage struct {
	Date         string               `json:"date"`
	TotalCost    float64              `json:"total_cost"`
	RequestCount int                  `json:"request_count"`
	ByProvider   map[Provider]float64 `json:"by_provider"`
}

type Stats struct {
	TotalCost       float64                    `json:"total_cost"`
	TodayCost       float64                    `json:"today_cost"`
	MonthlyCost     float64                    `json:"monthly_cost"`
	TotalRequests   int                        `json:"total_requests"`
	TodayRequests   int                        `json:"today_requests"`
	MonthlyRequests int                        `json:"monthly_requests"`
	ByProvider      map[Provider]ProviderStats `json:"by_provider"`
}

type ProviderStats struct {
	TotalCost     float64 `json:"total_cost"`
	RequestCount  int     `json:"request_count"`
	AvgCostPerReq float64 `json:"avg_cost_per_req"`
}

type Tracker struct {
	mu          sync.RWMutex
	records     []UsageRecord
	dailyUsage  map[string]*DailyUsage
	startTime   time.Time
	lastUpdated time.Time
	savePending bool
}

func NewTracker() *Tracker {
	t := &Tracker{
		records:    make([]UsageRecord, 0, 10000),
		dailyUsage: make(map[string]*DailyUsage),
		startTime:  time.Now(),
	}
	t.Load()
	return t
}

func (t *Tracker) Record(provider Provider, operation string, cost float64, success bool) {
	t.RecordDetailed(provider, "", operation, 0, 0, cost, success)
}

func (t *Tracker) RecordDetailed(provider Provider, model, operation string, inputTokens, outputTokens int, cost float64, success bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	record := UsageRecord{
		Timestamp:    time.Now(),
		Provider:     provider,
		Model:        model,
		Operation:    operation,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Cost:         cost,
		Success:      success,
	}

	t.records = append(t.records, record)
	t.lastUpdated = time.Now()

	date := record.Timestamp.Format("2006-01-02")
	if _, exists := t.dailyUsage[date]; !exists {
		t.dailyUsage[date] = &DailyUsage{
			Date:       date,
			ByProvider: make(map[Provider]float64),
		}
	}
	daily := t.dailyUsage[date]
	daily.TotalCost += cost
	daily.RequestCount++
	daily.ByProvider[provider] += cost

	log.Printf("[COST] %s %s: $%.6f (success=%v)", provider, operation, cost, success)

	if len(t.records)%100 == 0 && !t.savePending {
		t.savePending = true
		go func() {
			t.Save()
			t.mu.Lock()
			t.savePending = false
			t.mu.Unlock()
		}()
	}
}

func (t *Tracker) GetStats() Stats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	stats := Stats{
		ByProvider: make(map[Provider]ProviderStats),
	}

	now := time.Now()
	today := now.Format("2006-01-02")
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	providerStats := make(map[Provider]*ProviderStats)

	for _, r := range t.records {
		stats.TotalCost += r.Cost
		stats.TotalRequests++

		if r.Timestamp.Format("2006-01-02") == today {
			stats.TodayCost += r.Cost
			stats.TodayRequests++
		}

		if r.Timestamp.After(monthStart) {
			stats.MonthlyCost += r.Cost
			stats.MonthlyRequests++
		}

		if _, exists := providerStats[r.Provider]; !exists {
			providerStats[r.Provider] = &ProviderStats{}
		}
		ps := providerStats[r.Provider]
		ps.TotalCost += r.Cost
		ps.RequestCount++
	}

	for p, ps := range providerStats {
		avg := 0.0
		if ps.RequestCount > 0 {
			avg = ps.TotalCost / float64(ps.RequestCount)
		}
		stats.ByProvider[p] = ProviderStats{
			TotalCost:     ps.TotalCost,
			RequestCount:  ps.RequestCount,
			AvgCostPerReq: avg,
		}
	}

	return stats
}

func (t *Tracker) GetDailyUsage(days int) []DailyUsage {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]DailyUsage, 0)
	now := time.Now()

	for i := 0; i < days; i++ {
		date := now.AddDate(0, 0, -i).Format("2006-01-02")
		if daily, exists := t.dailyUsage[date]; exists {
			result = append(result, *daily)
		}
	}

	return result
}

func (t *Tracker) Save() error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	data := struct {
		Records    []UsageRecord          `json:"records"`
		DailyUsage map[string]*DailyUsage `json:"daily_usage"`
		StartTime  time.Time              `json:"start_time"`
	}{
		Records:    t.records,
		DailyUsage: t.dailyUsage,
		StartTime:  t.startTime,
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cost data: %v", err)
	}

	os.MkdirAll("data", 0755)
	if err := os.WriteFile("data/costs.json", jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write cost file: %v", err)
	}

	log.Printf("[COST] Saved cost data to disk")
	return nil
}

func (t *Tracker) Load() error {
	data, err := os.ReadFile("data/costs.json")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read cost file: %v", err)
	}

	var saved struct {
		Records    []UsageRecord          `json:"records"`
		DailyUsage map[string]*DailyUsage `json:"daily_usage"`
		StartTime  time.Time              `json:"start_time"`
	}

	if err := json.Unmarshal(data, &saved); err != nil {
		return fmt.Errorf("failed to unmarshal cost data: %v", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	t.records = saved.Records
	t.dailyUsage = saved.DailyUsage
	t.startTime = saved.StartTime

	if t.records == nil {
		t.records = make([]UsageRecord, 0)
	}
	if t.dailyUsage == nil {
		t.dailyUsage = make(map[string]*DailyUsage)
	}

	log.Printf("[COST] Loaded %d historical records from disk", len(t.records))
	return nil
}
