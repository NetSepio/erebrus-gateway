package nftgate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// solanaCoreChecker verifies ownership of a Metaplex Core (mpl-core) asset from
// a given collection via the Digital Asset Standard (DAS) `searchAssets` RPC.
// The RPC endpoint must be DAS-capable (e.g. Helius/Triton). It asks "does this
// owner hold any asset grouped under this collection?" in a single call.
type solanaCoreChecker struct {
	rpcURL     string
	collection string
	http       *http.Client
}

func newSolanaCore(rpcURL, collection string) *solanaCoreChecker {
	return &solanaCoreChecker{rpcURL: rpcURL, collection: collection, http: &http.Client{Timeout: 10 * time.Second}}
}

func (s *solanaCoreChecker) Enabled() bool { return true }

// Owns reports whether wallet holds ≥1 Metaplex Core asset in the collection.
func (s *solanaCoreChecker) Owns(ctx context.Context, wallet string) (bool, error) {
	// DAS searchAssets: filter by owner + collection grouping. interface
	// "MplCoreAsset" scopes the search to Metaplex Core (excludes legacy
	// Token-Metadata NFTs that may share a grouping value).
	reqBody, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      "erebrus-nftgate",
		"method":  "searchAssets",
		"params": map[string]any{
			"ownerAddress": wallet,
			"grouping":     []any{"collection", s.collection},
			"interface":    "MplCoreAsset",
			"page":         1,
			"limit":        1,
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.rpcURL, bytes.NewReader(reqBody))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var out struct {
		Result struct {
			Total int               `json:"total"`
			Items []json.RawMessage `json:"items"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false, err
	}
	if out.Error != nil {
		return false, fmt.Errorf("DAS error: %s", out.Error.Message)
	}
	return out.Result.Total > 0 || len(out.Result.Items) > 0, nil
}
