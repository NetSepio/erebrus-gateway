package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

const orgProfileCols = `id, org_id,
	COALESCE(legal_name,''), COALESCE(display_name,''), COALESCE(description,''),
	COALESCE(logo_url,''), COALESCE(website_url,''),
	COALESCE(public_email,''), COALESCE(billing_email,''), COALESCE(support_email,''),
	COALESCE(country,''), COALESCE(timezone,''), created_at, updated_at`

func scanOrgProfile(sc interface{ Scan(...any) error }) (*OrgProfile, error) {
	var p OrgProfile
	err := sc.Scan(
		&p.ID, &p.OrgID, &p.LegalName, &p.DisplayName, &p.Description,
		&p.LogoURL, &p.WebsiteURL, &p.PublicEmail, &p.BillingEmail, &p.SupportEmail,
		&p.Country, &p.Timezone, &p.CreatedAt, &p.UpdatedAt,
	)
	return &p, err
}

// GetOrgProfile returns the profile for an org.
func (s *Store) GetOrgProfile(ctx context.Context, orgID string) (*OrgProfile, error) {
	p, err := scanOrgProfile(s.db.QueryRowContext(ctx,
		`SELECT `+orgProfileCols+` FROM org_profiles WHERE org_id=$1`, orgID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

// GetOrgBySlug returns an org by its public slug.
func (s *Store) GetOrgBySlug(ctx context.Context, slug string) (*Org, error) {
	o, err := scanOrg(s.db.QueryRowContext(ctx,
		`SELECT `+orgCols+` FROM orgs WHERE slug=$1`, strings.TrimSpace(slug)), false)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return o, err
}

// UpdateOrgProfileInput carries optional profile fields.
type UpdateOrgProfileInput struct {
	LegalName    *string
	DisplayName  *string
	Description  *string
	LogoURL      *string
	WebsiteURL   *string
	PublicEmail  *string
	BillingEmail *string
	SupportEmail *string
	Country      *string
	Timezone     *string
}

// UpdateOrgProfile patches org profile fields.
func (s *Store) UpdateOrgProfile(ctx context.Context, orgID string, in UpdateOrgProfileInput) (*OrgProfile, error) {
	cur, err := s.GetOrgProfile(ctx, orgID)
	if err != nil {
		return nil, err
	}
	p := *cur
	applyStrPtr(&p.LegalName, in.LegalName)
	applyStrPtr(&p.DisplayName, in.DisplayName)
	applyStrPtr(&p.Description, in.Description)
	applyStrPtr(&p.LogoURL, in.LogoURL)
	applyStrPtr(&p.WebsiteURL, in.WebsiteURL)
	applyStrPtr(&p.PublicEmail, in.PublicEmail)
	applyStrPtr(&p.BillingEmail, in.BillingEmail)
	applyStrPtr(&p.SupportEmail, in.SupportEmail)
	applyStrPtr(&p.Country, in.Country)
	applyStrPtr(&p.Timezone, in.Timezone)
	return scanOrgProfile(s.db.QueryRowContext(ctx,
		`UPDATE org_profiles SET
			legal_name=NULLIF($2,''), display_name=NULLIF($3,''), description=NULLIF($4,''),
			logo_url=NULLIF($5,''), website_url=NULLIF($6,''),
			public_email=NULLIF($7,''), billing_email=NULLIF($8,''), support_email=NULLIF($9,''),
			country=NULLIF($10,''), timezone=NULLIF($11,''), updated_at=now()
		 WHERE org_id=$1
		 RETURNING `+orgProfileCols,
		orgID, p.LegalName, p.DisplayName, p.Description, p.LogoURL, p.WebsiteURL,
		p.PublicEmail, p.BillingEmail, p.SupportEmail, p.Country, p.Timezone))
}

// SetOrgPublicProfileEnabled toggles public profile visibility.
func (s *Store) SetOrgPublicProfileEnabled(ctx context.Context, orgID string, enabled bool) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE orgs SET public_profile_enabled=$2, updated_at=now() WHERE id=$1`, orgID, enabled)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// PublicOrgProfile is the public-facing org projection.
type PublicOrgProfile struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Description string `json:"description,omitempty"`
	LogoURL     string `json:"logo_url,omitempty"`
	WebsiteURL  string `json:"website_url,omitempty"`
	PublicEmail string `json:"public_email,omitempty"`
	Country     string `json:"country,omitempty"`
}

// GetPublicOrgBySlug returns a public org profile when enabled.
func (s *Store) GetPublicOrgBySlug(ctx context.Context, slug string) (*PublicOrgProfile, error) {
	var p PublicOrgProfile
	err := s.db.QueryRowContext(ctx,
		`SELECT o.slug, o.name,
		        COALESCE(p.display_name,''), COALESCE(p.description,''),
		        COALESCE(p.logo_url,''), COALESCE(p.website_url,''),
		        COALESCE(p.public_email,''), COALESCE(p.country,'')
		 FROM orgs o JOIN org_profiles p ON p.org_id = o.id
		 WHERE o.slug=$1 AND o.public_profile_enabled = true`, strings.TrimSpace(slug)).
		Scan(&p.Slug, &p.Name, &p.DisplayName, &p.Description, &p.LogoURL, &p.WebsiteURL, &p.PublicEmail, &p.Country)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &p, err
}

// UpdatePublicOrgProfileInput carries public-profile fields.
type UpdatePublicOrgProfileInput struct {
	DisplayName *string
	Description *string
	LogoURL     *string
	WebsiteURL  *string
	PublicEmail *string
	Country     *string
}

// UpdatePublicOrgProfile patches only public-facing profile fields.
func (s *Store) UpdatePublicOrgProfile(ctx context.Context, orgID string, in UpdatePublicOrgProfileInput) (*OrgProfile, error) {
	full := UpdateOrgProfileInput{
		DisplayName: in.DisplayName, Description: in.Description, LogoURL: in.LogoURL,
		WebsiteURL: in.WebsiteURL, PublicEmail: in.PublicEmail, Country: in.Country,
	}
	return s.UpdateOrgProfile(ctx, orgID, full)
}

func applyStrPtr(dst *string, src *string) {
	if src != nil {
		*dst = strings.TrimSpace(*src)
	}
}