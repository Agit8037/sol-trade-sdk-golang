// TradingClient Creation Example
//
// This example demonstrates two ways to create a TradingClient:
// 1. Simple method: NewTradeClient() - creates client with its own infrastructure
// 2. Shared method: NewTradeClientFromInfrastructure() - reuses existing infrastructure
//
// For multi-wallet scenarios, see the shared_infrastructure example.

package main

import (
	"context"
	"fmt"
	"log"
	"os"

	soltradesdk "github.com/0xfnzero/sol-trade-sdk-golang"
	"github.com/0xfnzero/sol-trade-sdk-golang/pkg/common"
	"github.com/0xfnzero/sol-trade-sdk-golang/pkg/trading"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

func main() {
	ctx := context.Background()

	// Method 1: Simple - NewTradeClient() (recommended for single wallet)
	client1, err := createTradingClientSimple(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Method 1: Created TradingClient with NewTradeClient()\n")
	fmt.Printf("  Wallet: %s\n", client1.PayerPubkey())

	// Method 2: From infrastructure (recommended for multiple wallets)
	client2, err := createTradingClientFromInfrastructure(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("\nMethod 2: Created TradingClient with FromInfrastructure()\n")
	fmt.Printf("  Wallet: %s\n", client2.PayerPubkey())
}

// Method 1: Create TradingClient using TradeConfig (simple, self-contained)
//
// Use this when you have a single wallet or don't need to share infrastructure.
func createTradingClientSimple(ctx context.Context) (*trading.TradeClient, error) {
	// Use your keypair here
	payer := solana.NewWallet()

	rpcURL := os.Getenv("RPC_URL")
	if rpcURL == "" {
		rpcURL = "https://api.mainnet-beta.solana.com"
	}

	commitment := rpc.CommitmentConfirmed

	swqosConfigs := []common.SwqosConfig{
		{Type: common.SwqosTypeDefault, URL: rpcURL},
		{Type: common.SwqosTypeJito, UUID: "your_uuid", Region: common.SwqosRegionFrankfurt},
		{Type: common.SwqosTypeBloxroute, APIToken: "your_api_token", Region: common.SwqosRegionFrankfurt},
		{Type: common.SwqosTypeZeroSlot, APIToken: "your_api_token", Region: common.SwqosRegionFrankfurt},
		{Type: common.SwqosTypeTemporal, APIToken: "your_api_token", Region: common.SwqosRegionFrankfurt},
	}

	tradeConfig := &common.TradeConfig{
		RPCURL:      rpcURL,
		SwqosConfigs: swqosConfigs,
		Commitment:  commitment,
	}

	// Creates new infrastructure internally
	client, err := trading.NewTradeClient(ctx, payer, tradeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create trade client: %w", err)
	}

	return client, nil
}

// Method 2: Create TradingClient from shared infrastructure
//
// Use this when you have multiple wallets sharing the same configuration.
// The infrastructure (RPC client, SWQOS clients) is created once and shared.
func createTradingClientFromInfrastructure(ctx context.Context) (*trading.TradeClient, error) {
	// Use your keypair here
	payer := solana.NewWallet()

	rpcURL := os.Getenv("RPC_URL")
	if rpcURL == "" {
		rpcURL = "https://api.mainnet-beta.solana.com"
	}

	commitment := rpc.CommitmentConfirmed

	swqosConfigs := []common.SwqosConfig{
		{Type: common.SwqosTypeDefault, URL: rpcURL},
		{Type: common.SwqosTypeJito, UUID: "your_uuid", Region: common.SwqosRegionFrankfurt},
	}

	// Create infrastructure separately (can be shared across multiple wallets)
	infraConfig := &common.InfrastructureConfig{
		RPCURL:      rpcURL,
		SwqosConfigs: swqosConfigs,
		Commitment:  commitment,
	}
	infrastructure, err := trading.NewInfrastructure(ctx, infraConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create infrastructure: %w", err)
	}

	// Create client from existing infrastructure (fast, no async needed)
	client := trading.NewTradeClientFromInfrastructure(payer, infrastructure, true)

	return client, nil
}
