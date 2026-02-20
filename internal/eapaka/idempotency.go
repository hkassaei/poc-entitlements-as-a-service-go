package eapaka

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/config"
	"github.com/redis/go-redis/v9"
)

// IdempotencyCache caches EAP responses for replay protection.
type IdempotencyCache struct {
	client *redis.Client
}

// NewIdempotencyCache creates a new IdempotencyCache.
func NewIdempotencyCache(client *redis.Client) *IdempotencyCache {
	return &IdempotencyCache{client: client}
}

// CacheResponse stores a response for later replay.
func (c *IdempotencyCache) CacheResponse(ctx context.Context, sessionID, eapRelay string, response interface{}) error {
	key := idempotencyKey(sessionID, eapRelay)
	data, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}
	return c.client.Set(ctx, key, data, time.Duration(config.IdempotencyTTLSeconds)*time.Second).Err()
}

// GetCachedResponse retrieves a cached response. Returns nil if not found.
func (c *IdempotencyCache) GetCachedResponse(ctx context.Context, sessionID, eapRelay string) (json.RawMessage, error) {
	key := idempotencyKey(sessionID, eapRelay)
	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get cached response: %w", err)
	}
	return json.RawMessage(data), nil
}

func idempotencyKey(sessionID, eapRelay string) string {
	h := sha256.Sum256([]byte(sessionID + eapRelay))
	return "eap_idempotency:" + hex.EncodeToString(h[:])
}
