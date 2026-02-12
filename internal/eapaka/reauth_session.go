package eapaka

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/hkassaei/entitlements-as-a-service-poc-go/internal/config"
	"github.com/redis/go-redis/v9"
)

// ReauthSessionData represents a re-auth session for challenge/response correlation.
type ReauthSessionData struct {
	ReauthID     string
	NextReauthID string
	NonceS       string // base64
	Counter      int
	Identifier   int
	KAut         string // base64
	KEncr        string // base64
	MK           string // base64
	SubscriberID string
	IMSI         string
}

// ReauthSessionStore manages re-auth sessions in Redis.
type ReauthSessionStore struct {
	client *redis.Client
}

// NewReauthSessionStore creates a new ReauthSessionStore.
func NewReauthSessionStore(client *redis.Client) *ReauthSessionStore {
	return &ReauthSessionStore{client: client}
}

func reauthSessionKey(sessionID string) string {
	return "reauth_session:" + sessionID
}

// Create creates a new re-auth session and returns the session ID.
func (s *ReauthSessionStore) Create(ctx context.Context, data *ReauthSessionData) (string, error) {
	sessionID := uuid.New().String()
	key := reauthSessionKey(sessionID)

	fields := map[string]interface{}{
		"reauthId":     data.ReauthID,
		"nextReauthId": data.NextReauthID,
		"nonceS":       data.NonceS,
		"counter":      strconv.Itoa(data.Counter),
		"identifier":   strconv.Itoa(data.Identifier),
		"kAut":         data.KAut,
		"kEncr":        data.KEncr,
		"mk":           data.MK,
		"subscriberId": data.SubscriberID,
		"imsi":         data.IMSI,
	}

	pipe := s.client.Pipeline()
	pipe.HSet(ctx, key, fields)
	pipe.Expire(ctx, key, time.Duration(config.ReauthSessionTTLSeconds)*time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", fmt.Errorf("create reauth session: %w", err)
	}

	return sessionID, nil
}

// Get retrieves a re-auth session by ID. Returns nil if not found.
func (s *ReauthSessionStore) Get(ctx context.Context, sessionID string) (*ReauthSessionData, error) {
	key := reauthSessionKey(sessionID)
	data, err := s.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("get reauth session: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}

	counter, _ := strconv.Atoi(data["counter"])
	identifier, _ := strconv.Atoi(data["identifier"])

	return &ReauthSessionData{
		ReauthID:     data["reauthId"],
		NextReauthID: data["nextReauthId"],
		NonceS:       data["nonceS"],
		Counter:      counter,
		Identifier:   identifier,
		KAut:         data["kAut"],
		KEncr:        data["kEncr"],
		MK:           data["mk"],
		SubscriberID: data["subscriberId"],
		IMSI:         data["imsi"],
	}, nil
}

// Delete removes a re-auth session.
func (s *ReauthSessionStore) Delete(ctx context.Context, sessionID string) error {
	return s.client.Del(ctx, reauthSessionKey(sessionID)).Err()
}
