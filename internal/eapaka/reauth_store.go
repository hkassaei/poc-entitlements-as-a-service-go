package eapaka

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// ReauthState is the long-lived re-auth state stored in Redis.
type ReauthState struct {
	SubscriberID string
	IMSI         string
	MK           string // base64
	KAut         string // base64
	KEncr        string // base64
	Counter      int
	Identity     string // the re-auth ID itself
}

// ReauthStore manages long-lived re-auth state in Redis.
type ReauthStore struct {
	client *redis.Client
	ttl    time.Duration
}

// NewReauthStore creates a new ReauthStore.
func NewReauthStore(client *redis.Client, ttlSeconds int) *ReauthStore {
	return &ReauthStore{
		client: client,
		ttl:    time.Duration(ttlSeconds) * time.Second,
	}
}

func reauthStoreKey(reauthID string) string {
	return "reauth:" + reauthID
}

// GenerateReauthID generates a cryptographically random re-auth identity.
func GenerateReauthID() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto/rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Store saves re-auth state in Redis with TTL.
func (s *ReauthStore) Store(ctx context.Context, state *ReauthState) error {
	key := reauthStoreKey(state.Identity)
	fields := map[string]interface{}{
		"subscriberId": state.SubscriberID,
		"imsi":         state.IMSI,
		"mk":           state.MK,
		"kAut":         state.KAut,
		"kEncr":        state.KEncr,
		"counter":      strconv.Itoa(state.Counter),
		"identity":     state.Identity,
	}

	pipe := s.client.Pipeline()
	pipe.HSet(ctx, key, fields)
	pipe.Expire(ctx, key, s.ttl)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("store reauth state: %w", err)
	}
	return nil
}

// Get retrieves re-auth state by identity. Returns nil if not found.
func (s *ReauthStore) Get(ctx context.Context, reauthID string) (*ReauthState, error) {
	key := reauthStoreKey(reauthID)
	data, err := s.client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("get reauth state: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}

	counter, _ := strconv.Atoi(data["counter"])

	return &ReauthState{
		SubscriberID: data["subscriberId"],
		IMSI:         data["imsi"],
		MK:           data["mk"],
		KAut:         data["kAut"],
		KEncr:        data["kEncr"],
		Counter:      counter,
		Identity:     data["identity"],
	}, nil
}

// Delete removes re-auth state.
func (s *ReauthStore) Delete(ctx context.Context, reauthID string) error {
	return s.client.Del(ctx, reauthStoreKey(reauthID)).Err()
}
