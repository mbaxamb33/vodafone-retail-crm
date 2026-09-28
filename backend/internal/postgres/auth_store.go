package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"vodafone/store/internal/crm"
)

const userCols = `u.id::text, u.store_id::text, u.email, u.name, u.role, u.active`

func (q *queries) UserByEmail(ctx context.Context, email string) (crm.User, string, error) {
	var u crm.User
	var hash string
	err := q.q.QueryRow(ctx, `SELECT `+userCols+`, u.password_hash FROM users u WHERE u.email = $1`, email).
		Scan(&u.ID, &u.StoreID, &u.Email, &u.Name, &u.Role, &u.Active, &hash)
	return u, hash, mapErr(err)
}

func (q *queries) UserByID(ctx context.Context, id string) (crm.User, string, error) {
	var u crm.User
	var hash string
	err := q.q.QueryRow(ctx, `SELECT `+userCols+`, u.password_hash FROM users u WHERE u.id = $1`, id).
		Scan(&u.ID, &u.StoreID, &u.Email, &u.Name, &u.Role, &u.Active, &hash)
	return u, hash, mapErr(err)
}

func (q *queries) InsertUser(ctx context.Context, u crm.User, passwordHash string) error {
	_, err := q.q.Exec(ctx, `INSERT INTO users (id, store_id, email, name, role, password_hash, active) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		u.ID, u.StoreID, u.Email, u.Name, u.Role, passwordHash, u.Active)
	return mapErr(err)
}

func (q *queries) SetPasswordHash(ctx context.Context, userID, passwordHash string, keepSession []byte) error {
	if _, err := q.q.Exec(ctx, `UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1`, userID, passwordHash); err != nil {
		return mapErr(err)
	}
	_, err := q.q.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1 AND ($2::bytea IS NULL OR token_hash <> $2)`, userID, keepSession)
	return mapErr(err)
}

func (q *queries) SetUserActive(ctx context.Context, email string, active bool) error {
	tag, err := q.q.Exec(ctx, `UPDATE users SET active = $2, updated_at = now() WHERE email = $1`, email, active)
	if err == nil && tag.RowsAffected() == 0 {
		return mapErr(pgx.ErrNoRows)
	}
	if err == nil && !active {
		_, err = q.q.Exec(ctx, `DELETE FROM sessions WHERE user_id = (SELECT id FROM users WHERE email = $1)`, email)
	}
	return mapErr(err)
}

func (q *queries) InsertSession(ctx context.Context, tokenHash []byte, userID string, expires time.Time) error {
	_, err := q.q.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`, tokenHash, userID, expires)
	return mapErr(err)
}

func (q *queries) SessionUser(ctx context.Context, tokenHash []byte, now time.Time) (crm.User, time.Time, error) {
	var u crm.User
	var lastSeen time.Time
	err := q.q.QueryRow(ctx, `SELECT `+userCols+`, s.last_seen_at FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token_hash = $1 AND s.expires_at > $2`, tokenHash, now).
		Scan(&u.ID, &u.StoreID, &u.Email, &u.Name, &u.Role, &u.Active, &lastSeen)
	return u, lastSeen, mapErr(err)
}

func (q *queries) TouchSession(ctx context.Context, tokenHash []byte, now time.Time) error {
	_, err := q.q.Exec(ctx, `UPDATE sessions SET last_seen_at = $2 WHERE token_hash = $1`, tokenHash, now)
	return mapErr(err)
}

func (q *queries) DeleteSession(ctx context.Context, tokenHash []byte) error {
	_, err := q.q.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return mapErr(err)
}

func (q *queries) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	if _, err := q.q.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= $1`, now); err != nil {
		return mapErr(err)
	}
	_, err := q.q.Exec(ctx, `DELETE FROM login_attempts WHERE at < $1`, now.Add(-24*time.Hour))
	return mapErr(err)
}

func (q *queries) RecordLoginAttempt(ctx context.Context, email, ip string, success bool, at time.Time) error {
	_, err := q.q.Exec(ctx, `INSERT INTO login_attempts (email, client_ip, success, at) VALUES ($1, $2, $3, $4)`, email, ip, success, at)
	return mapErr(err)
}

// RecentFailures counts failures since the window start. A successful login resets the
// per-email count; the per-IP count is not reset.
func (q *queries) RecentFailures(ctx context.Context, email, ip string, since time.Time) (int, int, error) {
	var byEmail, byIP int
	err := q.q.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM login_attempts WHERE email = $1 AND NOT success
				AND at >= greatest($3, coalesce((SELECT max(at) FROM login_attempts WHERE email = $1 AND success), $3))),
			(SELECT count(*) FROM login_attempts WHERE client_ip = $2 AND NOT success AND at >= $3)`, email, ip, since).Scan(&byEmail, &byIP)
	return byEmail, byIP, mapErr(err)
}
