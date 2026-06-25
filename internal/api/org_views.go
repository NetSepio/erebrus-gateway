package api

import (
	"context"

	"github.com/NetSepio/gateway/internal/store"
	"github.com/gin-gonic/gin"
)

// orgSummary is the org projection returned alongside nodes and org resources.
// org_id is omitted unless the caller is org owner/admin or platform admin.
type orgSummary struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Kind        string `json:"kind,omitempty"`
	Verified    bool   `json:"verified,omitempty"`
	Slug        string `json:"slug,omitempty"`
	Description string `json:"description,omitempty"`
	Website     string `json:"website,omitempty"`
}

func (s *Server) orgSummaryFor(ctx context.Context, orgID, callerRole string, privileged bool) *orgSummary {
	if orgID == "" {
		return nil
	}
	org, err := s.store.GetOrg(ctx, orgID)
	if err != nil {
		return nil
	}
	out := &orgSummary{
		Name: org.Name, Kind: org.Kind, Verified: org.Verified,
		Slug: org.Slug, Description: org.Description, Website: org.Website,
	}
	if privileged || store.IsOrgPrivileged(callerRole) {
		out.ID = org.ID
	}
	return out
}

func orgResponse(org *store.Org, privileged bool) gin.H {
	if org == nil {
		return gin.H{}
	}
	out := gin.H{
		"name":     org.Name,
		"kind":     org.Kind,
		"verified": org.Verified,
		"created_at": org.CreatedAt,
		"updated_at": org.UpdatedAt,
	}
	if privileged {
		out["id"] = org.ID
		out["owner_user_id"] = org.OwnerUserID
	}
	if org.Slug != "" {
		out["slug"] = org.Slug
	}
	if org.Description != "" {
		out["description"] = org.Description
	}
	if org.Website != "" {
		out["website"] = org.Website
	}
	if org.Role != "" {
		out["role"] = org.Role
	}
	if privileged {
		out["enrollment_secret"] = org.EnrollmentSecret
	}
	return out
}