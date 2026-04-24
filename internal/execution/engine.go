package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

// getPythonCmd returns the Python executable based on OS
func getPythonCmd() string {
	if runtime.GOOS == "windows" {
		return "py"
	}
	return "python3"
}

// L2Credentials represents the Polymarket L2 API keys
type L2Credentials struct {
	APIKey        string
	APISecret     string
	Passphrase    string
	PrivateKey    string
	SignatureType string // 0=EOA, 1=Proxy, 2=Gnosis Safe
	FunderAddress string // Required for Proxy/Gnosis Safe
}

// Engine handles order execution on Polymarket
type Engine struct {
	creds         *L2Credentials
	pendingOrders map[string]*BatchOrder
	orderHistory  []ExecutionResult
	mu            sync.RWMutex
	config        ExecutionConfig
	execSem       chan struct{} // semaphore to limit concurrent Python processes
}

// ExecutionConfig holds execution configuration
type ExecutionConfig struct {
	MaxRetries        int
	RetryDelay        time.Duration
	OrderTimeout      time.Duration
	BatchTimeout      time.Duration
	MaxBatchSize      int
	SlippageTolerance float64
}

// DefaultExecutionConfig returns default execution configuration
func DefaultExecutionConfig() ExecutionConfig {
	return ExecutionConfig{
		MaxRetries:        3,
		RetryDelay:        500 * time.Millisecond,
		OrderTimeout:      10 * time.Second,
		BatchTimeout:      30 * time.Second,
		MaxBatchSize:      10,
		SlippageTolerance: 0.02,
	}
}

// BatchOrder represents a batch of orders to execute atomically
type BatchOrder struct {
	ID        string
	Orders    []Order
	MarketID  string
	CreatedAt time.Time
	Status    string // "pending", "executing", "completed", "failed"
	Result    *ExecutionResult
}

// ExecutionResult represents the result of order execution
type ExecutionResult struct {
	BatchID       string
	Orders        []OrderResult
	TotalCost     float64
	TotalFilled   float64
	Success       bool
	ErrorMessage  string
	ExecutionTime time.Duration
	Timestamp     time.Time
}

// OrderResult represents the result of a single order
type OrderResult struct {
	OrderID    string
	TokenID    string
	Side       string
	Price      float64
	Size       float64
	FilledSize float64
	AvgPrice   float64
	Status     string // "filled", "partial", "rejected", "cancelled"
	Error      string
}

// Order represents an order structure
type Order struct {
	TokenID   string  `json:"tokenID"`
	Price     float64 `json:"price"`
	Size      float64 `json:"size"`
	Side      string  `json:"side"`
	OrderType string  `json:"type"`
}

// BalanceCheckRequest represents a balance check request
type BalanceCheckRequest struct {
	Action   string   `json:"action"`
	TokenIDs []string `json:"token_ids"`
}

// BalanceCheckResponse represents a balance check response
type BalanceCheckResponse struct {
	Success  bool               `json:"success"`
	Balances map[string]float64 `json:"balances"`
	Error    string             `json:"error,omitempty"`
}

// NewEngine initializes the execution engine
func NewEngine(creds *L2Credentials) *Engine {
	return &Engine{
		creds:         creds,
		pendingOrders: make(map[string]*BatchOrder),
		orderHistory:  make([]ExecutionResult, 0),
		config:        DefaultExecutionConfig(),
		execSem:       make(chan struct{}, 2), // max 2 concurrent Python processes
	}
}

// NewEngineWithConfig initializes the execution engine with custom config
func NewEngineWithConfig(creds *L2Credentials, config ExecutionConfig) *Engine {
	return &Engine{
		creds:         creds,
		pendingOrders: make(map[string]*BatchOrder),
		orderHistory:  make([]ExecutionResult, 0),
		config:        config,
		execSem:       make(chan struct{}, 2),
	}
}

// ExecuteArbitrage sends the orders to the Python script for signing and execution
func (e *Engine) ExecuteArbitrage(market string, orders []Order) error {
	log.Printf("[EXECUTION] Attempting to execute arbitrage for: %s", market)

	if e.creds == nil {
		log.Println("[EXECUTION] Simulation Mode: No credentials provided.")
		for _, o := range orders {
			log.Printf("[SIMULATION] -> %s %.2f shares of %s at $%.4f", o.Side, o.Size, o.TokenID, o.Price)
		}
		return nil
	}

	return e.executeBatch(market, orders)
}

// ExecuteBatch executes a batch of orders atomically
func (e *Engine) ExecuteBatch(batchID string, orders []Order) (*ExecutionResult, error) {
	batch := &BatchOrder{
		ID:        batchID,
		Orders:    orders,
		CreatedAt: time.Now(),
		Status:    "pending",
	}

	e.mu.Lock()
	e.pendingOrders[batchID] = batch
	e.mu.Unlock()

	result, err := e.executeBatchInternal(batch)

	e.mu.Lock()
	batch.Result = result
	batch.Status = "completed"
	if err != nil {
		batch.Status = "failed"
	}
	delete(e.pendingOrders, batchID)
	e.orderHistory = append(e.orderHistory, *result)
	// Cap history to prevent unbounded memory growth
	if len(e.orderHistory) > 1000 {
		e.orderHistory = e.orderHistory[len(e.orderHistory)-1000:]
	}
	e.mu.Unlock()

	return result, err
}

// executeBatch executes a batch of orders
func (e *Engine) executeBatch(market string, orders []Order) error {
	batchID := fmt.Sprintf("batch_%d", time.Now().UnixNano())
	_, err := e.ExecuteBatch(batchID, orders)
	return err
}

// executeBatchInternal handles the actual execution
func (e *Engine) executeBatchInternal(batch *BatchOrder) (*ExecutionResult, error) {
	startTime := time.Now()
	result := &ExecutionResult{
		BatchID:   batch.ID,
		Orders:    make([]OrderResult, len(batch.Orders)),
		Timestamp: time.Now(),
	}

	// Check if credentials are nil (simulation mode)
	if e.creds == nil {
		log.Printf("[EXECUTION] Simulation mode: batch %s would execute %d orders", batch.ID, len(batch.Orders))
		for i, order := range batch.Orders {
			result.Orders[i] = OrderResult{
				TokenID:    order.TokenID,
				Side:       order.Side,
				Price:      order.Price,
				Size:       order.Size,
				FilledSize: order.Size,
				AvgPrice:   order.Price,
				Status:     "simulated",
			}
		}
		result.Success = true
		result.TotalFilled = float64(len(batch.Orders))
		result.ExecutionTime = time.Since(startTime)
		return result, nil
	}

	ordersJSON, err := json.Marshal(batch.Orders)
	if err != nil {
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("failed to marshal orders: %v", err)
		return result, err
	}

	// Acquire semaphore to limit concurrent Python processes
	e.execSem <- struct{}{}
	defer func() { <-e.execSem }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, getPythonCmd(), "scripts/executor.py")
	cmd.Stdin = bytes.NewReader(ordersJSON)
	cmd.Env = append(os.Environ(),
		"POLY_PRIVATE_KEY="+e.creds.PrivateKey,
		"POLY_API_KEY="+e.creds.APIKey,
		"POLY_API_SECRET="+e.creds.APISecret,
		"POLY_PASSPHRASE="+e.creds.Passphrase,
		"POLY_SIGNATURE_TYPE="+e.creds.SignatureType,
		"POLY_FUNDER_ADDRESS="+e.creds.FunderAddress,
	)

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	log.Printf("[EXECUTION] Calling Python executor for batch %s (%d orders)...", batch.ID, len(batch.Orders))
	err = cmd.Run()

	result.ExecutionTime = time.Since(startTime)

	// Parse JSON output even if exit code is non-zero
	// Python script outputs JSON before exiting with code 1 on failure
	if out.Len() > 0 {
		var pyResult struct {
			BatchID      string  `json:"batch_id"`
			Success      bool    `json:"success"`
			TotalCost    float64 `json:"total_cost"`
			TotalFilled  float64 `json:"total_filled"`
			ErrorMessage string  `json:"error_message"`
			Orders       []struct {
				TokenID    string  `json:"token_id"`
				Side       string  `json:"side"`
				Price      float64 `json:"price"`
				Size       float64 `json:"size"`
				FilledSize float64 `json:"filled_size"`
				AvgPrice   float64 `json:"avg_price"`
				Status     string  `json:"status"`
				Error      string  `json:"error"`
			} `json:"orders"`
		}
		if parseErr := json.Unmarshal(out.Bytes(), &pyResult); parseErr == nil {
			result.Success = pyResult.Success
			result.TotalCost = pyResult.TotalCost
			result.TotalFilled = pyResult.TotalFilled
			result.ErrorMessage = pyResult.ErrorMessage
			result.Orders = make([]OrderResult, len(pyResult.Orders))
			for i, o := range pyResult.Orders {
				result.Orders[i] = OrderResult{
					TokenID:    o.TokenID,
					Side:       o.Side,
					Price:      o.Price,
					Size:       o.Size,
					FilledSize: o.FilledSize,
					AvgPrice:   o.AvgPrice,
					Status:     o.Status,
					Error:      o.Error,
				}
			}
			if !pyResult.Success {
				log.Printf("[EXECUTION ERROR] Batch %s failed: %s", batch.ID, pyResult.ErrorMessage)
				return result, fmt.Errorf("%s", pyResult.ErrorMessage)
			}
			// Log Python debug output if any
			if stderrStr := stderr.String(); stderrStr != "" {
				log.Printf("[EXECUTION DEBUG] %s", stderrStr)
			}
			log.Printf("[EXECUTION SUCCESS] Batch %s completed in %v", batch.ID, result.ExecutionTime)
			return result, nil
		}
	}

	// Fallback to old behavior if JSON parsing fails
	if err != nil {
		log.Printf("[EXECUTION ERROR] Python script failed: %v", err)
		log.Printf("Stderr: %s", stderr.String())
		log.Printf("Stdout: %s", out.String())
		result.Success = false
		result.ErrorMessage = fmt.Sprintf("execution failed: %v, stderr: %s", err, stderr.String())
		return result, err
	}

	result.Success = true
	log.Printf("[EXECUTION SUCCESS] Batch %s completed in %v", batch.ID, result.ExecutionTime)
	log.Printf("Python Output: %s", out.String())

	return result, nil
}

// ExecuteWithRetry executes orders with exponential backoff retry logic
func (e *Engine) ExecuteWithRetry(market string, orders []Order) (*ExecutionResult, error) {
	var lastErr error
	delay := e.config.RetryDelay

	for i := 0; i < e.config.MaxRetries; i++ {
		batchID := fmt.Sprintf("retry_%d_%d", i, time.Now().UnixNano())
		result, err := e.ExecuteBatch(batchID, orders)

		if err == nil && result.Success {
			return result, nil
		}

		lastErr = err
		log.Printf("[EXECUTION] Retry %d/%d failed: %v", i+1, e.config.MaxRetries, err)

		if i < e.config.MaxRetries-1 {
			log.Printf("[EXECUTION] Backing off %v before next retry", delay)
			time.Sleep(delay)
			delay = delay * 2 // Exponential backoff: 500ms → 1s → 2s
			if delay > 5*time.Second {
				delay = 5 * time.Second // Cap at 5 seconds
			}
		}
	}

	return nil, fmt.Errorf("all %d retries exhausted: %v", e.config.MaxRetries, lastErr)
}

// GetPendingOrders returns currently pending orders
func (e *Engine) GetPendingOrders() map[string]*BatchOrder {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make(map[string]*BatchOrder)
	for k, v := range e.pendingOrders {
		// Return a shallow copy to prevent data races on Status/Result fields
		copied := *v
		result[k] = &copied
	}
	return result
}

// GetOrderHistory returns the order execution history
func (e *Engine) GetOrderHistory() []ExecutionResult {
	e.mu.RLock()
	defer e.mu.RUnlock()
	history := make([]ExecutionResult, len(e.orderHistory))
	copy(history, e.orderHistory)
	return history
}

// GetStats returns execution statistics
func (e *Engine) GetStats() ExecutionStats {
	e.mu.RLock()
	defer e.mu.RUnlock()

	stats := ExecutionStats{}
	for _, result := range e.orderHistory {
		if result.Success {
			stats.SuccessfulBatches++
			stats.TotalFilled += result.TotalFilled
			stats.TotalCost += result.TotalCost
		} else {
			stats.FailedBatches++
		}
	}
	stats.TotalBatches = len(e.orderHistory)
	stats.PendingBatches = len(e.pendingOrders)

	return stats
}

// ExecutionStats holds execution statistics
type ExecutionStats struct {
	TotalBatches      int
	SuccessfulBatches int
	FailedBatches     int
	PendingBatches    int
	TotalFilled       float64
	TotalCost         float64
}

// CancelBatch cancels a pending batch
func (e *Engine) CancelBatch(batchID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	batch, exists := e.pendingOrders[batchID]
	if !exists {
		return fmt.Errorf("batch %s not found", batchID)
	}

	if batch.Status == "executing" {
		return fmt.Errorf("cannot cancel executing batch")
	}

	batch.Status = "cancelled"
	delete(e.pendingOrders, batchID)

	return nil
}

// ClaimResult represents the result of a claim operation
type ClaimResult struct {
	Success      bool `json:"success"`
	TotalClaimed int  `json:"total_claimed"`
}

// ClaimPositions claims winnings from settled markets
func (e *Engine) ClaimPositions(conditionIDs []string) (*ClaimResult, error) {
	log.Printf("[EXECUTION] Claiming %d positions", len(conditionIDs))

	e.execSem <- struct{}{}
	defer func() { <-e.execSem }()

	if e.creds == nil {
		log.Println("[EXECUTION] Simulation Mode: No credentials provided.")
		return &ClaimResult{Success: true, TotalClaimed: 0}, nil
	}

	claimReq := map[string]interface{}{
		"action":        "claim",
		"condition_ids": conditionIDs,
	}

	claimJSON, err := json.Marshal(claimReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal claim request: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, getPythonCmd(), "scripts/executor.py")
	cmd.Stdin = bytes.NewReader(claimJSON)
	cmd.Env = append(os.Environ(),
		"POLY_PRIVATE_KEY="+e.creds.PrivateKey,
		"POLY_API_KEY="+e.creds.APIKey,
		"POLY_API_SECRET="+e.creds.APISecret,
		"POLY_PASSPHRASE="+e.creds.Passphrase,
		"POLY_SIGNATURE_TYPE="+e.creds.SignatureType,
		"POLY_FUNDER_ADDRESS="+e.creds.FunderAddress,
	)

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	log.Printf("[EXECUTION] Calling Python executor for claim...")
	err = cmd.Run()

	// Try parsing JSON output even if exit code is non-zero
	if out.Len() > 0 {
		var result ClaimResult
		if parseErr := json.Unmarshal(out.Bytes(), &result); parseErr == nil {
			if err != nil {
				log.Printf("[CLAIM] Python exited with error but produced parseable output")
			}
			return &result, nil
		}
	}

	if err != nil {
		log.Printf("[CLAIM ERROR] Python script failed: %v", err)
		log.Printf("Stderr: %s", stderr.String())
		return nil, fmt.Errorf("claim failed: %v", err)
	}

	log.Printf("[CLAIM SUCCESS] Python Output: %s", out.String())

	var result ClaimResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse claim result: %v", err)
	}

	return &result, nil
}

// AutoRedeem scans the user's wallet for settled positions and automatically redeems pUSD directly from CTF via Gnosis Safe or EOA.
func (e *Engine) AutoRedeem() (*ClaimResult, error) {
	e.execSem <- struct{}{}
	defer func() { <-e.execSem }()

	if e.creds == nil {
		log.Println("[EXECUTION] Simulation Mode: Cannot auto_redeem without credentials")
		return &ClaimResult{Success: true, TotalClaimed: 0}, nil
	}

	request := map[string]string{
		"action": "auto_redeem",
	}

	requestJSON, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal auto_redeem request: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second) // Could take longer because it iterates RPCs
	defer cancel()

	cmd := exec.CommandContext(ctx, getPythonCmd(), "scripts/executor.py")
	cmd.Stdin = bytes.NewReader(requestJSON)
	cmd.Env = append(os.Environ(),
		"POLY_PRIVATE_KEY="+e.creds.PrivateKey,
		"POLY_API_KEY="+e.creds.APIKey,
		"POLY_API_SECRET="+e.creds.APISecret,
		"POLY_PASSPHRASE="+e.creds.Passphrase,
		"POLY_SIGNATURE_TYPE="+e.creds.SignatureType,
		"POLY_FUNDER_ADDRESS="+e.creds.FunderAddress,
	)

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	log.Printf("[EXECUTION] Calling Python executor for auto_redeem...")
	err = cmd.Run()

	// Try parsing JSON output even if exit code is non-zero
	if out.Len() > 0 {
		var result ClaimResult
		if parseErr := json.Unmarshal(out.Bytes(), &result); parseErr == nil {
			if err != nil {
				log.Printf("[AUTO_REDEEM] Python exited with error but produced parseable output")
			}
			if result.TotalClaimed > 0 {
				log.Printf("[AUTO_REDEEM SUCCESS] Automatically redeemed %d settled positions directly to pUSD!", result.TotalClaimed)
			}
			return &result, nil
		}
	}

	if err != nil {
		log.Printf("[AUTO_REDEEM ERROR] Python script failed: %v", err)
		log.Printf("Stderr: %s", stderr.String())
		return nil, fmt.Errorf("auto_redeem failed: %v", err)
	}

	if out.Len() == 0 {
		log.Printf("[AUTO_REDEEM] Empty response")
		return nil, fmt.Errorf("auto_redeem returned empty response")
	}

	var result ClaimResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		log.Printf("[AUTO_REDEEM ERROR] Failed to parse result: %v | Raw: %s", err, out.String())
		return nil, fmt.Errorf("failed to parse auto_redeem result: %v", err)
	}

	if result.TotalClaimed > 0 {
		log.Printf("[AUTO_REDEEM SUCCESS] Automatically redeemed %d settled positions directly to pUSD!", result.TotalClaimed)
	}

	return &result, nil
}

// CancelAllOrders cancels all open orders on Polymarket CLOB
func (e *Engine) CancelAllOrders() error {
	e.execSem <- struct{}{}
	defer func() { <-e.execSem }()

	if e.creds == nil {
		return nil // Simulation mode
	}

	request := map[string]string{"action": "cancel_all"}
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal cancel request: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, getPythonCmd(), "scripts/executor.py")
	cmd.Stdin = bytes.NewReader(requestJSON)
	cmd.Env = append(os.Environ(),
		"POLY_PRIVATE_KEY="+e.creds.PrivateKey,
		"POLY_API_KEY="+e.creds.APIKey,
		"POLY_API_SECRET="+e.creds.APISecret,
		"POLY_PASSPHRASE="+e.creds.Passphrase,
		"POLY_SIGNATURE_TYPE="+e.creds.SignatureType,
		"POLY_FUNDER_ADDRESS="+e.creds.FunderAddress,
	)

	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Printf("[EXECUTION] Cancel all failed: %v, stderr: %s", err, stderr.String())
		return err
	}
	log.Printf("[EXECUTION] Cancelled all open orders")
	return nil
}

// CheckBalances queries the balance for multiple token IDs
func (e *Engine) CheckBalances(tokenIDs []string) (map[string]float64, error) {
	e.execSem <- struct{}{}
	defer func() { <-e.execSem }()

	if e.creds == nil {
		log.Println("[EXECUTION] Simulation Mode: Cannot check balances without credentials")
		return nil, fmt.Errorf("no credentials provided")
	}

	request := BalanceCheckRequest{
		Action:   "check_balances",
		TokenIDs: tokenIDs,
	}

	requestJSON, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal balance request: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, getPythonCmd(), "scripts/executor.py")
	cmd.Stdin = bytes.NewReader(requestJSON)
	cmd.Env = append(os.Environ(),
		"POLY_PRIVATE_KEY="+e.creds.PrivateKey,
		"POLY_API_KEY="+e.creds.APIKey,
		"POLY_API_SECRET="+e.creds.APISecret,
		"POLY_PASSPHRASE="+e.creds.Passphrase,
		"POLY_SIGNATURE_TYPE="+e.creds.SignatureType,
		"POLY_FUNDER_ADDRESS="+e.creds.FunderAddress,
	)

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	log.Printf("[EXECUTION] Checking balances for %d tokens...", len(tokenIDs))
	err = cmd.Run()

	// Try parsing JSON output even if exit code is non-zero
	if out.Len() > 0 {
		var response BalanceCheckResponse
		if parseErr := json.Unmarshal(out.Bytes(), &response); parseErr == nil && response.Success {
			if err != nil {
				log.Printf("[BALANCE] Python exited with error but produced parseable output")
			}
			return response.Balances, nil
		}
	}

	if err != nil {
		log.Printf("[BALANCE ERROR] Python script failed: %v", err)
		log.Printf("Stderr: %s", stderr.String())
		return nil, fmt.Errorf("balance check failed: %v", err)
	}

	var response BalanceCheckResponse
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		log.Printf("[BALANCE ERROR] Failed to parse response: %v", err)
		log.Printf("Output: %s", out.String())
		return nil, fmt.Errorf("failed to parse balance response: %v", err)
	}

	if !response.Success {
		return nil, fmt.Errorf("balance check failed: %s", response.Error)
	}

	return response.Balances, nil
}

// CheckCollateralBalance queries the available CLOB collateral balance (pUSD) of the current configured wallet.
func (e *Engine) CheckCollateralBalance() (float64, error) {
	e.execSem <- struct{}{}
	defer func() { <-e.execSem }()

	if e.creds == nil {
		return 0, fmt.Errorf("no credentials provided")
	}

	request := map[string]string{
		"action": "check_collateral_balance",
	}

	requestJSON, err := json.Marshal(request)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal request: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, getPythonCmd(), "scripts/executor.py")
	cmd.Stdin = bytes.NewReader(requestJSON)
	cmd.Env = append(os.Environ(),
		"POLY_PRIVATE_KEY="+e.creds.PrivateKey,
		"POLY_API_KEY="+e.creds.APIKey,
		"POLY_API_SECRET="+e.creds.APISecret,
		"POLY_PASSPHRASE="+e.creds.Passphrase,
		"POLY_SIGNATURE_TYPE="+e.creds.SignatureType,
		"POLY_FUNDER_ADDRESS="+e.creds.FunderAddress,
	)

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err = cmd.Run()

	type collateralResponse struct {
		Success bool    `json:"success"`
		Balance float64 `json:"balance"`
		Error   string  `json:"error,omitempty"`
	}

	// Try parsing JSON output even if exit code is non-zero
	if out.Len() > 0 {
		var response collateralResponse
		if parseErr := json.Unmarshal(out.Bytes(), &response); parseErr == nil && response.Success {
			if err != nil {
				log.Printf("[COLLATERAL BALANCE] Python exited with error but produced parseable output")
			}
			return response.Balance, nil
		}
	}

	if err != nil {
		log.Printf("[COLLATERAL BALANCE ERROR] Python script failed: %v", err)
		log.Printf("Stderr: %s", stderr.String())
		return 0, fmt.Errorf("collateral balance check failed: %v", err)
	}

	var response collateralResponse
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		return 0, fmt.Errorf("failed to parse collateral balance response: %v", err)
	}

	if !response.Success {
		return 0, fmt.Errorf("collateral balance check failed: %s", response.Error)
	}

	return response.Balance, nil
}

// CheckUSDCBalance is a backward-compatible alias retained for older callers.
func (e *Engine) CheckUSDCBalance() (float64, error) {
	return e.CheckCollateralBalance()
}
