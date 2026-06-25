package store

import (
	"context"
	"time"
)

// ActivityEntry is one account-activity / audit record.
type ActivityEntry struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id,omitempty"` // populated in admin views
	Wallet    string    `json:"wallet,omitempty"`  // populated (truncated) in admin views
	Action    string    `json:"action"`
	Target    string    `json:"target,omitempty"`
	IP        string    `json:"ip,omitempty"`
	UserAgent string    `json:"user_agent,omitempty"`
	Device    string    `json:"device,omitempty"`
	App       string    `json:"app,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// LogActivity appends an activity record (best-effort at the call site).
func (s *Store) LogActivity(ctx context.Context, e ActivityEntry) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO activity_log (user_id, action, target, ip, user_agent, device, app)
		 VALUES (NULLIF($1,'')::uuid, $2, NULLIF($3,''), NULLIF($4,''), NULLIF($5,''), NULLIF($6,''), NULLIF($7,''))`,
		e.UserID, e.Action, e.Target, e.IP, e.UserAgent, e.Device, e.App)
	return err
}

// ListUserActivity returns a user's own activity, newest first. cursor is the
// created_at of the last seen row (RFC3339Nano); empty starts at the newest.
func (s *Store) ListUserActivity(ctx context.Context, userID, cursor string, limit int) ([]ActivityEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, action, COALESCE(target,''), COALESCE(ip,''), COALESCE(user_agent,''),
		        COALESCE(device,''), COALESCE(app,''), created_at
		 FROM activity_log
		 WHERE user_id = $1 AND ($2 = '' OR created_at < $2::timestamptz)
		 ORDER BY created_at DESC LIMIT $3`, userID, cursor, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ActivityEntry{}
	for rows.Next() {
		var e ActivityEntry
		if err := rows.Scan(&e.ID, &e.Action, &e.Target, &e.IP, &e.UserAgent, &e.Device, &e.App, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListAllActivity returns fleet-wide activity (admin), newest first, with the
// actor's wallet for cross-reference.
func (s *Store) ListAllActivity(ctx context.Context, cursor string, limit int) ([]ActivityEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT a.id, COALESCE(a.user_id::text,''), COALESCE(u.wallet_address,''),
		        a.action, COALESCE(a.target,''), COALESCE(a.ip,''), COALESCE(a.user_agent,''),
		        COALESCE(a.device,''), COALESCE(a.app,''), a.created_at
		 FROM activity_log a LEFT JOIN users u ON u.id = a.user_id
		 WHERE ($1 = '' OR a.created_at < $1::timestamptz)
		 ORDER BY a.created_at DESC LIMIT $2`, cursor, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ActivityEntry{}
	for rows.Next() {
		var e ActivityEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.Wallet, &e.Action, &e.Target, &e.IP,
			&e.UserAgent, &e.Device, &e.App, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
