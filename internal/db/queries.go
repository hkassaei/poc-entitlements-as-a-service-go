package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrSubscriberNotFound  = errors.New("subscriber not found")
	ErrEntitlementNotFound = errors.New("entitlement not found")
	ErrTokenNotFound       = errors.New("token not found")
)

// Queries provides database query methods.
type Queries struct {
	pool *pgxpool.Pool
}

// NewQueries creates a new Queries instance.
func NewQueries(pool *pgxpool.Pool) *Queries {
	return &Queries{pool: pool}
}

// FindSubscriberByIMSI looks up a subscriber by IMSI.
func (q *Queries) FindSubscriberByIMSI(ctx context.Context, imsi string) (*Subscriber, error) {
	var s Subscriber
	err := q.pool.QueryRow(ctx, `
		SELECT id, imsi, msisdn, iccid, eid, ki_encrypted, op_encrypted, ki_dek_wrapped, sqn, created_at, updated_at
		FROM subscribers WHERE imsi = $1
	`, imsi).Scan(
		&s.ID, &s.IMSI, &s.MSISDN, &s.ICCID, &s.EID,
		&s.KiEncrypted, &s.OpEncrypted, &s.KiDEKWrapped,
		&s.SQN, &s.CreatedAt, &s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSubscriberNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find subscriber by IMSI: %w", err)
	}
	return &s, nil
}

// UpdateSubscriberSQN updates the SQN for a subscriber.
func (q *Queries) UpdateSubscriberSQN(ctx context.Context, subscriberID string, sqn int64) error {
	_, err := q.pool.Exec(ctx, `
		UPDATE subscribers SET sqn = $1, updated_at = NOW() WHERE id = $2
	`, sqn, subscriberID)
	if err != nil {
		return fmt.Errorf("update subscriber SQN: %w", err)
	}
	return nil
}

// FindEntitlementsBySubscriber returns all entitlements for a subscriber.
func (q *Queries) FindEntitlementsBySubscriber(ctx context.Context, subscriberID string) ([]Entitlement, error) {
	rows, err := q.pool.Query(ctx, `
		SELECT id, subscriber_id, app_id, status, prov_status, tc_status, config_data, created_at, updated_at
		FROM entitlements WHERE subscriber_id = $1
	`, subscriberID)
	if err != nil {
		return nil, fmt.Errorf("find entitlements: %w", err)
	}
	defer rows.Close()

	var entitlements []Entitlement
	for rows.Next() {
		var e Entitlement
		if err := rows.Scan(
			&e.ID, &e.SubscriberID, &e.AppID, &e.Status, &e.ProvStatus, &e.TcStatus,
			&e.ConfigData, &e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan entitlement: %w", err)
		}
		entitlements = append(entitlements, e)
	}
	return entitlements, nil
}

// FindEntitlement finds a specific entitlement by subscriber and appId.
func (q *Queries) FindEntitlement(ctx context.Context, subscriberID, appID string) (*Entitlement, error) {
	var e Entitlement
	err := q.pool.QueryRow(ctx, `
		SELECT id, subscriber_id, app_id, status, prov_status, tc_status, config_data, created_at, updated_at
		FROM entitlements WHERE subscriber_id = $1 AND app_id = $2
	`, subscriberID, appID).Scan(
		&e.ID, &e.SubscriberID, &e.AppID, &e.Status, &e.ProvStatus, &e.TcStatus,
		&e.ConfigData, &e.CreatedAt, &e.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEntitlementNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find entitlement: %w", err)
	}
	return &e, nil
}

// InsertToken inserts a new token record.
func (q *Queries) InsertToken(ctx context.Context, t *Token) error {
	_, err := q.pool.Exec(ctx, `
		INSERT INTO tokens (subscriber_id, token_value, token_type, issued_at, expires_at, scope, operation_targets, consumed, created_by_ip)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, t.SubscriberID, t.TokenValue, t.TokenType, t.IssuedAt, t.ExpiresAt,
		t.Scope, t.OperationTargets, t.Consumed, t.CreatedByIP)
	if err != nil {
		return fmt.Errorf("insert token: %w", err)
	}
	return nil
}

// FindTokenByValue looks up a token by its value.
func (q *Queries) FindTokenByValue(ctx context.Context, tokenValue string) (*Token, error) {
	var t Token
	err := q.pool.QueryRow(ctx, `
		SELECT id, subscriber_id, token_value, token_type, issued_at, expires_at, scope, operation_targets, consumed, created_by_ip
		FROM tokens WHERE token_value = $1
	`, tokenValue).Scan(
		&t.ID, &t.SubscriberID, &t.TokenValue, &t.TokenType,
		&t.IssuedAt, &t.ExpiresAt, &t.Scope, &t.OperationTargets,
		&t.Consumed, &t.CreatedByIP,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find token: %w", err)
	}
	return &t, nil
}

// ConsumeToken marks a token as consumed.
func (q *Queries) ConsumeToken(ctx context.Context, tokenID string) error {
	_, err := q.pool.Exec(ctx, `UPDATE tokens SET consumed = TRUE WHERE id = $1`, tokenID)
	if err != nil {
		return fmt.Errorf("consume token: %w", err)
	}
	return nil
}

// InsertAuditLog writes an audit log entry.
func (q *Queries) InsertAuditLog(ctx context.Context, log *AuditLog) error {
	var reqSummary interface{}
	if log.RequestSummary != nil {
		reqSummary = json.RawMessage(log.RequestSummary)
	}

	_, err := q.pool.Exec(ctx, `
		INSERT INTO audit_log (subscriber_id, app_id, operation, request_summary, response_code, client_ip, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, log.SubscriberID, log.AppID, log.Operation, reqSummary, log.ResponseCode, log.ClientIP, log.UserAgent)
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}
	return nil
}

// DeleteExpiredTokens removes tokens that have expired before the given time.
func (q *Queries) DeleteExpiredTokens(ctx context.Context, before time.Time) (int64, error) {
	tag, err := q.pool.Exec(ctx, `DELETE FROM tokens WHERE expires_at < $1`, before)
	if err != nil {
		return 0, fmt.Errorf("delete expired tokens: %w", err)
	}
	return tag.RowsAffected(), nil
}
