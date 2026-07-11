// Package dropclient is the gateway's HTTP client for a node's private Drop
// (Kubo-backed) surface. Unlike internal/nodeclient — which sets a 10-second
// whole-request timeout and buffers the whole body — this client is built for
// streaming large objects:
//
//   - no http.Client.Timeout (that would cap the entire transfer); instead it
//     bounds only connection setup and the wait for response headers, and relies
//     on the caller's context for overall cancellation;
//   - request/response bodies are streamed via io.Reader/io.Writer, never fully
//     buffered in memory;
//   - transfers are byte-limited to protect the gateway from oversized bodies;
//   - the caller supplies an exact-purpose gateway-call token per operation.
package dropclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// ErrByteLimitExceeded is returned when a stream exceeds its byte limit.
var ErrByteLimitExceeded = errors.New("drop transfer exceeded byte limit")

// Gateway-call token purposes for node-private Drop calls. These name the exact
// operation the token authorizes so the node can enforce least privilege.
const (
	PurposeStatus   = "drop_status"
	PurposeUpload   = "drop_upload"
	PurposeRead     = "drop_read"
	PurposePinCheck = "drop_pin_check"
	PurposeUnpin    = "drop_unpin"
	PurposeWebUI    = "drop_webui"
)

// Client streams to/from a node's private Drop API.
type Client struct {
	http *http.Client
}

// New builds a streaming Drop client. It deliberately leaves http.Client.Timeout
// unset so long uploads/downloads are not truncated; setup and header waits are
// bounded, and overall lifetime is governed by the request context.
func New() *Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	return &Client{http: &http.Client{Transport: tr}}
}

// StatusResult is the node's Drop status snapshot.
type StatusResult struct {
	State           string `json:"state"`
	KuboVersion     string `json:"kubo_version"`
	RepoSizeBytes   int64  `json:"repo_size_bytes"`
	StorageMaxBytes int64  `json:"storage_max_bytes"`
	NumObjects      int64  `json:"num_objects"`
}

// UploadResult is the node's response after ingesting upload content.
type UploadResult struct {
	CID       string `json:"cid"`
	SizeBytes int64  `json:"size_bytes"`
}

// Status fetches the node's live Drop status (small JSON body).
func (c *Client) Status(ctx context.Context, baseURL, token, nodeKey string) (*StatusResult, error) {
	resp, err := c.do(ctx, http.MethodGet, baseURL, "/api/v2/drop/status", token, nodeKey, nil)
	if err != nil {
		return nil, err
	}
	defer drain(resp)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, statusError(resp)
	}
	var out StatusResult
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode drop status: %w", err)
	}
	return &out, nil
}

// PutUploadContent streams the upload body to the node and returns the resulting
// CID. The body is read straight from src (no buffering) and capped at maxBytes.
func (c *Client) PutUploadContent(ctx context.Context, baseURL, token, nodeKey, uploadID, sha256 string, src io.Reader, size, maxBytes int64) (*UploadResult, error) {
	limited := &limitReader{r: src, remaining: maxBytes}
	headers := http.Header{}
	headers.Set("X-Erebrus-Declared-Size", fmt.Sprintf("%d", size))
	if sha256 != "" {
		headers.Set("X-Erebrus-SHA256", sha256)
	}
	resp, err := c.doStream(ctx, http.MethodPut, baseURL, "/api/v2/drop/uploads/"+uploadID, token, nodeKey, limited, size, headers)
	if err != nil {
		if errors.Is(limited.err, ErrByteLimitExceeded) {
			return nil, ErrByteLimitExceeded
		}
		return nil, err
	}
	defer drain(resp)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, statusError(resp)
	}
	var out UploadResult
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode upload result: %w", err)
	}
	return &out, nil
}

// GetObject opens a streaming read of a CID's content from the node. The caller
// owns the returned ReadCloser and must close it. The returned reader is capped
// at maxBytes. contentType is echoed from the node when present.
func (c *Client) GetObject(ctx context.Context, baseURL, token, nodeKey, cid string, maxBytes int64) (rc io.ReadCloser, contentType string, err error) {
	resp, err := c.do(ctx, http.MethodGet, baseURL, "/api/v2/drop/objects/"+cid, token, nodeKey, nil)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode == http.StatusNotFound {
		drain(resp)
		return nil, "", errNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		e := statusError(resp)
		drain(resp)
		return nil, "", e
	}
	return &limitReadCloser{rc: resp.Body, remaining: maxBytes}, resp.Header.Get("Content-Type"), nil
}

// PinCheck reports whether a CID is pinned on the node.
func (c *Client) PinCheck(ctx context.Context, baseURL, token, nodeKey, cid string) (bool, error) {
	resp, err := c.do(ctx, http.MethodGet, baseURL, "/api/v2/drop/pins/"+cid, token, nodeKey, nil)
	if err != nil {
		return false, err
	}
	defer drain(resp)
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, statusError(resp)
	}
	return true, nil
}

// Unpin removes a CID pin on the node. A 404 is treated as success (idempotent).
func (c *Client) Unpin(ctx context.Context, baseURL, token, nodeKey, cid string) error {
	resp, err := c.do(ctx, http.MethodDelete, baseURL, "/api/v2/drop/pins/"+cid, token, nodeKey, nil)
	if err != nil {
		return err
	}
	defer drain(resp)
	if resp.StatusCode == http.StatusNotFound || (resp.StatusCode >= 200 && resp.StatusCode < 300) {
		return nil
	}
	return statusError(resp)
}

var errNotFound = errors.New("drop object not found")

// ErrNotFound reports whether err is the client's not-found sentinel.
func ErrNotFound(err error) bool { return errors.Is(err, errNotFound) }

func (c *Client) do(ctx context.Context, method, baseURL, path, token, nodeKey string, body io.Reader) (*http.Response, error) {
	return c.doStream(ctx, method, baseURL, path, token, nodeKey, body, -1, nil)
}

func (c *Client) doStream(ctx context.Context, method, baseURL, path, token, nodeKey string, body io.Reader, size int64, headers http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(baseURL, "/")+path, body)
	if err != nil {
		return nil, err
	}
	if size >= 0 {
		req.ContentLength = size
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if nodeKey != "" {
		req.Header.Set("X-Erebrus-Node-Key", nodeKey)
	}
	if body != nil && method != http.MethodGet {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	return c.http.Do(req)
}

func statusError(resp *http.Response) error {
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("node returned %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
}

func drain(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	_ = resp.Body.Close()
}

// limitReader caps the number of bytes read from an upstream reader, recording
// an error (never partially forwarding beyond the cap) when the limit is hit.
type limitReader struct {
	r         io.Reader
	remaining int64
	err       error
}

func (l *limitReader) Read(p []byte) (int, error) {
	if l.remaining < 0 {
		n, err := l.r.Read(p)
		return n, err
	}
	if l.remaining == 0 {
		// Peek one byte to detect overflow.
		var one [1]byte
		if n, _ := l.r.Read(one[:]); n > 0 {
			l.err = ErrByteLimitExceeded
			return 0, ErrByteLimitExceeded
		}
		return 0, io.EOF
	}
	if int64(len(p)) > l.remaining {
		p = p[:l.remaining]
	}
	n, err := l.r.Read(p)
	l.remaining -= int64(n)
	return n, err
}

// limitReadCloser wraps a response body, capping bytes forwarded to the caller.
type limitReadCloser struct {
	rc        io.ReadCloser
	remaining int64
}

func (l *limitReadCloser) Read(p []byte) (int, error) {
	if l.remaining == 0 {
		var one [1]byte
		n, err := l.rc.Read(one[:])
		if n > 0 {
			return 0, ErrByteLimitExceeded
		}
		return 0, err
	}
	if int64(len(p)) > l.remaining {
		p = p[:l.remaining]
	}
	n, err := l.rc.Read(p)
	l.remaining -= int64(n)
	return n, err
}

func (l *limitReadCloser) Close() error { return l.rc.Close() }
