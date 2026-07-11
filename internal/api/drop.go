// Drop (Kubo storage) HTTP APIs: authenticated node discovery, upload
// reservation/content/status, file list/metadata/content/delete, usage, org
// metadata/usage, encrypted vault, opaque public share, and a short-lived
// same-origin WebUI proxy. Uploads/downloads stream through the gateway without
// buffering; raw Kubo RPC addresses and node credentials are never exposed to
// callers, and upload bodies are never logged.
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/NetSepio/gateway/internal/dropclient"
	"github.com/NetSepio/gateway/internal/metrics"
	"github.com/NetSepio/gateway/internal/store"
	"github.com/gin-gonic/gin"
)

// cidPattern is a conservative CIDv0/CIDv1 shape check applied before any node
// or Kubo call. It intentionally rejects path separators and other characters
// that could be used to escape the object path.
var cidPattern = regexp.MustCompile(`^[A-Za-z0-9]{20,120}$`)

func validCID(cid string) bool { return cidPattern.MatchString(cid) }

// dropRateLimit rate-limits Drop routes per authenticated user (falling back to
// client IP for unauthenticated callers). write selects the create/content
// budget; otherwise the read budget.
func (s *Server) dropRateLimit(scope string, write bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		p := s.platform.Snapshot()
		perMin := p.RateLimitDropReadPerMin
		if write {
			perMin = p.RateLimitDropWritePerMin
		}
		key := userID(c)
		if key == "" {
			key = c.ClientIP()
		}
		if perMin > 0 && !s.cache.Allow(c, "rl:"+scope+":"+key, perMin, time.Minute) {
			c.Header("Retry-After", "60")
			fail(c, http.StatusTooManyRequests, "rate limit exceeded; slow down")
			return
		}
		c.Next()
	}
}

// dropNodeEndpoint resolves a node's private Drop base URL + credential and mints
// a short-lived, exact-purpose gateway-call token. The returned values stay
// server-side; callers never see them.
func (s *Server) dropNodeEndpoint(c *gin.Context, peerID, purpose string) (baseURL, nodeKey, tok string, ok bool) {
	baseURL, nodeKey, _, err := s.store.NodeAPI(c, peerID)
	if errors.Is(err, store.ErrNotFound) || baseURL == "" {
		fail(c, http.StatusServiceUnavailable, "drop node unavailable")
		return "", "", "", false
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to resolve node")
		return "", "", "", false
	}
	tok, err = s.tokens.IssueGatewayCall(peerID, purpose)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to authorize node call")
		return "", "", "", false
	}
	return baseURL, nodeKey, tok, true
}

// revalidateDropNode confirms a node is still accepting Drop traffic for the
// given scope right before a reservation/stream. Returns false (and writes an
// error) when the node cannot serve the request.
func (s *Server) revalidateDropNode(c *gin.Context, peerID, scope string) bool {
	st, err := s.store.GetNodeDropStatus(c, peerID)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusServiceUnavailable, "drop not available on node")
		return false
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "node status check failed")
		return false
	}
	if !st.Enabled || (st.State != store.DropStateActive && st.State != store.DropStateDegraded) {
		fail(c, http.StatusServiceUnavailable, "drop not available on node")
		return false
	}
	if scope == store.DropScopePublic && !st.AcceptsPublicUploads {
		fail(c, http.StatusServiceUnavailable, "node not accepting public uploads")
		return false
	}
	return true
}

// ── discovery ──────────────────────────────────────

func (s *Server) handleDropNodes(c *gin.Context) {
	scope := c.Query("scope")
	if scope == "" {
		scope = store.DropScopePublic
	}
	orgID := c.Query("org_id")
	if scope == store.DropScopePrivateOrg {
		if orgID == "" {
			fail(c, http.StatusBadRequest, "org_id required for private scope")
			return
		}
		if _, err := s.store.MemberRole(c, orgID, userID(c)); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				fail(c, http.StatusForbidden, "not a member of this org")
				return
			}
			fail(c, http.StatusInternalServerError, "membership check failed")
			return
		}
	} else {
		scope = store.DropScopePublic
	}
	nodes, err := s.store.ListDropNodes(c, scope, orgID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list drop nodes")
		return
	}
	ok(c, http.StatusOK, gin.H{"nodes": nodes, "scope": scope})
}

// ── uploads ────────────────────────────────────────

type dropReserveReq struct {
	NodeID             string          `json:"node_id"`
	Scope              string          `json:"scope"`
	Visibility         string          `json:"visibility"`
	Filename           string          `json:"filename"`
	ContentType        string          `json:"content_type"`
	Size               int64           `json:"size"`
	SHA256             string          `json:"sha256"`
	Encrypted          bool            `json:"encrypted"`
	EncryptionMetadata json.RawMessage `json:"encryption_metadata"`
	OrgID              string          `json:"org_id"`
	IdempotencyKey     string          `json:"idempotency_key"`
}

func (s *Server) handleDropReserveUpload(c *gin.Context) {
	uid := userID(c)
	var req dropReserveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.NodeID == "" || req.Size <= 0 {
		fail(c, http.StatusBadRequest, "node_id and positive size required")
		return
	}
	if req.IdempotencyKey == "" {
		fail(c, http.StatusBadRequest, "idempotency_key required")
		return
	}

	scope := req.Scope
	if scope == "" {
		scope = store.DropScopePublic
	}
	node, err := s.store.GetNode(c, req.NodeID)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "node not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load node")
		return
	}

	in := store.ReserveDropUploadInput{
		UserID: uid, NodeID: req.NodeID, Scope: scope,
		Filename: req.Filename, ContentType: req.ContentType, DeclaredSize: req.Size,
		SHA256: req.SHA256, Encrypted: req.Encrypted, EncryptionMetadata: req.EncryptionMetadata,
		IdempotencyKey: req.IdempotencyKey,
	}

	switch scope {
	case store.DropScopePrivateOrg:
		if req.OrgID == "" {
			fail(c, http.StatusBadRequest, "org_id required for private_org scope")
			return
		}
		if _, err := s.store.MemberRole(c, req.OrgID, uid); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				fail(c, http.StatusForbidden, "not a member of this org")
				return
			}
			fail(c, http.StatusInternalServerError, "membership check failed")
			return
		}
		if node.OrgID != req.OrgID {
			fail(c, http.StatusForbidden, "node does not belong to this org")
			return
		}
		in.OrgID = req.OrgID
		in.Visibility = store.DropVisibilityPrivate // private-org files are never public shares
	default:
		scope = store.DropScopePublic
		in.Scope = scope
		if node.AccessMode != store.NodeAccessPublic {
			fail(c, http.StatusForbidden, "node is not public")
			return
		}
		ent, err := s.store.ResolveDropEntitlement(c, uid)
		if err != nil {
			fail(c, http.StatusInternalServerError, "entitlement check failed")
			return
		}
		in.EntitlementOrgID = ent.EntitlementOrgID
		in.Visibility = req.Visibility
		if in.Visibility != store.DropVisibilityPublic {
			in.Visibility = store.DropVisibilityPrivate
		}
	}

	if !s.revalidateDropNode(c, req.NodeID, scope) {
		return
	}

	up, err := s.store.ReserveDropUpload(c, in)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrDropQuotaExceeded):
			tier := store.DropTierFree
			if ent, e := s.store.ResolveDropEntitlement(c, uid); e == nil {
				tier = ent.Tier
			}
			metrics.DropQuotaRejectionsTotal.WithLabelValues(tier).Inc()
			fail(c, http.StatusConflict, "storage quota exceeded")
		case errors.Is(err, store.ErrDropNodeCapacity):
			fail(c, http.StatusInsufficientStorage, "node storage capacity exhausted")
		case errors.Is(err, store.ErrDropNodeUnavailable):
			fail(c, http.StatusServiceUnavailable, "drop node unavailable")
		default:
			fail(c, http.StatusInternalServerError, "failed to reserve upload")
		}
		return
	}
	ok(c, http.StatusCreated, dropUploadView(up))
}

func (s *Server) handleDropUploadContent(c *gin.Context) {
	uid := userID(c)
	uploadID := c.Param("uploadId")
	up, err := s.store.GetDropUpload(c, uid, uploadID)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "upload not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load upload")
		return
	}
	if up.Status != store.DropUploadReserved && up.Status != store.DropUploadUploading {
		fail(c, http.StatusConflict, "upload is not accepting content")
		return
	}
	if !s.revalidateDropNode(c, up.NodeID, up.StorageScope) {
		return
	}

	baseURL, nodeKey, tok, okNode := s.dropNodeEndpoint(c, up.NodeID, dropclient.PurposeUpload)
	if !okNode {
		return
	}

	maxBytes := up.DeclaredSizeBytes
	if maxBytes <= 0 {
		maxBytes = store.DropMaxFileBytes
	}

	res, err := s.drop.PutUploadContent(c.Request.Context(), baseURL, tok, nodeKey, uploadID, c.Request.Body, up.DeclaredSizeBytes, maxBytes)
	if err != nil {
		metrics.DropUploadsTotal.WithLabelValues("failed", up.StorageScope).Inc()
		metrics.DropNodeOperationsTotal.WithLabelValues("upload", "failed").Inc()
		_ = s.store.ReleaseDropReservation(c, uploadID, store.DropUploadFailed)
		if errors.Is(err, dropclient.ErrByteLimitExceeded) {
			fail(c, http.StatusRequestEntityTooLarge, "upload exceeds declared size")
			return
		}
		fail(c, http.StatusBadGateway, "node upload failed")
		return
	}
	metrics.DropNodeOperationsTotal.WithLabelValues("upload", "ok").Inc()

	if !validCID(res.CID) {
		metrics.DropUploadsTotal.WithLabelValues("failed", up.StorageScope).Inc()
		s.compensateUnpin(up.NodeID, res.CID)
		_ = s.store.ReleaseDropReservation(c, uploadID, store.DropUploadFailed)
		fail(c, http.StatusBadGateway, "node returned invalid cid")
		return
	}

	f, err := s.store.CommitDropUpload(c, uid, uploadID, res.CID, res.SizeBytes)
	if err != nil {
		// Metadata commit failed after the node pinned the object: compensate by
		// unpinning so we do not leak an orphaned pin, then release the hold.
		metrics.DropUploadsTotal.WithLabelValues("failed", up.StorageScope).Inc()
		s.compensateUnpin(up.NodeID, res.CID)
		_ = s.store.ReleaseDropReservation(c, uploadID, store.DropUploadFailed)
		fail(c, http.StatusInternalServerError, "failed to finalize upload")
		return
	}

	metrics.DropUploadsTotal.WithLabelValues("ok", up.StorageScope).Inc()
	metrics.DropUploadBytesTotal.WithLabelValues(up.StorageScope).Add(float64(f.SizeBytes))
	ok(c, http.StatusOK, dropFileView(f, true))
}

// compensateUnpin best-effort removes a pin the gateway could not record. The
// physical CID is never logged with request context; failures are swallowed and
// left for reconciliation.
func (s *Server) compensateUnpin(peerID, cid string) {
	if !validCID(cid) {
		return
	}
	baseURL, nodeKey, _, err := s.store.NodeAPI(context.Background(), peerID)
	if err != nil || baseURL == "" {
		return
	}
	tok, err := s.tokens.IssueGatewayCall(peerID, dropclient.PurposeUnpin)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := s.drop.Unpin(ctx, baseURL, tok, nodeKey, cid); err != nil {
		metrics.DropNodeOperationsTotal.WithLabelValues("unpin", "failed").Inc()
		return
	}
	metrics.DropNodeOperationsTotal.WithLabelValues("unpin", "ok").Inc()
}

func (s *Server) handleDropUploadStatus(c *gin.Context) {
	up, err := s.store.GetDropUpload(c, userID(c), c.Param("uploadId"))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "upload not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load upload")
		return
	}
	ok(c, http.StatusOK, dropUploadView(up))
}

// ── files ──────────────────────────────────────────

func (s *Server) handleDropListFiles(c *gin.Context) {
	files, err := s.store.ListDropFilesByUser(c, userID(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list files")
		return
	}
	out := make([]gin.H, 0, len(files))
	for _, f := range files {
		out = append(out, dropFileView(f, true))
	}
	ok(c, http.StatusOK, gin.H{"files": out})
}

func (s *Server) handleDropGetFile(c *gin.Context) {
	f, owner, ok2 := s.loadAuthorizedFile(c, c.Param("fileId"))
	if !ok2 {
		return
	}
	ok(c, http.StatusOK, dropFileView(f, owner))
}

func (s *Server) handleDropFileContent(c *gin.Context) {
	f, _, ok2 := s.loadAuthorizedFile(c, c.Param("fileId"))
	if !ok2 {
		return
	}
	s.streamDropContent(c, f)
	s.logActivity(c, userID(c), "drop.download", f.ID)
}

func (s *Server) handleDropPatchFile(c *gin.Context) {
	var req struct {
		Visibility string `json:"visibility"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Visibility != store.DropVisibilityPublic && req.Visibility != store.DropVisibilityPrivate {
		fail(c, http.StatusBadRequest, "visibility must be public or private")
		return
	}
	f, err := s.store.SetDropFileVisibility(c, c.Param("fileId"), userID(c), req.Visibility)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "file not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to update file")
		return
	}
	ok(c, http.StatusOK, dropFileView(f, true))
}

func (s *Server) handleDropDeleteFile(c *gin.Context) {
	fileID := c.Param("fileId")
	f, err := s.store.GetDropFile(c, fileID)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "file not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load file")
		return
	}
	if !s.canDeleteDropFile(c, f) {
		fail(c, http.StatusForbidden, "not allowed to delete this file")
		return
	}

	unpinNeeded, _, err := s.store.MarkDropFileDeletePending(c, fileID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to delete file")
		return
	}
	unpinned := true
	if unpinNeeded && validCID(f.CID) {
		baseURL, nodeKey, tok, okNode := s.dropNodeEndpoint(c, f.NodeID, dropclient.PurposeUnpin)
		if okNode {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
			defer cancel()
			if uerr := s.drop.Unpin(ctx, baseURL, tok, nodeKey, f.CID); uerr != nil {
				unpinned = false
				metrics.DropNodeOperationsTotal.WithLabelValues("unpin", "failed").Inc()
			} else {
				metrics.DropNodeOperationsTotal.WithLabelValues("unpin", "ok").Inc()
			}
		} else {
			// endpoint failure already wrote a response; treat as deferred unpin.
			c.Writer.WriteHeaderNow()
			return
		}
	}
	// Reconciliation will retry pins left in error state on a later pass.
	_ = s.store.FinalizeDropFileDeletion(c, fileID, unpinned)
	ok(c, http.StatusOK, gin.H{"status": "deleted", "unpinned": unpinned})
}

// ── usage + entitlement ────────────────────────────

func (s *Server) handleDropUsage(c *gin.Context) {
	uid := userID(c)
	ent, err := s.store.ResolveDropEntitlement(c, uid)
	if err != nil {
		fail(c, http.StatusInternalServerError, "entitlement check failed")
		return
	}
	usage, err := s.store.GetDropQuotaUsage(c, store.DropPrincipalUser, uid)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load usage")
		return
	}
	ok(c, http.StatusOK, gin.H{
		"tier":               ent.Tier,
		"entitlement_org_id": ent.EntitlementOrgID,
		"public_quota_bytes": ent.PublicStorageBytes,
		"max_file_bytes":     ent.MaxFileBytes,
		"used_bytes":         usage.UsedBytes,
		"reserved_bytes":     usage.ReservedBytes,
		"available_bytes":    dropAvail(ent.PublicStorageBytes - usage.UsedBytes - usage.ReservedBytes),
	})
}

// ── org views ──────────────────────────────────────

func (s *Server) handleOrgDropFiles(c *gin.Context) {
	if _, ok2 := s.orgMember(c); !ok2 {
		return
	}
	files, err := s.store.ListDropFilesByOrg(c, c.Param("id"))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list org files")
		return
	}
	out := make([]gin.H, 0, len(files))
	for _, f := range files {
		// Org viewers see metadata only — no decryption keys are stored/returned.
		out = append(out, dropFileView(f, false))
	}
	ok(c, http.StatusOK, gin.H{"files": out})
}

func (s *Server) handleOrgDropUsage(c *gin.Context) {
	if _, ok2 := s.orgMember(c); !ok2 {
		return
	}
	u, err := s.store.OrgDropUsage(c, c.Param("id"))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load org usage")
		return
	}
	ok(c, http.StatusOK, u)
}

// ── encrypted vault ────────────────────────────────

func (s *Server) handleDropGetVault(c *gin.Context) {
	p, err := s.store.GetDropCryptoProfile(c, userID(c))
	if errors.Is(err, store.ErrNotFound) {
		ok(c, http.StatusOK, gin.H{"version": 0, "encrypted_vault": ""})
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load vault")
		return
	}
	ok(c, http.StatusOK, p)
}

func (s *Server) handleDropPutVault(c *gin.Context) {
	var req struct {
		PublicKey       string          `json:"public_key"`
		EncryptedVault  string          `json:"encrypted_vault"`
		KDFMetadata     json.RawMessage `json:"kdf_metadata"`
		ExpectedVersion int             `json:"expected_version"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.EncryptedVault == "" {
		fail(c, http.StatusBadRequest, "encrypted_vault required")
		return
	}
	// Bound the opaque vault so a client cannot use it as unbounded storage.
	if len(req.EncryptedVault) > 1<<20 {
		fail(c, http.StatusRequestEntityTooLarge, "vault too large")
		return
	}
	p, err := s.store.PutDropCryptoProfile(c, userID(c), req.PublicKey, req.EncryptedVault, req.KDFMetadata, req.ExpectedVersion)
	if errors.Is(err, store.ErrConflict) {
		fail(c, http.StatusConflict, "vault version conflict; reload and retry")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to save vault")
		return
	}
	ok(c, http.StatusOK, p)
}

// ── public opaque share ────────────────────────────

func (s *Server) handleDropPublicGet(c *gin.Context) {
	f, err := s.store.GetDropFile(c, c.Param("fileId"))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load file")
		return
	}
	if f.Visibility != store.DropVisibilityPublic || f.Status != store.DropFileActive {
		fail(c, http.StatusNotFound, "not found")
		return
	}
	s.streamDropContent(c, f)
}

// ── WebUI session + proxy ──────────────────────────

type dropWebUISession struct {
	PeerID string `json:"peer_id"`
	OrgID  string `json:"org_id"`
	UserID string `json:"user_id"`
}

func (s *Server) handleDropWebUISession(c *gin.Context) {
	if _, ok2 := s.requireNodeOperator(c); !ok2 {
		return
	}
	orgID := c.Param("id")
	peerID, err := s.store.GetNodePeerID(c, c.Param("nodeId"))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "node not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to resolve node")
		return
	}
	node, err := s.store.GetNode(c, peerID)
	if err != nil || node.OrgID != orgID {
		fail(c, http.StatusForbidden, "node does not belong to this org")
		return
	}
	st, err := s.store.GetNodeDropStatus(c, peerID)
	if err != nil || !st.WebUIAvailable {
		fail(c, http.StatusConflict, "webui not available on node")
		return
	}
	sid, err := randomToken()
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to open session")
		return
	}
	if err := s.cache.SetJSON(c, "drop:webui:"+sid, dropWebUISession{PeerID: peerID, OrgID: orgID, UserID: userID(c)}, 5*time.Minute); err != nil {
		fail(c, http.StatusInternalServerError, "failed to open session")
		return
	}
	ok(c, http.StatusOK, gin.H{"session_path": "/api/v2/drop/webui/" + sid + "/", "expires_in": 300})
}

// handleDropWebUIProxy relays an authenticated same-origin request to a node's
// private Drop WebUI. The node/Kubo address stays server-side; the caller only
// ever sees the gateway-relative session path.
func (s *Server) handleDropWebUIProxy(c *gin.Context) {
	sid := c.Param("sessionId")
	var sess dropWebUISession
	found, err := s.cache.GetJSON(c, "drop:webui:"+sid, &sess)
	if err != nil || !found {
		fail(c, http.StatusNotFound, "session not found or expired")
		return
	}
	baseURL, nodeKey, _, err := s.store.NodeAPI(c, sess.PeerID)
	if err != nil || baseURL == "" {
		fail(c, http.StatusServiceUnavailable, "drop node unavailable")
		return
	}
	tok, err := s.tokens.IssueGatewayCall(sess.PeerID, dropclient.PurposeWebUI)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to authorize session")
		return
	}
	target, err := url.Parse(baseURL)
	if err != nil {
		fail(c, http.StatusServiceUnavailable, "drop node unavailable")
		return
	}
	proxyPath := c.Param("proxyPath")
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Director = func(r *http.Request) {
		r.URL.Scheme = target.Scheme
		r.URL.Host = target.Host
		r.Host = target.Host
		r.URL.Path = strings.TrimRight(target.Path, "/") + "/api/v2/drop/webui" + proxyPath
		r.Header.Set("Authorization", "Bearer "+tok)
		r.Header.Set("X-Erebrus-Node-Key", nodeKey)
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		// Never let the node's absolute address leak back through redirects.
		if loc := resp.Header.Get("Location"); loc != "" {
			resp.Header.Set("Location", "/api/v2/drop/webui/"+sid+"/")
		}
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		w.WriteHeader(http.StatusBadGateway)
	}
	proxy.ServeHTTP(c.Writer, c.Request)
}

// ── shared helpers ─────────────────────────────────

// loadAuthorizedFile loads a file and confirms the caller may read it (owner,
// org member for private-org files, or admin). owner reports whether the caller
// is the owning user (used to decide whether to surface the CID).
func (s *Server) loadAuthorizedFile(c *gin.Context, fileID string) (f *store.DropFile, owner, ok2 bool) {
	f, err := s.store.GetDropFile(c, fileID)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "file not found")
		return nil, false, false
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load file")
		return nil, false, false
	}
	uid := userID(c)
	if f.OwnerUserID == uid {
		return f, true, true
	}
	if c.GetString(ctxRole) == "admin" {
		return f, false, true
	}
	if f.OrgID != "" {
		if _, err := s.store.MemberRole(c, f.OrgID, uid); err == nil {
			return f, false, true
		}
	}
	fail(c, http.StatusForbidden, "not allowed to access this file")
	return nil, false, false
}

// canDeleteDropFile allows the owner, an admin, or an owner/node-operator of the
// file's org to delete. Deletion never returns decryption keys.
func (s *Server) canDeleteDropFile(c *gin.Context, f *store.DropFile) bool {
	uid := userID(c)
	if f.OwnerUserID == uid || c.GetString(ctxRole) == "admin" {
		return true
	}
	if f.OrgID != "" {
		if role, err := s.store.MemberRole(c, f.OrgID, uid); err == nil {
			return role == store.OrgRoleOwner || role == store.OrgRoleNodeOperator
		}
	}
	return false
}

// streamDropContent streams a file's bytes from its node through the gateway to
// the caller without buffering. It validates the CID, sanitizes the filename in
// the Content-Disposition header, and never trusts a client-provided MIME type
// blindly (the stored content type is echoed only as a hint).
func (s *Server) streamDropContent(c *gin.Context, f *store.DropFile) {
	if !validCID(f.CID) {
		fail(c, http.StatusUnprocessableEntity, "invalid object reference")
		return
	}
	if !s.revalidateDropNode(c, f.NodeID, f.StorageScope) {
		return
	}
	baseURL, nodeKey, tok, okNode := s.dropNodeEndpoint(c, f.NodeID, dropclient.PurposeRead)
	if !okNode {
		return
	}
	maxBytes := f.SizeBytes
	if maxBytes <= 0 {
		maxBytes = store.DropMaxFileBytes
	}
	rc, ctype, err := s.drop.GetObject(c.Request.Context(), baseURL, tok, nodeKey, f.CID, maxBytes+(1<<16))
	if dropclient.ErrNotFound(err) {
		metrics.DropNodeOperationsTotal.WithLabelValues("download", "missing").Inc()
		fail(c, http.StatusNotFound, "object not found on node")
		return
	}
	if err != nil {
		metrics.DropNodeOperationsTotal.WithLabelValues("download", "failed").Inc()
		fail(c, http.StatusBadGateway, "node download failed")
		return
	}
	defer rc.Close()

	// Encrypted payloads are opaque octet-streams; only echo a stored type for
	// unencrypted content, and never the raw node-provided type for encrypted.
	ct := "application/octet-stream"
	if !f.Encrypted {
		if f.ContentType != "" {
			ct = f.ContentType
		} else if ctype != "" {
			ct = ctype
		}
	}
	c.Header("Content-Type", ct)
	c.Header("Content-Disposition", contentDisposition(f.Filename))
	c.Header("X-Content-Type-Options", "nosniff")
	c.Status(http.StatusOK)
	n, _ := io.Copy(c.Writer, rc)
	metrics.DropNodeOperationsTotal.WithLabelValues("download", "ok").Inc()
	metrics.DropDownloadBytesTotal.WithLabelValues(f.StorageScope).Add(float64(n))
}

// contentDisposition builds a safe attachment header, stripping control
// characters and quotes and providing both an ASCII fallback and a UTF-8
// (RFC 5987) filename.
func contentDisposition(name string) string {
	clean := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, name)
	if clean == "" {
		clean = "download"
	}
	ascii := strings.Map(func(r rune) rune {
		if r > 0x7e || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, clean)
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, ascii, url.PathEscape(clean))
}

func dropUploadView(u *store.DropUpload) gin.H {
	return gin.H{
		"id":                  u.ID,
		"node_id":             u.NodeID,
		"scope":               u.StorageScope,
		"visibility":          u.Visibility,
		"filename":            u.Filename,
		"declared_size_bytes": u.DeclaredSizeBytes,
		"status":              u.Status,
		"cid":                 u.CID,
		"expires_at":          u.ExpiresAt,
	}
}

// dropFileView renders a file. When owner is false (org viewers/admins) the CID
// is withheld along with any owner-only detail.
func dropFileView(f *store.DropFile, owner bool) gin.H {
	h := gin.H{
		"id":         f.ID,
		"node_id":    f.NodeID,
		"scope":      f.StorageScope,
		"filename":   f.Filename,
		"size_bytes": f.SizeBytes,
		"visibility": f.Visibility,
		"encrypted":  f.Encrypted,
		"status":     f.Status,
		"created_at": f.CreatedAt,
	}
	if owner {
		h["cid"] = f.CID
		if len(f.EncryptionMetadata) > 0 {
			h["encryption_metadata"] = f.EncryptionMetadata
		}
	}
	return h
}

func randomToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func dropAvail(n int64) int64 {
	if n < 0 {
		return 0
	}
	return n
}
