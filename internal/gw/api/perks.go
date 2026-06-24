package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/NetSepio/gateway/internal/gw/store"
	"github.com/gin-gonic/gin"
)

var validPerkTypes = map[string]bool{"nft": true, "xp": true, "free_days": true, "node_pool": true}

// handleListPerks lists active catalog perks, annotated for the caller with
// whether their tier unlocks each perk and whether it's already granted.
func (s *Server) handleListPerks(c *gin.Context) {
	perks, err := s.store.ListPerks(c, false)
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list perks")
		return
	}
	_, _, tier, _ := s.store.UserXP(c, userID(c))
	granted, _ := s.store.GrantedPerkIDs(c, userID(c))

	out := make([]gin.H, 0, len(perks))
	for _, p := range perks {
		out = append(out, gin.H{
			"id": p.ID, "name": p.Name, "type": p.Type, "min_tier": p.MinTier,
			"meta": p.Meta, "unlocked": tier >= p.MinTier, "granted": granted[p.ID],
		})
	}
	ok(c, http.StatusOK, gin.H{"tier": tier, "perks": out})
}

// handleMyPerks lists the caller's granted perks.
func (s *Server) handleMyPerks(c *gin.Context) {
	perks, err := s.store.ListUserPerks(c, userID(c))
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to list perks")
		return
	}
	ok(c, http.StatusOK, perks)
}

type upsertPerkReq struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	MinTier  int             `json:"min_tier"`
	Meta     json.RawMessage `json:"meta"`
	IsActive *bool           `json:"is_active"`
}

// handleAdminUpsertPerk creates or updates a catalog perk (admin).
func (s *Server) handleAdminUpsertPerk(c *gin.Context) {
	var req upsertPerkReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.ID) == "" || req.Name == "" {
		fail(c, http.StatusBadRequest, "id and name are required")
		return
	}
	if !validPerkTypes[req.Type] {
		fail(c, http.StatusBadRequest, "type must be nft|xp|free_days|node_pool")
		return
	}
	if req.MinTier < 0 {
		fail(c, http.StatusBadRequest, "min_tier must be >= 0")
		return
	}
	active := true
	if req.IsActive != nil {
		active = *req.IsActive
	}
	if err := s.store.UpsertPerk(c, store.Perk{
		ID: req.ID, Name: req.Name, Type: req.Type, MinTier: req.MinTier, Meta: req.Meta, IsActive: active,
	}); err != nil {
		fail(c, http.StatusInternalServerError, "failed to save perk")
		return
	}
	ok(c, http.StatusOK, gin.H{"id": req.ID, "saved": true})
}

type grantPerkReq struct {
	UserID string `json:"user_id"`
	Wallet string `json:"wallet"`
}

// handleAdminGrantPerk grants a catalog perk to a user (by user_id or wallet).
func (s *Server) handleAdminGrantPerk(c *gin.Context) {
	var req grantPerkReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	uid := strings.TrimSpace(req.UserID)
	if uid == "" && req.Wallet != "" {
		resolved, err := s.store.UserIDByWallet(c, req.Wallet)
		if errors.Is(err, store.ErrNotFound) {
			fail(c, http.StatusNotFound, "no user for that wallet")
			return
		}
		if err != nil {
			fail(c, http.StatusInternalServerError, "failed to resolve user")
			return
		}
		uid = resolved
	}
	if uid == "" {
		fail(c, http.StatusBadRequest, "user_id or wallet is required")
		return
	}
	if err := s.store.GrantPerk(c, uid, c.Param("id"), nil); err != nil {
		fail(c, http.StatusInternalServerError, "failed to grant perk")
		return
	}
	ok(c, http.StatusOK, gin.H{"user_id": uid, "perk_id": c.Param("id"), "granted": true})
}
