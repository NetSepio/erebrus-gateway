package nftgate

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// Contract is a chain-specific gating collection address loaded from the database.
type Contract struct {
	Chain   string
	Address string
}

type multiChecker struct {
	checkers []Checker
}

func (m *multiChecker) Enabled() bool { return len(m.checkers) > 0 }

func (m *multiChecker) Owns(ctx context.Context, wallet string) (bool, error) {
	for _, c := range m.checkers {
		owns, err := c.Owns(ctx, wallet)
		if err != nil {
			return false, err
		}
		if owns {
			return true, nil
		}
	}
	return false, nil
}

// NewFromContracts builds a checker that accepts ownership of any listed contract.
// RPC URLs are per-chain; omit a chain RPC to skip contracts on that chain.
func NewFromContracts(solanaRPC, evmRPC string, contracts []Contract) Checker {
	var checkers []Checker
	for _, c := range contracts {
		chain := normalizeChain(c.Chain)
		addr := strings.TrimSpace(c.Address)
		if addr == "" {
			continue
		}
		switch chain {
		case "solana":
			if solanaRPC == "" {
				continue
			}
			checkers = append(checkers, newSolanaCore(solanaRPC, addr))
		case "evm":
			if evmRPC == "" {
				continue
			}
			checkers = append(checkers, &evmChecker{
				rpcURL:   evmRPC,
				contract: addr,
				http:     defaultHTTPClient(),
			})
		}
	}
	if len(checkers) == 0 {
		return disabled{}
	}
	if len(checkers) == 1 {
		return checkers[0]
	}
	return &multiChecker{checkers: checkers}
}

func normalizeChain(chain string) string {
	switch strings.ToLower(strings.TrimSpace(chain)) {
	case "sol":
		return "solana"
	default:
		return strings.ToLower(strings.TrimSpace(chain))
	}
}

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 8 * time.Second}
}
