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
var sha256Pattern = regexp.MustCompile(`^[A-Fa-f0-9]{64}$`)

func validCID(cid string) bool { return cidPattern.MatchString(cid) }

func normalizeDropScope(scope string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "", store.DropScopePublic:
		return store.DropScopePublic, true
	case "private", store.DropScopePrivateOrg:
		return store.DropScopePrivateOrg, true
	default:
		return "", false
	}
}

func publicDropScope(scope string) string {
	if scope == store.DropScopePrivateOrg {
		return "private"
	}
	return store.DropScopePublic
}

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

// revalidateDropNode confirms a node can serve the requested operation. Writes
// require an active node; reads and cleanup may continue while full/degraded.
func (s *Server) revalidateDropNode(c *gin.Context, peerID, scope string, write bool) bool {
	node, err := s.store.GetNode(c, peerID)
	if err != nil || node.Status != "online" {
		fail(c, http.StatusServiceUnavailable, "drop node is offline")
		return false
	}
	st, err := s.store.GetNodeDropStatus(c, peerID)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusServiceUnavailable, "drop not available on node")
		return false
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "node status check failed")
		return false
	}
	usable := st.State == store.DropStateActive
	if !write {
		usable = usable || st.State == store.DropStateDegraded || st.State == store.DropStateFull
	}
	if !st.Enabled || !usable {
		fail(c, http.StatusServiceUnavailable, "drop not available on node")
		return false
	}
	if write && scope == store.DropScopePublic && !st.AcceptsPublicUploads {
		fail(c, http.StatusServiceUnavailable, "node not accepting public uploads")
		return false
	}
	return true
}

// ── discovery ──────────────────────────────────────

func (s *Server) handleDropNodes(c *gin.Context) {
	scope, valid := normalizeDropScope(c.Query("scope"))
	if !valid {
		fail(c, http.StatusBadRequest, "scope must be public or private")
		return
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
	ok(c, http.StatusOK, gin.H{"nodes": nodes, "scope": publicDropScope(scope)})
}

// ── uploads ────────────────────────────────────────

type dropReserveReq struct {
	NodeID             string          `json:"node_id"`
	Scope              string          `json:"scope"`
	Visibility         string          `json:"visibility"`
	Filename           string          `json:"filename"`
	ContentType        string          `json:"content_type"`
	SizeBytes          int64           `json:"size_bytes"`
	LegacySize         int64           `json:"size"`
	SHA256             string          `json:"sha256"`
	Encrypted          bool            `json:"encrypted"`
	EncryptionMetadata json.RawMessage `json:"encryption_metadata"`
	OrgID              string          `json:"org_id"`
	IdempotencyKey     string          `json:"idempotency_key"`
}

func (s *Server) handleDropReserveUpload(c *gin.Context) {
	uid := userID(c)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var req dropReserveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	size := req.SizeBytes
	if size == 0 {
		size = req.LegacySize
	}
	if req.NodeID == "" || size <= 0 {
		fail(c, http.StatusBadRequest, "node_id and positive size_bytes required")
		return
	}
	if req.IdempotencyKey == "" || len(req.IdempotencyKey) > 128 {
		fail(c, http.StatusBadRequest, "idempotency_key must contain 1 to 128 characters")
		return
	}
	if len(req.NodeID) > 128 || len(req.Filename) > 255 || len(req.ContentType) > 255 {
		fail(c, http.StatusBadRequest, "upload metadata is too long")
		return
	}

	scope, valid := normalizeDropScope(req.Scope)
	if !valid {
		fail(c, http.StatusBadRequest, "scope must be public or private")
		return
	}
	if req.SHA256 != "" && !sha256Pattern.MatchString(req.SHA256) {
		fail(c, http.StatusBadRequest, "sha256 must be a 64-character hexadecimal digest")
		return
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
		Filename: req.Filename, ContentType: req.ContentType, DeclaredSize: size,
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
		in.Visibility = req.Visibility
		if in.Visibility != store.DropVisibilityPublic {
			in.Visibility = store.DropVisibilityPrivate
		}
	default:
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

	if in.Visibility == store.DropVisibilityPrivate &&
		(!in.Encrypted || len(in.EncryptionMetadata) == 0) {
		fail(c, http.StatusBadRequest, "private files require client-side encryption metadata")
		return
	}
	if in.Visibility == store.DropVisibilityPublic && in.Encrypted {
		fail(c, http.StatusBadRequest, "public files must be uploaded as plaintext")
		return
	}

	if !s.revalidateDropNode(c, req.NodeID, scope, true) {
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
	if up.Status == store.DropUploadCommitted {
		f, fileErr := s.store.GetDropFileByUpload(c, uploadID)
		if fileErr != nil {
			fail(c, http.StatusInternalServerError, "failed to load committed upload")
			return
		}
		ok(c, http.StatusOK, dropFileView(f, true))
		return
	}
	if up.Status != store.DropUploadReserved && up.Status != store.DropUploadUploading {
		fail(c, http.StatusConflict, "upload is not accepting content")
		return
	}
	if !s.revalidateDropNode(c, up.NodeID, up.StorageScope, true) {
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

	res, err := s.drop.PutUploadContent(c.Request.Context(), baseURL, tok, nodeKey, uploadID, up.SHA256, c.Request.Body, up.DeclaredSizeBytes, maxBytes)
	if err != nil {
		metrics.DropUploadsTotal.WithLabelValues("failed", up.StorageScope).Inc()
		metrics.DropNodeOperationsTotal.WithLabelValues("upload", "failed").Inc()
		cleanupCtx, cancel := dropCleanupContext(c)
		_ = s.store.ReleaseDropReservation(cleanupCtx, uploadID, store.DropUploadFailed)
		cancel()
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
		cleanupCtx, cancel := dropCleanupContext(c)
		_ = s.store.ReleaseDropReservation(cleanupCtx, uploadID, store.DropUploadFailed)
		cancel()
		fail(c, http.StatusBadGateway, "node returned invalid cid")
		return
	}

	f, err := s.store.CommitDropUpload(c, uid, uploadID, res.CID, res.SizeBytes)
	if err != nil {
		// Metadata commit failed after the node pinned the object. Persist a
		// cleanup job; reconciliation checks shared-CID references before unpin.
		metrics.DropUploadsTotal.WithLabelValues("failed", up.StorageScope).Inc()
		cleanupCtx, cancel := dropCleanupContext(c)
		_ = s.store.FailDropUploadWithOrphanPin(cleanupCtx, uploadID, res.CID, "metadata commit failed")
		cancel()
		fail(c, http.StatusInternalServerError, "failed to finalize upload")
		return
	}

	metrics.DropUploadsTotal.WithLabelValues("ok", up.StorageScope).Inc()
	metrics.DropUploadBytesTotal.WithLabelValues(up.StorageScope).Add(float64(f.SizeBytes))
	ok(c, http.StatusOK, dropFileView(f, true))
}

func dropCleanupContext(c *gin.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(c.Request.Context()), 10*time.Second)
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
		out = append(out, s.renderDropFile(c, f, true))
	}
	ok(c, http.StatusOK, gin.H{"files": out})
}

func (s *Server) handleDropGetFile(c *gin.Context) {
	f, owner, ok2 := s.loadAuthorizedFile(c, c.Param("fileId"))
	if !ok2 {
		return
	}
	ok(c, http.StatusOK, s.renderDropFile(c, f, owner))
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
	current, err := s.store.GetDropFile(c, c.Param("fileId"))
	if errors.Is(err, store.ErrNotFound) || (err == nil && current.OwnerUserID != userID(c)) {
		fail(c, http.StatusNotFound, "file not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load file")
		return
	}
	if req.Visibility == store.DropVisibilityPublic && current.Encrypted {
		fail(c, http.StatusBadRequest, "encrypted files cannot be made public")
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
	ok(c, http.StatusOK, s.renderDropFile(c, f, true))
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
		baseURL, nodeKey, _, endpointErr := s.store.NodeAPI(c, f.NodeID)
		tok, tokenErr := s.tokens.IssueGatewayCall(f.NodeID, dropclient.PurposeUnpin)
		if endpointErr == nil && baseURL != "" && tokenErr == nil {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
			defer cancel()
			if uerr := s.drop.Unpin(ctx, baseURL, tok, nodeKey, f.CID); uerr != nil {
				unpinned = false
				metrics.DropNodeOperationsTotal.WithLabelValues("unpin", "failed").Inc()
			} else {
				metrics.DropNodeOperationsTotal.WithLabelValues("unpin", "ok").Inc()
			}
		} else {
			unpinned = false
		}
	}
	if err := s.store.FinalizeDropFileDeletion(c, fileID, unpinned); err != nil {
		fail(c, http.StatusInternalServerError, "failed to finalize deletion")
		return
	}
	if !unpinned {
		ok(c, http.StatusAccepted, gin.H{"status": "delete_pending", "unpinned": false})
		return
	}
	ok(c, http.StatusOK, gin.H{"status": "deleted", "unpinned": true})
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
		"scope":              store.DropScopePublic,
		"entitlement_org_id": ent.EntitlementOrgID,
		"limit_bytes":        ent.PublicStorageBytes,
		"public_quota_bytes": ent.PublicStorageBytes,
		"max_file_bytes":     ent.MaxFileBytes,
		"used_bytes":         usage.UsedBytes,
		"reserved_bytes":     usage.ReservedBytes,
		"available_bytes":    dropAvail(ent.PublicStorageBytes - usage.UsedBytes - usage.ReservedBytes),
	})
}

// ── org views ──────────────────────────────────────

func (s *Server) handleOrgDropFiles(c *gin.Context) {
	role, ok2 := s.orgMember(c)
	if !ok2 {
		return
	}
	files, err := s.store.ListDropFilesByOrg(c, c.Param("id"))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list org files")
		return
	}
	out := make([]gin.H, 0, len(files))
	for _, f := range files {
		if role != store.OrgRoleOwner && role != store.OrgRoleNodeOperator &&
			f.OwnerUserID != userID(c) {
			continue
		}
		// Org viewers see metadata only — no decryption keys are stored/returned.
		out = append(out, dropFileView(f, f.OwnerUserID == userID(c)))
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
		ok(c, http.StatusOK, nil)
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load vault")
		return
	}
	var backup any
	if err := json.Unmarshal([]byte(p.EncryptedVault), &backup); err != nil {
		fail(c, http.StatusInternalServerError, "stored vault is invalid")
		return
	}
	ok(c, http.StatusOK, backup)
}

func (s *Server) handleDropPutVault(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	var req struct {
		Version    int    `json:"version"`
		KDF        string `json:"kdf"`
		Iterations int    `json:"iterations"`
		Salt       string `json:"salt"`
		IV         string `json:"iv"`
		Ciphertext string `json:"ciphertext"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Version <= 0 || req.KDF == "" || req.Iterations <= 0 ||
		req.Salt == "" || req.IV == "" || req.Ciphertext == "" {
		fail(c, http.StatusBadRequest, "complete encrypted vault backup required")
		return
	}
	encryptedVault, err := json.Marshal(req)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid vault")
		return
	}
	if len(encryptedVault) > 1<<20 {
		fail(c, http.StatusRequestEntityTooLarge, "vault too large")
		return
	}
	expectedVersion := 0
	if current, err := s.store.GetDropCryptoProfile(c, userID(c)); err == nil {
		expectedVersion = current.Version
	} else if !errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusInternalServerError, "failed to load vault")
		return
	}
	kdf, _ := json.Marshal(gin.H{
		"kdf": req.KDF, "iterations": req.Iterations, "salt": req.Salt,
	})
	p, err := s.store.PutDropCryptoProfile(c, userID(c), "", string(encryptedVault), kdf, expectedVersion)
	if errors.Is(err, store.ErrConflict) {
		fail(c, http.StatusConflict, "vault version conflict; reload and retry")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to save vault")
		return
	}
	_ = p
	ok(c, http.StatusOK, req)
}

// ── public opaque share ────────────────────────────

// handleDropPublicGet returns public metadata for an opaquely-identified file.
// Content is retrieved only via the gateway proxy at /content (nodes no longer
// advertise a public IPFS gateway base).
func (s *Server) handleDropPublicGet(c *gin.Context) {
	f, ok2 := s.loadPublicDropFile(c)
	if !ok2 {
		return
	}
	ok(c, http.StatusOK, gin.H{
		"id":           f.ID,
		"cid":          f.CID,
		"filename":     f.Filename,
		"size_bytes":   f.SizeBytes,
		"content_type": f.ContentType,
		"encrypted":    f.Encrypted,
		"created_at":   f.CreatedAt,
		"content_url":  "/api/v2/drop/public/" + f.ID + "/content",
	})
}

// handleDropPublicContent streams a public file's bytes through the gateway.
// It sources the object from any healthy node that holds the CID pinned so a
// single node going offline does not break retrieval.
func (s *Server) handleDropPublicContent(c *gin.Context) {
	f, ok2 := s.loadPublicDropFile(c)
	if !ok2 {
		return
	}
	s.streamDropContent(c, f)
}

// loadPublicDropFile loads a file and enforces that it is publicly shareable
// (public visibility + scope, unencrypted, active). Any other state is 404 so
// the opaque id does not leak the existence of private/deleted files.
func (s *Server) loadPublicDropFile(c *gin.Context) (*store.DropFile, bool) {
	f, err := s.store.GetDropFile(c, c.Param("fileId"))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "not found")
		return nil, false
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load file")
		return nil, false
	}
	if f.Visibility != store.DropVisibilityPublic ||
		f.Encrypted || f.Status != store.DropFileActive {
		fail(c, http.StatusNotFound, "not found")
		return nil, false
	}
	return f, true
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
	if node.Status != "online" {
		fail(c, http.StatusServiceUnavailable, "node is offline")
		return
	}
	st, err := s.store.GetNodeDropStatus(c, peerID)
	if err != nil || !st.Enabled || !st.WebUIAvailable ||
		(st.State != store.DropStateActive &&
			st.State != store.DropStateDegraded &&
			st.State != store.DropStateFull) {
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

// handleDropWebUIProxy relays a short-lived capability session to a node's
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
			suffix := ""
			if parsed, parseErr := url.Parse(loc); parseErr == nil && !parsed.IsAbs() {
				suffix = strings.TrimPrefix(parsed.Path, "/api/v2/drop/webui/")
				if suffix == parsed.Path {
					suffix = strings.TrimPrefix(parsed.Path, "/")
				}
				if parsed.RawQuery != "" {
					suffix += "?" + parsed.RawQuery
				}
			}
			resp.Header.Set("Location", "/api/v2/drop/webui/"+sid+"/"+suffix)
		}
		resp.Header.Set("Cache-Control", "no-store")
		resp.Header.Set("Referrer-Policy", "no-referrer")
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		w.WriteHeader(http.StatusBadGateway)
	}
	proxy.ServeHTTP(c.Writer, c.Request)
}

// ── shared helpers ─────────────────────────────────

// loadAuthorizedFile loads a file and confirms the caller may read it. Ordinary
// members can read only their own files; org owners/operators may inspect or
// retrieve ciphertext but never receive another user's encryption metadata.
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
		if role, err := s.store.MemberRole(c, f.OrgID, uid); err == nil &&
			(role == store.OrgRoleOwner || role == store.OrgRoleNodeOperator) {
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

// streamDropContent streams a file's bytes from a node through the gateway to
// the caller without buffering. It validates the CID, sanitizes the filename in
// the Content-Disposition header, and never trusts a client-provided MIME type
// blindly (the stored content type is echoed only as a hint). Retrieval is
// sourced from any healthy node holding the CID pinned (origin first), so a
// single node going offline does not break reads.
func (s *Server) streamDropContent(c *gin.Context, f *store.DropFile) {
	if !validCID(f.CID) {
		fail(c, http.StatusUnprocessableEntity, "invalid object reference")
		return
	}
	candidates, err := s.store.DropPinnedNodes(c, f.CID, f.NodeID)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to resolve object sources")
		return
	}
	if len(candidates) == 0 {
		// Fall back to the recorded origin node when the pin index has no
		// healthy match (e.g. status not yet reconciled to 'pinned').
		candidates = []string{f.NodeID}
	}
	maxBytes := f.SizeBytes
	if maxBytes <= 0 {
		maxBytes = store.DropMaxFileBytes
	}
	var (
		rc     io.ReadCloser
		ctype  string
		anyErr bool
	)
	for _, nodeID := range candidates {
		baseURL, nodeKey, _, aerr := s.store.NodeAPI(c, nodeID)
		if aerr != nil || baseURL == "" {
			anyErr = true
			continue
		}
		tok, terr := s.tokens.IssueGatewayCall(nodeID, dropclient.PurposeRead)
		if terr != nil {
			anyErr = true
			continue
		}
		r, ct2, gerr := s.drop.GetObject(c.Request.Context(), baseURL, tok, nodeKey, f.CID, maxBytes)
		if gerr != nil {
			if !dropclient.ErrNotFound(gerr) {
				anyErr = true
			}
			continue
		}
		rc, ctype = r, ct2
		break
	}
	if rc == nil {
		if anyErr {
			metrics.DropNodeOperationsTotal.WithLabelValues("download", "failed").Inc()
			fail(c, http.StatusBadGateway, "node download failed")
			return
		}
		metrics.DropNodeOperationsTotal.WithLabelValues("download", "missing").Inc()
		fail(c, http.StatusNotFound, "object not found on node")
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
	n, copyErr := io.Copy(c.Writer, rc)
	result := "ok"
	if copyErr != nil {
		result = "failed"
	}
	metrics.DropNodeOperationsTotal.WithLabelValues("download", result).Inc()
	metrics.DropDownloadBytesTotal.WithLabelValues(f.StorageScope).Add(float64(n))
}

// renderDropFile builds a file's response view. Content retrieval is always
// gateway-proxied (`content_url`); nodes no longer advertise a public IPFS
// gateway base for direct browser retrieval.
func (s *Server) renderDropFile(c *gin.Context, f *store.DropFile, owner bool) gin.H {
	return dropFileView(f, owner)
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
		"upload_id":           u.ID,
		"node_id":             u.NodeID,
		"scope":               publicDropScope(u.StorageScope),
		"visibility":          u.Visibility,
		"filename":            u.Filename,
		"size_bytes":          u.DeclaredSizeBytes,
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
		"id":           f.ID,
		"file_id":      f.ID,
		"node_id":      f.NodeID,
		"org_id":       f.OrgID,
		"scope":        publicDropScope(f.StorageScope),
		"filename":     f.Filename,
		"content_type": f.ContentType,
		"size_bytes":   f.SizeBytes,
		"visibility":   f.Visibility,
		"encrypted":    f.Encrypted,
		"can_decrypt":  owner,
		"status":       publicDropFileStatus(f.Status),
		"created_at":   f.CreatedAt,
	}
	if owner {
		h["cid"] = f.CID
		if len(f.EncryptionMetadata) > 0 {
			h["encryption_metadata"] = f.EncryptionMetadata
		}
	}
	return h
}

func publicDropFileStatus(status string) string {
	if status == store.DropFileActive {
		return "available"
	}
	return status
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
