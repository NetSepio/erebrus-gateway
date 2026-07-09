package api

import (
	"errors"
	"net/http"

	"github.com/NetSepio/gateway/internal/store"
	"github.com/gin-gonic/gin"
)

// handleGetProfile returns the authenticated user's profile.
func (s *Server) handleGetProfile(c *gin.Context) {
	u, err := s.store.GetUser(c, userID(c))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load profile")
		return
	}
	ok(c, http.StatusOK, u)
}

type patchProfileReq struct {
	// Pointer fields: absent keys leave the column untouched (PATCH semantics).
	Name *string `json:"name"`
	// ProfilePicture is the bare IPFS CID of the uploaded avatar ("" clears it).
	ProfilePicture *string `json:"profile_picture"`
	// Email is not patchable here — link it via POST /api/v2/auth/email (verified).
}

// handlePatchProfile updates mutable profile fields.
func (s *Server) handlePatchProfile(c *gin.Context) {
	var req patchProfileReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.store.UpdateProfile(c, userID(c), req.Name, req.ProfilePicture); err != nil {
		fail(c, http.StatusInternalServerError, "failed to update profile")
		return
	}
	u, _ := s.store.GetUser(c, userID(c))
	ok(c, http.StatusOK, u)
}
