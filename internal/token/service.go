package token

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/config"
	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/db"
	"github.com/redis/go-redis/v9"
)

// ErrTokenNotFound is returned when a token does not exist or has expired.
var ErrTokenNotFound = errors.New("token not found")

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
		return nil, fmt.Errorf("persist token: %w", err)
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
	if info, ok := s.checkCache(ctx, tokenValue); ok {
		return info, nil
	}

	// Fallback to Postgres
	token, err := s.queries.FindTokenByValue(ctx, tokenValue)
	if errors.Is(err, db.ErrTokenNotFound) {
		return nil, ErrTokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query token: %w", err)
	}

	if token.ExpiresAt.Before(time.Now()) || token.Consumed {
		return nil, ErrTokenNotFound
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

// checkCache attempts to validate a token from Redis cache.
// Returns the token info and true if a valid cached entry exists.
func (s *Service) checkCache(ctx context.Context, tokenValue string) (*TokenInfo, bool) {
	cached, err := s.redis.Get(ctx, tokenCacheKey(tokenValue)).Bytes()
	if err != nil || len(cached) == 0 {
		return nil, false
	}

	var data cachedToken
	if err := json.Unmarshal(cached, &data); err != nil {
		return nil, false
	}

	expiresAt, _ := time.Parse(time.RFC3339Nano, data.ExpiresAt)
	if expiresAt.Before(time.Now()) {
		_ = s.redis.Del(ctx, tokenCacheKey(tokenValue)).Err()
		return nil, false
	}

	return &TokenInfo{
		TokenValue:   tokenValue,
		SubscriberID: data.SubscriberID,
		TokenType:    data.TokenType,
		ExpiresAt:    expiresAt,
	}, true
}

// GenerateTemporaryToken creates a temporary token for ODSA operations.
// Stores scope and operation targets in Redis alongside the token cache.
func (s *Service) GenerateTemporaryToken(ctx context.Context, subscriberID, clientIP, scope string, operationTargets []string) (*TokenInfo, error) {
	tokenInfo, err := s.GenerateToken(ctx, subscriberID, config.TokenTypeTemp, clientIP)
	if err != nil {
		return nil, fmt.Errorf("generate temporary token: %w", err)
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
