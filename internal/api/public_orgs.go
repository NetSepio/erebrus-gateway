package api

import (
	"errors"
	"net/http"

	"github.com/NetSepio/gateway/internal/store"
	"github.com/gin-gonic/gin"
)

func (s *Server) handlePublicOrgBySlug(c *gin.Context) {
	profile, err := s.store.GetPublicOrgBySlug(c, c.Param("slug"))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "public org not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load public org")
		return
	}
	ok(c, http.StatusOK, profile)
}

func (s *Server) handleGetOrgPublicProfile(c *gin.Context) {
	org, err := s.store.GetOrg(c, c.Param("id"))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "org not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load org")
		return
	}
	if !org.PublicProfileEnabled {
		fail(c, http.StatusNotFound, "public profile disabled")
		return
	}
	profile, err := s.store.GetOrgProfile(c, c.Param("id"))
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "profile not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to load profile")
		return
	}
	ok(c, http.StatusOK, gin.H{
		"slug":         org.Slug,
		"name":         org.Name,
		"display_name": profile.DisplayName,
		"description":  profile.Description,
		"logo_url":     profile.LogoURL,
		"website_url":  profile.WebsiteURL,
		"public_email": profile.PublicEmail,
		"country":      profile.Country,
	})
}

func (s *Server) handlePatchOrgPublicProfile(c *gin.Context) {
	if _, ok := s.orgPrivileged(c); !ok {
		return
	}
	var req store.UpdatePublicOrgProfileInput
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	profile, err := s.store.UpdatePublicOrgProfile(c, c.Param("id"), req)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "profile not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "failed to update public profile")
		return
	}
	if err := s.store.SetOrgPublicProfileEnabled(c, c.Param("id"), true); err != nil {
		fail(c, http.StatusInternalServerError, "failed to enable public profile")
		return
	}
	ok(c, http.StatusOK, gin.H{
		"display_name": profile.DisplayName,
		"description":  profile.Description,
		"logo_url":     profile.LogoURL,
		"website_url":  profile.WebsiteURL,
		"public_email": profile.PublicEmail,
		"country":      profile.Country,
	})
}