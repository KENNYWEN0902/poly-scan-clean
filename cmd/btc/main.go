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

	restClient := polymarket.NewClient()

	var creds *execution.L2Credentials
	if os.Getenv("POLY_PRIVATE_KEY") != "" {
		creds = &execution.L2Credentials{
			APIKey:        os.Getenv("POLY_API_KEY"),
			APISecret:     os.Getenv("POLY_API_SECRET"),
			Passphrase:    os.Getenv("POLY_PASSPHRASE"),
			PrivateKey:    os.Getenv("POLY_PRIVATE_KEY"),
			SignatureType: os.Getenv("POLY_SIGNATURE_TYPE"),
			FunderAddress: os.Getenv("POLY_FUNDER_ADDRESS"),
		}
		log.Println("Running in LIVE Trading Mode")
	} else {
		log.Println("Running in SIMULATION Mode (No private key provided)")
	}
	execEngine := execution.NewEngine(creds)

	config := btc.DefaultBTCMarketConfig()
	strategy := btc.NewBTCStrategy(restClient, execEngine, config)

	if err := strategy.Start(); err != nil {
		log.Fatalf("Failed to start BTC strategy: %v", err)
	}

	log.Println("Strategy started. Press Ctrl+C to stop.")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")
	strategy.Stop()
	log.Println("Strategy stopped")
}
