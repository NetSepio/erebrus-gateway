// Package nodeclient is the gateway's HTTP client for a node's /api/v2 surface
// (peer provisioning + credential re-fetch). It mirrors the node's
// erebrus/internal/api request/response shapes.
package nodeclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client calls node REST APIs.
type Client struct {
	http *http.Client
}

// New builds a node client with sane timeouts.
func New() *Client {
	return &Client{http: &http.Client{Timeout: 10 * time.Second}}
}

// PeerRequest is the body of PUT /api/v2/peers/{id} on the node.
type PeerRequest struct {
	Name           string `json:"name"`
	Wallet         string `json:"wallet,omitempty"`
	WGPublicKey    string `json:"wg_public_key"`
	WGPresharedKey string `json:"wg_preshared_key,omitempty"`
	ExpiresAt      int64  `json:"expires_at,omitempty"`
}

// Bundle is the node's credential bundle. The gateway forwards it to the client
// verbatim; only the WireGuard address is read out to persist on the client row.
type Bundle struct {
	Raw       json.RawMessage
	WGAddress string
}

// UpsertPeer provisions (or updates) a peer on the node. Idempotent by id, so it
// retries on transport errors up to 2 extra times.
func (c *Client) UpsertPeer(ctx context.Context, baseURL, gatewayToken, nodeKey, peerID string, req PeerRequest) (*Bundle, error) {
	body, _ := json.Marshal(req)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		raw, status, err := c.do(ctx, http.MethodPut, baseURL, "/api/v2/peers/"+peerID, gatewayToken, nodeKey, body)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * 300 * time.Millisecond)
			continue
		}
		if status < 200 || status >= 300 {
			return nil, fmt.Errorf("node returned %d: %s", status, truncate(raw))
		}
		b := &Bundle{Raw: raw}
		var parsed struct {
			WireGuard struct {
				Address string `json:"address"`
			} `json:"wireguard"`
		}
		_ = json.Unmarshal(raw, &parsed)
		b.WGAddress = parsed.WireGuard.Address
		return b, nil
	}
	return nil, fmt.Errorf("node unreachable after retries: %w", lastErr)
}

// Credentials re-fetches a peer's bundle.
func (c *Client) Credentials(ctx context.Context, baseURL, gatewayToken, nodeKey, peerID string) (json.RawMessage, error) {
	raw, status, err := c.do(ctx, http.MethodGet, baseURL, "/api/v2/peers/"+peerID+"/credentials", gatewayToken, nodeKey, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("not found")
	}
	if status < 200 || status >= 300 {
		return nil, fmt.Errorf("node returned %d", status)
	}
	return raw, nil
}

// DeletePeer removes a peer on the node. Treats 404 as success (idempotent).
func (c *Client) DeletePeer(ctx context.Context, baseURL, gatewayToken, nodeKey, peerID string) error {
	_, status, err := c.do(ctx, http.MethodDelete, baseURL, "/api/v2/peers/"+peerID, gatewayToken, nodeKey, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound || (status >= 200 && status < 300) {
		return nil
	}
	return fmt.Errorf("node returned %d", status)
}

func (c *Client) do(ctx context.Context, method, baseURL, path, gatewayToken, nodeKey string, body []byte) (json.RawMessage, int, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(baseURL, "/")+path, rdr)
	if err != nil {
		return nil, 0, err
	}
	if gatewayToken != "" {
		req.Header.Set("Authorization", "Bearer "+gatewayToken)
	}
	if nodeKey != "" {
		req.Header.Set("X-Erebrus-Node-Key", nodeKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return raw, resp.StatusCode, nil
}

func truncate(b []byte) string {
	if len(b) > 200 {
		return string(b[:200])
	}
	return string(b)
}