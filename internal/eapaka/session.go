package eapaka

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/config"
	"github.com/redis/go-redis/v9"
)

// EapSessionData represents an EAP session stored in Redis.
type EapSessionData struct {
	IMSI       string
	State      string
	RAND       []byte
	XRES       []byte
	CK         []byte
	IK         []byte
	Identifier int
	KAut       []byte
	KEncr      []byte
	MK         []byte
}

// SessionStore manages EAP session state in Redis.
type SessionStore struct {
	client *redis.Client
}

// NewSessionStore creates a new SessionStore.
func NewSessionStore(client *redis.Client) *SessionStore {
	return &SessionStore{client: client}
}

func sessionKey(sessionID string) string {
	return "eap_session:" + sessionID
}

// Create creates a new EAP session and returns the session ID.
func (s *SessionStore) Create(ctx context.Context, data *EapSessionData) (string, error) {
	sessionID := uuid.New().String()
	key := sessionKey(sessionID)

	fields := map[string]interface{}{
		"imsi":       data.IMSI,
		"state":      data.State,
		"rand":       base64.StdEncoding.EncodeToString(data.RAND),
		"xres":       base64.StdEncoding.EncodeToString(data.XRES),
		"ck":         base64.StdEncoding.EncodeToString(data.CK),
		"ik":         base64.StdEncoding.EncodeToString(data.IK),
		"identifier": strconv.Itoa(data.Identifier),
		"kAut":       base64.StdEncoding.EncodeToString(data.KAut),
		"kEncr":      base64.StdEncoding.EncodeToString(data.KEncr),
		"mk":         base64.StdEncoding.EncodeToString(data.MK),
	}

	pipe := s.client.Pipeline()
	pipe.HSet(ctx, key, fields)
	pipe.Expire(ctx, key, time.Duration(config.SessionTTLSeconds)*time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	return sessionID, nil
}

// Get retrieves an EAP session by ID. Returns nil if not found.
func (s *SessionStore) Get(ctx context.Context, sessionID string) (*EapSessionData, error) {
	key := sessionKey(sessionID)
	data, err := s.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}

	randBytes, _ := base64.StdEncoding.DecodeString(data["rand"])
	xresBytes, _ := base64.StdEncoding.DecodeString(data["xres"])
	ckBytes, _ := base64.StdEncoding.DecodeString(data["ck"])
	ikBytes, _ := base64.StdEncoding.DecodeString(data["ik"])
	kAutBytes, _ := base64.StdEncoding.DecodeString(data["kAut"])
	kEncrBytes, _ := base64.StdEncoding.DecodeString(data["kEncr"])
	mkBytes, _ := base64.StdEncoding.DecodeString(data["mk"])
	identifier, _ := strconv.Atoi(data["identifier"])

	return &EapSessionData{
		IMSI:       data["imsi"],
		State:      data["state"],
		RAND:       randBytes,
		XRES:       xresBytes,
		CK:         ckBytes,
		IK:         ikBytes,
		Identifier: identifier,
		KAut:       kAutBytes,
		KEncr:      kEncrBytes,
		MK:         mkBytes,
	}, nil
}

// Delete removes an EAP session.
func (s *SessionStore) Delete(ctx context.Context, sessionID string) error {
	return s.client.Del(ctx, sessionKey(sessionID)).Err()
}

// Update updates specific fields in an EAP session and resets the TTL.
func (s *SessionStore) Update(ctx context.Context, sessionID string, updates *EapSessionData) error {
	key := sessionKey(sessionID)
	fields := map[string]interface{}{}

	if updates.RAND != nil {
		fields["rand"] = base64.StdEncoding.EncodeToString(updates.RAND)
	}
	if updates.XRES != nil {
		fields["xres"] = base64.StdEncoding.EncodeToString(updates.XRES)
	}
	if updates.CK != nil {
		fields["ck"] = base64.StdEncoding.EncodeToString(updates.CK)
	}
	if updates.IK != nil {
		fields["ik"] = base64.StdEncoding.EncodeToString(updates.IK)
	}
	if updates.KAut != nil {
		fields["kAut"] = base64.StdEncoding.EncodeToString(updates.KAut)
	}
	if updates.KEncr != nil {
		fields["kEncr"] = base64.StdEncoding.EncodeToString(updates.KEncr)
	}
	if updates.MK != nil {
		fields["mk"] = base64.StdEncoding.EncodeToString(updates.MK)
	}
	if updates.Identifier != 0 {
		fields["identifier"] = strconv.Itoa(updates.Identifier)
	}

	if len(fields) == 0 {
		return nil
	}

	pipe := s.client.Pipeline()
	pipe.HSet(ctx, key, fields)
	pipe.Expire(ctx, key, time.Duration(config.SessionTTLSeconds)*time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("update session: %w", err)
	}
	return nil
}
