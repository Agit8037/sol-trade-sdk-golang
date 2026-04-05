# Sol Trade SDK Go Examples

This directory contains examples demonstrating how to use the Sol Trade SDK for Go.

## Examples Summary

| Description | Directory | Run Command |
|-------------|-----------|-------------|
| Create and configure TradingClient instance | [trading_client](trading_client/main.go) | `go run ./examples/trading_client` |
| Share infrastructure across multiple wallets | [shared_infrastructure](shared_infrastructure/main.go) | `go run ./examples/shared_infrastructure` |
| PumpFun token sniping trading | [pumpfun_sniper_trading](pumpfun_sniper_trading/main.go) | `go run ./examples/pumpfun_sniper_trading` |
| Gas fee strategy example | [gas_fee_strategy](gas_fee_strategy/main.go) | `go run ./examples/gas_fee_strategy` |

## Environment Setup

Set the following environment variables before running examples:

```bash
export RPC_URL="https://api.mainnet-beta.solana.com"
# Or use Helius for better performance:
# export RPC_URL="https://mainnet.helius-rpc.com/?api-key=your_api_key"
```

## Quick Start

1. Install the SDK:
```bash
go get github.com/0xfnzero/sol-trade-sdk-golang
```

2. Configure your keypair and settings

3. Run an example:
```bash
go run ./examples/trading_client
```

## Important Notes

- Replace placeholder keypairs with your actual keypairs
- Configure SWQoS services with your API tokens for better transaction landing
- Test thoroughly before using on mainnet
- Monitor balances and transaction fees
