package token

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hkassaei/entitlements-as-a-service-poc-go/internal/config"
	"github.com/hkassaei/entitlements-as-a-service-poc-go/internal/db"
	"github.com/redis/go-redis/v9"
)

const tokenCachePrefix = "token:"

// TokenInfo holds validated token information.
type TokenInfo struct {
	TokenValue   string
	SubscriberID string
	TokenType    string
	ExpiresAt    time.Time
}

// Service generates and validates authentication tokens.
// Tokens are stored in Postgres (source of truth) and cached in Redis.
type Service struct {
	queries *db.Queries
	redis   *redis.Client
	cfg     config.Config
}

// NewService creates a new token service.
func NewService(queries *db.Queries, redisClient *redis.Client, cfg config.Config) *Service {
	return &Service{
		queries: queries,
		redis:   redisClient,
		cfg:     cfg,
	}
}

func tokenCacheKey(tokenValue string) string {
	return tokenCachePrefix + tokenValue
}

type cachedToken struct {
	SubscriberID string `json:"subscriberId"`
	TokenType    string `json:"tokenType"`
	ExpiresAt    string `json:"expiresAt"`
}

// GenerateToken creates a new authentication token for a subscriber.
// Inserts into Postgres and caches in Redis with appropriate TTL.
func (s *Service) GenerateToken(ctx context.Context, subscriberID, tokenType, clientIP string) (*TokenInfo, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate token bytes: %w", err)
	}
	tokenValue := hex.EncodeToString(b)

	var ttlSeconds int
	switch tokenType {
	case config.TokenTypeFastAuth:
		ttlSeconds = s.cfg.FastAuthTokenTTLSeconds
	case config.TokenTypeTemp:
		ttlSeconds = s.cfg.TempTokenTTLSeconds
	default:
		ttlSeconds = s.cfg.AuthTokenTTLSeconds
	}

	expiresAt := time.Now().Add(time.Duration(ttlSeconds) * time.Second)

	err := s.queries.InsertToken(ctx, &db.Token{
		SubscriberID: subscriberID,
		TokenValue:   tokenValue,
		TokenType:    tokenType,
		ExpiresAt:    expiresAt,
		CreatedByIP:  &clientIP,
	})
	if err != nil {
		return nil, fmt.Errorf("insert token: %w", err)
	}

	// Cache in Redis
	cacheData, _ := json.Marshal(cachedToken{
		SubscriberID: subscriberID,
		TokenType:    tokenType,
		ExpiresAt:    expiresAt.UTC().Format(time.RFC3339Nano),
	})
	_ = s.redis.Set(ctx, tokenCacheKey(tokenValue), cacheData, time.Duration(ttlSeconds)*time.Second).Err()

	return &TokenInfo{
		TokenValue:   tokenValue,
		SubscriberID: subscriberID,
		TokenType:    tokenType,
		ExpiresAt:    expiresAt,
	}, nil
}

// ValidateToken checks a token. Checks Redis cache first, falls back to Postgres.
// Returns nil if invalid/expired.
func (s *Service) ValidateToken(ctx context.Context, tokenValue string) (*TokenInfo, error) {
	// Try Redis cache first
	cached, err := s.redis.Get(ctx, tokenCacheKey(tokenValue)).Bytes()
	if err == nil && len(cached) > 0 {
		var data cachedToken
		if json.Unmarshal(cached, &data) == nil {
			expiresAt, _ := time.Parse(time.RFC3339Nano, data.ExpiresAt)
			if expiresAt.After(time.Now()) {
				return &TokenInfo{
					TokenValue:   tokenValue,
					SubscriberID: data.SubscriberID,
					TokenType:    data.TokenType,
					ExpiresAt:    expiresAt,
				}, nil
			}
			// Expired in cache — delete it
			_ = s.redis.Del(ctx, tokenCacheKey(tokenValue)).Err()
		}
	}

	// Fallback to Postgres
	token, err := s.queries.FindTokenByValue(ctx, tokenValue)
	if err != nil {
		return nil, nil // not found
	}

	if token.ExpiresAt.Before(time.Now()) || token.Consumed {
		return nil, nil
	}

	// Re-cache in Redis
	remainingTTL := time.Until(token.ExpiresAt)
	if remainingTTL > 0 {
		cacheData, _ := json.Marshal(cachedToken{
			SubscriberID: token.SubscriberID,
			TokenType:    token.TokenType,
			ExpiresAt:    token.ExpiresAt.UTC().Format(time.RFC3339Nano),
		})
		_ = s.redis.Set(ctx, tokenCacheKey(tokenValue), cacheData, remainingTTL).Err()
	}

	return &TokenInfo{
		TokenValue:   token.TokenValue,
		SubscriberID: token.SubscriberID,
		TokenType:    token.TokenType,
		ExpiresAt:    token.ExpiresAt,
	}, nil
}

// GenerateTemporaryToken creates a temporary token for ODSA operations.
// Stores scope and operation targets in Redis alongside the token cache.
func (s *Service) GenerateTemporaryToken(ctx context.Context, subscriberID, clientIP, scope string, operationTargets []string) (*TokenInfo, error) {
	tokenInfo, err := s.GenerateToken(ctx, subscriberID, config.TokenTypeTemp, clientIP)
	if err != nil {
		return nil, err
	}

	// Store scope and targets in Redis
	scopeData, _ := json.Marshal(struct {
		Scope            string   `json:"scope"`
		OperationTargets []string `json:"operationTargets"`
	}{
		Scope:            scope,
		OperationTargets: operationTargets,
	})
	ttl := time.Duration(s.cfg.TempTokenTTLSeconds) * time.Second
	_ = s.redis.Set(ctx, tokenCachePrefix+"scope:"+tokenInfo.TokenValue, scopeData, ttl).Err()

	return tokenInfo, nil
}
