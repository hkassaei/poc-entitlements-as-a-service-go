package eapaka

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"log/slog"
	"sync/atomic"

	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/config"
)

// ReauthChallengeResult is returned from re-auth RT1.
type ReauthChallengeResult struct {
	StatusCode int
	EapRelay   string
	SessionID  string
}

// ReauthResult is returned from re-auth RT2.
type ReauthResult struct {
	StatusCode   int
	ReauthID     string
	EapRelay     string
	SubscriberID string
	SessionID    string // set when counter-too-small triggers full-auth fallback
}

var reauthIdentifierCounter uint32 = 128

func nextReauthIdentifier() int {
	return int(atomic.AddUint32(&reauthIdentifierCounter, 1) % 256)
}

// ReauthHandler handles EAP-AKA fast re-authentication.
type ReauthHandler struct {
	reauthStore        *ReauthStore
	reauthSessionStore *ReauthSessionStore
	orchestrator       *Orchestrator
}

// NewReauthHandler creates a new ReauthHandler.
func NewReauthHandler(reauthStore *ReauthStore, reauthSessionStore *ReauthSessionStore, orchestrator *Orchestrator) *ReauthHandler {
	return &ReauthHandler{
		reauthStore:        reauthStore,
		reauthSessionStore: reauthSessionStore,
		orchestrator:       orchestrator,
	}
}

// HandleReauthRequest handles re-auth RT1.
// Returns nil if re-auth state not found or counter exhausted (caller should fall back).
func (h *ReauthHandler) HandleReauthRequest(ctx context.Context, reauthID string) (*ReauthChallengeResult, error) {
	state, err := h.reauthStore.Get(ctx, reauthID)
	if err != nil {
		return nil, err
	}
	if state == nil {
		slog.Info("Re-auth state not found, falling back to full auth", "reauthId", reauthID)
		return nil, nil
	}

	if state.Counter >= config.MaxReauthCounter {
		slog.Info("Re-auth counter exhausted", "reauthId", reauthID, "counter", state.Counter)
		_ = h.reauthStore.Delete(ctx, reauthID)
		return nil, nil
	}

	kAut, _ := base64.StdEncoding.DecodeString(state.KAut)
	kEncr, _ := base64.StdEncoding.DecodeString(state.KEncr)
	nonceS := make([]byte, 16)
	_, _ = rand.Read(nonceS)
	nextReauthID := GenerateReauthID()
	iv := make([]byte, 16)
	_, _ = rand.Read(iv)
	identifier := nextReauthIdentifier()

	// Build inner attributes
	counterBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(counterBuf, uint16(state.Counter))

	innerAttrs := []EapAttribute{
		{Type: config.ATCounter, Value: counterBuf},
		{Type: config.ATNonceS, Value: nonceS},
		{Type: config.ATNextReauthID, Value: []byte(nextReauthID)},
	}

	ciphertext := EncryptAttributes(kEncr, iv, innerAttrs)

	reauthPacket := EapPacket{
		Code: config.EAPCodeRequest, Identifier: identifier,
		Type: config.EAPTypeAKA, Subtype: config.AKASubtypeReauthentication,
		Attributes: []EapAttribute{
			{Type: config.ATIV, Value: iv},
			{Type: config.ATEncrData, Value: ciphertext},
			{Type: config.ATMac, Value: make([]byte, 16)},
		},
	}

	packetBytes := EncodeEapPacket(reauthPacket)
	mac := ComputeMAC(kAut, packetBytes)
	macOffset := FindATMACOffset(packetBytes)
	copy(packetBytes[macOffset+4:], mac)

	eapRelay := base64.StdEncoding.EncodeToString(packetBytes)

	sessionID, err := h.reauthSessionStore.Create(ctx, &ReauthSessionData{
		ReauthID:     reauthID,
		NextReauthID: nextReauthID,
		NonceS:       base64.StdEncoding.EncodeToString(nonceS),
		Counter:      state.Counter,
		Identifier:   identifier,
		KAut:         state.KAut,
		KEncr:        state.KEncr,
		MK:           state.MK,
		SubscriberID: state.SubscriberID,
		IMSI:         state.IMSI,
	})
	if err != nil {
		return nil, err
	}

	slog.Info("Re-auth challenge sent", "reauthId", reauthID, "sessionId", sessionID, "counter", state.Counter)

	return &ReauthChallengeResult{StatusCode: 401, EapRelay: eapRelay, SessionID: sessionID}, nil
}

// HandleReauthResponse handles re-auth RT2.
func (h *ReauthHandler) HandleReauthResponse(ctx context.Context, eapRelayBase64, sessionID, clientIP string) (*ReauthResult, error) {
	session, err := h.reauthSessionStore.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		slog.Warn("Re-auth session not found or expired", "sessionId", sessionID)
		return reauthFailureResult(0), nil
	}

	rawBytes, err := base64.StdEncoding.DecodeString(eapRelayBase64)
	if err != nil {
		_ = h.reauthSessionStore.Delete(ctx, sessionID)
		return reauthFailureResult(session.Identifier), nil
	}

	packet, err := DecodeEapPacket(rawBytes)
	if err != nil {
		slog.Error("Failed to decode re-auth EAP packet", "err", err, "sessionId", sessionID)
		_ = h.reauthSessionStore.Delete(ctx, sessionID)
		return reauthFailureResult(session.Identifier), nil
	}

	// Handle AUTH_REJECT
	if packet.Subtype == config.AKASubtypeAuthReject {
		slog.Warn("Client sent AUTH_REJECT during re-auth", "sessionId", sessionID)
		_ = h.reauthSessionStore.Delete(ctx, sessionID)
		_ = h.reauthStore.Delete(ctx, session.ReauthID)
		return reauthFailureResult(session.Identifier), nil
	}

	// Expect AKA-Reauthentication
	if packet.Subtype != config.AKASubtypeReauthentication {
		_ = h.reauthSessionStore.Delete(ctx, sessionID)
		return reauthFailureResult(session.Identifier), nil
	}

	atIV := findAttribute(packet.Attributes, config.ATIV)
	atEncrData := findAttribute(packet.Attributes, config.ATEncrData)
	atMac := findAttribute(packet.Attributes, config.ATMac)

	if atIV == nil || atEncrData == nil || atMac == nil {
		_ = h.reauthSessionStore.Delete(ctx, sessionID)
		return reauthFailureResult(session.Identifier), nil
	}

	// Verify MAC BEFORE decryption
	kAut, _ := base64.StdEncoding.DecodeString(session.KAut)
	macOffset := FindATMACOffset(rawBytes)
	if macOffset < 0 || !VerifyMAC(kAut, rawBytes, macOffset+4, atMac.Value) {
		slog.Warn("Re-auth AT_MAC verification failed", "sessionId", sessionID)
		_ = h.reauthSessionStore.Delete(ctx, sessionID)
		return reauthFailureResult(session.Identifier), nil
	}

	// Decrypt inner attributes
	kEncr, _ := base64.StdEncoding.DecodeString(session.KEncr)
	innerAttrs := DecryptAttributes(kEncr, atIV.Value, atEncrData.Value)

	// Check AT_COUNTER_TOO_SMALL → fall back to full auth
	if findAttribute(innerAttrs, config.ATCounterTooSmall) != nil {
		slog.Info("Client sent AT_COUNTER_TOO_SMALL, falling back to full auth",
			"sessionId", sessionID, "reauthId", session.ReauthID)
		_ = h.reauthSessionStore.Delete(ctx, sessionID)
		_ = h.reauthStore.Delete(ctx, session.ReauthID)

		fullAuthResult, err := h.orchestrator.HandleInitialRequest(ctx, session.IMSI)
		if err != nil {
			return nil, err
		}
		return &ReauthResult{
			StatusCode: 401,
			EapRelay:   fullAuthResult.EapRelay,
			SessionID:  fullAuthResult.SessionID,
		}, nil
	}

	// Verify AT_COUNTER
	atCounter := findAttribute(innerAttrs, config.ATCounter)
	if atCounter == nil {
		_ = h.reauthSessionStore.Delete(ctx, sessionID)
		return reauthFailureResult(session.Identifier), nil
	}

	clientCounter := int(binary.BigEndian.Uint16(atCounter.Value))
	if clientCounter != session.Counter {
		slog.Warn("Re-auth counter mismatch", "expected", session.Counter, "got", clientCounter)
		_ = h.reauthSessionStore.Delete(ctx, sessionID)
		_ = h.reauthStore.Delete(ctx, session.ReauthID)
		return reauthFailureResult(session.Identifier), nil
	}

	// Derive new session keys
	mk, _ := base64.StdEncoding.DecodeString(session.MK)
	nonceS, _ := base64.StdEncoding.DecodeString(session.NonceS)
	DeriveReauthKeys(session.NextReauthID, session.Counter, nonceS, mk)

	// Rotate re-auth state
	_ = h.reauthStore.Delete(ctx, session.ReauthID)
	err = h.reauthStore.Store(ctx, &ReauthState{
		SubscriberID: session.SubscriberID,
		IMSI:         session.IMSI,
		MK:           session.MK,
		KAut:         session.KAut,
		KEncr:        session.KEncr,
		Counter:      session.Counter + 1,
		Identity:     session.NextReauthID,
	})
	if err != nil {
		return nil, err
	}

	_ = h.reauthSessionStore.Delete(ctx, sessionID)

	slog.Info("Re-auth successful",
		"reauthId", session.ReauthID, "nextReauthId", session.NextReauthID,
		"counter", session.Counter, "subscriberId", session.SubscriberID)

	return &ReauthResult{
		StatusCode:   200,
		ReauthID:     session.NextReauthID,
		SubscriberID: session.SubscriberID,
		EapRelay:     EncodeEapToBase64(EapPacket{Code: config.EAPCodeSuccess, Identifier: session.Identifier}),
	}, nil
}

func reauthFailureResult(identifier int) *ReauthResult {
	return &ReauthResult{
		StatusCode: 401,
		EapRelay:   EncodeEapToBase64(EapPacket{Code: config.EAPCodeFailure, Identifier: identifier}),
	}
}
