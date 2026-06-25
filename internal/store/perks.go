package store

import (
	"context"
	"encoding/json"
	"time"
)

// Perk is a catalog entry (admin-managed).
type Perk struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Type     string          `json:"type"` // nft | xp | free_days | node_pool
	MinTier  int             `json:"min_tier"`
	Meta     json.RawMessage `json:"meta"`
	IsActive bool            `json:"is_active"`
}

// UserPerk is a perk granted to a user (joined with its catalog entry).
type UserPerk struct {
	PerkID    string          `json:"perk_id"`
	Name      string          `json:"name"`
	Type      string          `json:"type"`
	GrantedAt time.Time       `json:"granted_at"`
	Meta      json.RawMessage `json:"meta"`
}

// UpsertPerk creates or updates a catalog perk (admin).
func (s *Store) UpsertPerk(ctx context.Context, p Perk) error {
	meta := p.Meta
	if len(meta) == 0 {
		meta = json.RawMessage("{}")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO perks (id, name, type, min_tier, meta, is_active)
		 VALUES ($1, $2, $3, $4, $5::jsonb, $6)
		 ON CONFLICT (id) DO UPDATE SET
		   name = EXCLUDED.name, type = EXCLUDED.type, min_tier = EXCLUDED.min_tier,
		   meta = EXCLUDED.meta, is_active = EXCLUDED.is_active`,
		p.ID, p.Name, p.Type, p.MinTier, string(meta), p.IsActive)
	return err
}

// ListPerks returns catalog perks (active only unless includeInactive).
func (s *Store) ListPerks(ctx context.Context, includeInactive bool) ([]Perk, error) {
	q := `SELECT id, name, type, min_tier, meta, is_active FROM perks`
	if !includeInactive {
		q += ` WHERE is_active`
	}
	q += ` ORDER BY min_tier, id`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Perk{}
	for rows.Next() {
		var p Perk
		if err := rows.Scan(&p.ID, &p.Name, &p.Type, &p.MinTier, &p.Meta, &p.IsActive); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GrantPerk grants a perk to a user (idempotent).
func (s *Store) GrantPerk(ctx context.Context, userID, perkID string, meta map[string]any) error {
	raw, err := json.Marshal(meta)
	if err != nil || len(raw) == 0 {
		raw = []byte("{}")
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO user_perks (user_id, perk_id, meta) VALUES ($1, $2, $3::jsonb)
		 ON CONFLICT (user_id, perk_id) DO NOTHING`, userID, perkID, string(raw))
	return err
}

// ListUserPerks returns the perks granted to a user.
func (s *Store) ListUserPerks(ctx context.Context, userID string) ([]UserPerk, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT up.perk_id, p.name, p.type, up.granted_at, up.meta
		 FROM user_perks up JOIN perks p ON p.id = up.perk_id
		 WHERE up.user_id = $1 ORDER BY up.granted_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserPerk{}
	for rows.Next() {
		var u UserPerk
		if err := rows.Scan(&u.PerkID, &u.Name, &u.Type, &u.GrantedAt, &u.Meta); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// GrantedPerkIDs returns the set of perk ids granted to a user.
func (s *Store) GrantedPerkIDs(ctx context.Context, userID string) (map[string]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT perk_id FROM user_perks WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}
