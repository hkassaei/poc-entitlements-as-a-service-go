package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/config"
	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/eapaka"
	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/services"
	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/token"
)

// EntitlementHandler handles POST and GET /entitlement requests.
type EntitlementHandler struct {
	orchestrator       *eapaka.Orchestrator
	reauthHandler      *eapaka.ReauthHandler
	reauthStore        *eapaka.ReauthStore
	reauthSessionStore *eapaka.ReauthSessionStore
	idempotencyCache   *eapaka.IdempotencyCache
	tokenService       *token.Service
	responseBuilder    *services.ResponseBuilder
	cfg                config.Config
}

// NewEntitlementHandler creates a new EntitlementHandler.
func NewEntitlementHandler(
	orchestrator *eapaka.Orchestrator,
	reauthHandler *eapaka.ReauthHandler,
	reauthStore *eapaka.ReauthStore,
	reauthSessionStore *eapaka.ReauthSessionStore,
	idempotencyCache *eapaka.IdempotencyCache,
	tokenService *token.Service,
	responseBuilder *services.ResponseBuilder,
	cfg config.Config,
) *EntitlementHandler {
	return &EntitlementHandler{
		orchestrator:       orchestrator,
		reauthHandler:      reauthHandler,
		reauthStore:        reauthStore,
		reauthSessionStore: reauthSessionStore,
		idempotencyCache:   idempotencyCache,
		tokenService:       tokenService,
		responseBuilder:    responseBuilder,
		cfg:                cfg,
	}
}

type entitlementPostRequest struct {
	App                string `json:"app"`
	TerminalID         string `json:"terminal_id"`
	EntitlementVersion string `json:"entitlement_version"`
	IMSI               string `json:"imsi,omitempty"`
	Token              string `json:"token,omitempty"`
	EapRelay           string `json:"eap_relay,omitempty"`
	AcceptContentType  string `json:"accept_content_type,omitempty"`
	Operation          string `json:"operation,omitempty"`
	OperationType      int    `json:"operation_type,omitempty"`
}

// HandlePost handles POST /entitlement with 4-path routing.
func (h *EntitlementHandler) HandlePost(w http.ResponseWriter, r *http.Request) {
	var body entitlementPostRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "Bad Request", "message": "Invalid request body", "statusCode": 400,
		})
		return
	}

	ctx := r.Context()
	ip := clientIP(r)

	// --- Path 1: EAP Response (eap_relay present) ---
	if body.EapRelay != "" {
		h.handleEapRelayPath(ctx, w, r, body, ip)
		return
	}

	// --- Path 2: Token present ---
	if body.Token != "" {
		h.handleTokenPath(ctx, w, body, ip)
		return
	}

	// --- Path 3: Initial Request ---
	h.handleInitialPath(ctx, w, body)
}

func (h *EntitlementHandler) handleEapRelayPath(ctx context.Context, w http.ResponseWriter, r *http.Request, body entitlementPostRequest, ip string) {
	sessionID := r.Header.Get("X-EAP-Session-Id")
	if sessionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "Bad Request", "message": "Missing X-EAP-Session-Id header", "statusCode": 400,
		})
		return
	}

	// Check idempotency cache
	cached, _ := h.idempotencyCache.GetCachedResponse(ctx, sessionID, body.EapRelay)
	if cached != nil {
		slog.Info("Returning cached EAP response", "sessionId", sessionID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(cached)
		return
	}

	odsaCtx := h.buildOdsaContext(body.Operation, body.OperationType)

	// Try re-auth session first, fall back to full auth
	reauthSession, _ := h.reauthSessionStore.Get(ctx, sessionID)
	if reauthSession != nil {
		h.handleReauthEapRelay(ctx, w, body, sessionID, ip, odsaCtx)
		return
	}

	h.handleFullAuthEapRelay(ctx, w, body, sessionID, odsaCtx)
}

func (h *EntitlementHandler) handleReauthEapRelay(ctx context.Context, w http.ResponseWriter, body entitlementPostRequest, sessionID, ip string, odsaCtx *services.OdsaContext) {
	result, err := h.reauthHandler.HandleReauthResponse(ctx, body.EapRelay, sessionID, ip)
	if err != nil {
		slog.Error("Re-auth response error", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "Internal Server Error", "message": "Re-authentication failed", "statusCode": 500,
		})
		return
	}

	if result.StatusCode == 200 && result.ReauthID != "" {
		formatted, err := h.responseBuilder.BuildEntitlementResponse(ctx, result.ReauthID, result.SubscriberID, body.App, body.AcceptContentType, odsaCtx)
		if err != nil {
			slog.Error("Build response error", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"error": "Internal Server Error", "message": "Failed to build response", "statusCode": 500,
			})
			return
		}
		h.handleTempTokenSideEffect(ctx, formatted, body.Operation, body.App, result.SubscriberID, ip)
		h.sendFormattedResponse(ctx, w, formatted, result.EapRelay, sessionID, body.EapRelay)
		return
	}

	// Counter-too-small fallback
	if result.StatusCode == 401 && result.SessionID != "" {
		w.Header().Set("X-EAP-Session-Id", result.SessionID)
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"eap_relay": result.EapRelay, "statusCode": 401,
		})
		return
	}

	writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
		"error": "Unauthorized", "message": "Re-authentication failed",
		"eap_relay": result.EapRelay, "statusCode": 401,
	})
}

func (h *EntitlementHandler) handleFullAuthEapRelay(ctx context.Context, w http.ResponseWriter, body entitlementPostRequest, sessionID string, odsaCtx *services.OdsaContext) {
	result, err := h.orchestrator.HandleEapResponse(ctx, body.EapRelay, sessionID)
	if err != nil {
		slog.Error("EAP response error", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "Internal Server Error", "message": "Authentication failed", "statusCode": 500,
		})
		return
	}

	if result.StatusCode == 200 && result.Token != "" {
		formatted, err := h.responseBuilder.BuildEntitlementResponse(ctx, result.Token, result.SubscriberID, body.App, body.AcceptContentType, odsaCtx)
		if err != nil {
			slog.Error("Build response error", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"error": "Internal Server Error", "message": "Failed to build response", "statusCode": 500,
			})
			return
		}
		h.sendFormattedResponse(ctx, w, formatted, result.EapRelay, sessionID, body.EapRelay)
		return
	}

	// SQN resync: new challenge issued
	if result.StatusCode == 401 && result.SessionID != "" {
		w.Header().Set("X-EAP-Session-Id", result.SessionID)
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"eap_relay": result.EapRelay, "statusCode": 401,
		})
		return
	}

	writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
		"error": "Unauthorized", "message": "EAP-AKA authentication failed",
		"eap_relay": result.EapRelay, "statusCode": 401,
	})
}

func (h *EntitlementHandler) handleTokenPath(ctx context.Context, w http.ResponseWriter, body entitlementPostRequest, ip string) {
	// Path 2a: Try re-auth state first
	reauthState, _ := h.reauthStore.Get(ctx, body.Token)
	if reauthState != nil {
		reauthChallenge, err := h.reauthHandler.HandleReauthRequest(ctx, body.Token)
		if err != nil {
			slog.Error("Re-auth request error", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"error": "Internal Server Error", "message": "Re-authentication failed", "statusCode": 500,
			})
			return
		}
		if reauthChallenge != nil {
			w.Header().Set("X-EAP-Session-Id", reauthChallenge.SessionID)
			writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
				"eap_relay": reauthChallenge.EapRelay, "statusCode": 401,
			})
			return
		}
		// Counter exhausted — fall through to check ODSA token
	}

	// Path 2b: ODSA temporary token
	tokenInfo, err := h.tokenService.ValidateToken(ctx, body.Token)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"error": "Unauthorized", "message": "Invalid or expired token", "statusCode": 401,
		})
		return
	}

	odsaCtx := h.buildOdsaContext(body.Operation, body.OperationType)
	formatted, err := h.responseBuilder.BuildEntitlementResponse(ctx, body.Token, tokenInfo.SubscriberID, body.App, body.AcceptContentType, odsaCtx)
	if err != nil {
		slog.Error("Build response error", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "Internal Server Error", "message": "Failed to build response", "statusCode": 500,
		})
		return
	}

	h.handleTempTokenSideEffect(ctx, formatted, body.Operation, body.App, tokenInfo.SubscriberID, ip)
	h.sendFormattedResponseNoCache(w, formatted)
}

func (h *EntitlementHandler) handleInitialPath(ctx context.Context, w http.ResponseWriter, body entitlementPostRequest) {
	if body.IMSI == "" {
		writeJSON(w, http.StatusBadRequest, map[string]interface{}{
			"error": "Bad Request", "message": "IMSI is required for initial authentication", "statusCode": 400,
		})
		return
	}

	challenge, err := h.orchestrator.HandleInitialRequest(ctx, body.IMSI)
	if err != nil {
		if errors.Is(err, eapaka.ErrHSSSubscriberNotFound) {
			writeJSON(w, http.StatusForbidden, map[string]interface{}{
				"error": "Forbidden", "message": "Unknown subscriber", "statusCode": 403,
			})
			return
		}
		slog.Error("Initial request error", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "Internal Server Error", "message": "Authentication failed", "statusCode": 500,
		})
		return
	}

	w.Header().Set("X-EAP-Session-Id", challenge.SessionID)
	writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
		"eap_relay": challenge.EapRelay, "statusCode": 401,
	})
}

// HandleGet handles GET /entitlement — read-only, requires valid token.
func (h *EntitlementHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	tokenVal := q.Get("token")
	if tokenVal == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"error": "Unauthorized", "message": "Token is required", "statusCode": 401,
		})
		return
	}

	appID := q.Get("app")
	ct := q.Get("accept_content_type")
	operation := q.Get("operation")
	opTypeRaw := q.Get("operation_type")

	var odsaCtx *services.OdsaContext
	if operation != "" {
		opType := 0
		if opTypeRaw != "" {
			opType, _ = strconv.Atoi(opTypeRaw)
		}
		odsaCtx = &services.OdsaContext{Operation: operation, OperationType: opType}
	}

	// Try re-auth state first (read-only, no counter change)
	reauthState, _ := h.reauthStore.Get(ctx, tokenVal)
	if reauthState != nil {
		formatted, err := h.responseBuilder.BuildEntitlementResponse(ctx, tokenVal, reauthState.SubscriberID, appID, ct, odsaCtx)
		if err != nil {
			slog.Error("Build response error", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"error": "Internal Server Error", "message": "Failed to build response", "statusCode": 500,
			})
			return
		}
		h.sendFormattedResponseNoCache(w, formatted)
		return
	}

	// Fall back to ODSA temporary token
	tokenInfo, err := h.tokenService.ValidateToken(ctx, tokenVal)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"error": "Unauthorized", "message": "Invalid or expired token", "statusCode": 401,
		})
		return
	}

	formatted, err := h.responseBuilder.BuildEntitlementResponse(ctx, tokenVal, tokenInfo.SubscriberID, appID, ct, odsaCtx)
	if err != nil {
		slog.Error("Build response error", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
			"error": "Internal Server Error", "message": "Failed to build response", "statusCode": 500,
		})
		return
	}
	h.sendFormattedResponseNoCache(w, formatted)
}

func (h *EntitlementHandler) buildOdsaContext(operation string, operationType int) *services.OdsaContext {
	if operation == "" {
		return nil
	}
	return &services.OdsaContext{Operation: operation, OperationType: operationType}
}

func (h *EntitlementHandler) sendFormattedResponse(ctx context.Context, w http.ResponseWriter, formatted *services.FormattedResponse, eapRelay, sessionID, reqEapRelay string) {
	if xmlStr, ok := formatted.Body.(string); ok {
		cacheData := map[string]interface{}{"_xml": xmlStr, "eap_relay": eapRelay}
		_ = h.idempotencyCache.CacheResponse(ctx, sessionID, reqEapRelay, cacheData)
		writeXML(w, http.StatusOK, xmlStr)
		return
	}

	jsonBody, ok := formatted.Body.(map[string]interface{})
	if !ok {
		writeJSON(w, http.StatusOK, formatted.Body)
		return
	}

	jsonBody["eap_relay"] = eapRelay
	_ = h.idempotencyCache.CacheResponse(ctx, sessionID, reqEapRelay, jsonBody)
	writeJSON(w, http.StatusOK, jsonBody)
}

func (h *EntitlementHandler) sendFormattedResponseNoCache(w http.ResponseWriter, formatted *services.FormattedResponse) {
	if xmlStr, ok := formatted.Body.(string); ok {
		writeXML(w, http.StatusOK, xmlStr)
		return
	}
	writeJSON(w, http.StatusOK, formatted.Body)
}

func (h *EntitlementHandler) handleTempTokenSideEffect(ctx context.Context, _ *services.FormattedResponse, operation, appID, subscriberID, ip string) {
	if operation != "AcquireTemporaryToken" || (appID != "ap2006" && appID != "ap2009") {
		return
	}
	// Generate temporary token — in full implementation this gets embedded into the response body
	_, _ = h.tokenService.GenerateTemporaryToken(ctx, subscriberID, ip, appID, []string{operation})
}
