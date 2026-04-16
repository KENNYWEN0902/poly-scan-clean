package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"poly-scan/internal/btc"
	"poly-scan/internal/execution"
	"poly-scan/internal/polymarket"
)

func main() {
	log.Println("Starting BTC 5-Minute Delay Arbitrage Strategy...")

	// Initialize API clients
	restClient := polymarket.NewClient()

	// Initialize Execution Engine
	var creds *execution.L2Credentials
	simMode := os.Getenv("POLY_SIMULATION") == "1" || os.Getenv("POLY_SIMULATION") == "true"
	if os.Getenv("POLY_PRIVATE_KEY") != "" {
		creds = &execution.L2Credentials{
			APIKey:        os.Getenv("POLY_API_KEY"),
			APISecret:     os.Getenv("POLY_API_SECRET"),
			Passphrase:    os.Getenv("POLY_PASSPHRASE"),
			PrivateKey:    os.Getenv("POLY_PRIVATE_KEY"),
			SignatureType: os.Getenv("POLY_SIGNATURE_TYPE"),
			FunderAddress: os.Getenv("POLY_FUNDER_ADDRESS"),
		}
		if simMode {
			log.Println("Running in SIMULATION Mode (orders will NOT be executed)")
		} else {
			log.Println("Running in LIVE Trading Mode")
		}
	} else {
		log.Println("Running in SIMULATION Mode (No private key provided)")
	}
	execEngine := execution.NewEngine(creds)

	// Initialize BTC Strategy
	config := btc.DefaultBTCMarketConfig()
	strategy := btc.NewBTCStrategy(restClient, execEngine, config)

	// Start strategy
	if err := strategy.Start(); err != nil {
		log.Fatalf("Failed to start BTC strategy: %v", err)
	}

	log.Println("Strategy started. Press Ctrl+C to stop.")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	strategy.Stop()
	log.Println("Strategy stopped")
}
