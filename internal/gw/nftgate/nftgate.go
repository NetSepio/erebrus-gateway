// Package nftgate checks whether a wallet owns the gating NFT collection, used
// as a (free) entitlement source alongside the trial. v2.0 supports EVM
// ERC-721 collections via a JSON-RPC balanceOf call; other chains fall back to
// "disabled" until implemented.
package nftgate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"
)

// Checker reports NFT ownership for entitlement.
type Checker interface {
	// Enabled reports whether NFT gating is configured.
	Enabled() bool
	// Owns reports whether the wallet holds at least one gating NFT.
	Owns(ctx context.Context, wallet string) (bool, error)
}

// New builds a Checker from config. Returns a disabled checker when the
// collection/RPC are unset or the chain is unsupported.
func New(chain, rpcURL, contract string) Checker {
	if contract == "" || rpcURL == "" {
		return disabled{}
	}
	switch chain {
	case "evm":
		return &evmChecker{rpcURL: rpcURL, contract: contract, http: &http.Client{Timeout: 8 * time.Second}}
	default:
		return disabled{}
	}
}

type disabled struct{}

func (disabled) Enabled() bool                              { return false }
func (disabled) Owns(context.Context, string) (bool, error) { return false, nil }

type evmChecker struct {
	rpcURL   string
	contract string
	http     *http.Client
}

func (e *evmChecker) Enabled() bool { return true }

// Owns calls ERC-721 balanceOf(address) and reports balance > 0.
func (e *evmChecker) Owns(ctx context.Context, wallet string) (bool, error) {
	addr := strings.TrimPrefix(strings.ToLower(wallet), "0x")
	if len(addr) != 40 {
		return false, fmt.Errorf("not an EVM address: %s", wallet)
	}
	// balanceOf(address) selector 0x70a08231 + 32-byte left-padded address.
	data := "0x70a08231" + strings.Repeat("0", 24) + addr

	reqBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "eth_call",
		"params": []any{map[string]string{"to": e.contract, "data": data}, "latest"},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.rpcURL, bytes.NewReader(reqBody))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var out struct {
		Result string `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false, err
	}
	if out.Error != nil {
		return false, fmt.Errorf("rpc error: %s", out.Error.Message)
	}
	bal, ok := new(big.Int).SetString(strings.TrimPrefix(out.Result, "0x"), 16)
	if !ok {
		return false, nil
	}
	return bal.Sign() > 0, nil
}
