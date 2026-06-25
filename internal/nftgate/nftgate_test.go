package nftgate

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDisabledWhenUnconfigured(t *testing.T) {
	c := New("solana", "", "") // no rpc/contract
	if c.Enabled() {
		t.Fatal("expected disabled checker")
	}
	owns, err := c.Owns(context.Background(), "anyone")
	if err != nil || owns {
		t.Fatalf("disabled checker should report (false, nil), got (%v, %v)", owns, err)
	}

	if New("dogechain", "rpc", "contract").Enabled() {
		t.Fatal("unsupported chain should be disabled")
	}
}

func TestSolanaCoreOwns(t *testing.T) {
	const collection = "CoLLeCt1onAddreSs11111111111111111111111111"
	const wallet = "WaLLeT1111111111111111111111111111111111111"

	var gotParams map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		_ = json.Unmarshal(body, &req)
		if req.Method != "searchAssets" {
			t.Errorf("method = %q, want searchAssets", req.Method)
		}
		gotParams = req.Params
		// Owner holds one asset in the collection.
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"x","result":{"total":1,"items":[{"id":"asset1"}]}}`))
	}))
	defer srv.Close()

	c := New("solana", srv.URL, collection)
	if !c.Enabled() {
		t.Fatal("expected enabled checker")
	}
	owns, err := c.Owns(context.Background(), wallet)
	if err != nil {
		t.Fatalf("owns: %v", err)
	}
	if !owns {
		t.Fatal("expected ownership = true")
	}
	// Verify the DAS request was scoped correctly.
	if gotParams["ownerAddress"] != wallet {
		t.Errorf("ownerAddress = %v", gotParams["ownerAddress"])
	}
	if gotParams["interface"] != "MplCoreAsset" {
		t.Errorf("interface = %v, want MplCoreAsset", gotParams["interface"])
	}
	grp, _ := gotParams["grouping"].([]any)
	if len(grp) != 2 || grp[0] != "collection" || grp[1] != collection {
		t.Errorf("grouping = %v", gotParams["grouping"])
	}
}

func TestSolanaCoreNotOwned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"x","result":{"total":0,"items":[]}}`))
	}))
	defer srv.Close()

	owns, err := New("sol", srv.URL, "col").Owns(context.Background(), "wallet")
	if err != nil {
		t.Fatalf("owns: %v", err)
	}
	if owns {
		t.Fatal("expected ownership = false")
	}
}

func TestSolanaCoreRPCError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":"x","error":{"code":-32000,"message":"bad request"}}`))
	}))
	defer srv.Close()

	_, err := New("solana", srv.URL, "col").Owns(context.Background(), "wallet")
	if err == nil || !strings.Contains(err.Error(), "DAS error") {
		t.Fatalf("expected DAS error, got %v", err)
	}
}
