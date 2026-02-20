package eapaka

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/config"
)

// ChallengeResult is returned from RT1 (initial challenge).
type ChallengeResult struct {
	StatusCode int
	EapRelay   string
	SessionID  string
}

// AuthResult is returned from RT2 (response verification).
type AuthResult struct {
	StatusCode   int
	Token        string
	EapRelay     string
	SubscriberID string
	SessionID    string // set when resync issues a new challenge
}

// Orchestrator handles EAP-AKA authentication flows.
type Orchestrator struct {
	hssClient        *HSSClient
	sessionStore     *SessionStore
	reauthStore      *ReauthStore
	subscriberLookup func(ctx context.Context, imsi string) (string, error) // returns subscriberID
}

// NewOrchestrator creates a new EAP-AKA orchestrator.
func NewOrchestrator(
	hssClient *HSSClient,
	sessionStore *SessionStore,
	reauthStore *ReauthStore,
	subscriberLookup func(ctx context.Context, imsi string) (string, error),
) *Orchestrator {
	return &Orchestrator{
		hssClient:        hssClient,
		sessionStore:     sessionStore,
		reauthStore:      reauthStore,
		subscriberLookup: subscriberLookup,
	}
}

// HandleInitialRequest handles RT1: IMSI → EAP-Request/AKA-Challenge.
func (o *Orchestrator) HandleInitialRequest(ctx context.Context, imsi string) (*ChallengeResult, error) {
	vectors, err := o.hssClient.FetchVectors(ctx, imsi)
	if err != nil {
		return nil, fmt.Errorf("fetch HSS vectors: %w", err)
	}

	identity := BuildIdentity(imsi)
	mk := DeriveMasterKey(identity, vectors.IK, vectors.CK)
	keys := DeriveKeys(identity, vectors.IK, vectors.CK)
	identifier := nextIdentifier()

	// Build EAP-Request/AKA-Challenge with zeroed MAC
	challengePacket := EapPacket{
		Code: config.EAPCodeRequest, Identifier: identifier,
		Type: config.EAPTypeAKA, Subtype: config.AKASubtypeChallenge,
		Attributes: []EapAttribute{
			{Type: config.ATRand, Value: vectors.RAND},
			{Type: config.ATAutn, Value: vectors.AUTN},
			{Type: config.ATMac, Value: make([]byte, 16)},
		},
	}

	packetBytes := EncodeEapPacket(challengePacket)
	mac := ComputeMAC(keys.KAut, packetBytes)
	macOffset := FindATMACOffset(packetBytes)
	copy(packetBytes[macOffset+4:], mac)

	eapRelay := base64.StdEncoding.EncodeToString(packetBytes)

	sessionID, err := o.sessionStore.Create(ctx, &EapSessionData{
		IMSI:       imsi,
		State:      config.EAPStateChallengeSent,
		RAND:       vectors.RAND,
		XRES:       vectors.XRES,
		CK:         vectors.CK,
		IK:         vectors.IK,
		Identifier: identifier,
		KAut:       keys.KAut,
		KEncr:      keys.KEncr,
		MK:         mk,
	})
	if err != nil {
		return nil, fmt.Errorf("create EAP session: %w", err)
	}

	slog.Info("EAP-AKA challenge sent", "imsi", imsi, "sessionId", sessionID)

	return &ChallengeResult{StatusCode: 401, EapRelay: eapRelay, SessionID: sessionID}, nil
}

// HandleEapResponse handles RT2: EAP-Response verification.
func (o *Orchestrator) HandleEapResponse(ctx context.Context, eapRelayBase64, sessionID string) (*AuthResult, error) {
	session, err := o.sessionStore.Get(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get EAP session: %w", err)
	}
	if session == nil {
		slog.Warn("EAP session not found or expired", "sessionId", sessionID)
		return eapFailureResult(0), nil
	}

	if session.State != config.EAPStateChallengeSent {
		slog.Warn("EAP session in unexpected state", "sessionId", sessionID, "state", session.State)
		_ = o.sessionStore.Delete(ctx, sessionID)
		return eapFailureResult(session.Identifier), nil
	}

	rawBytes, err := base64.StdEncoding.DecodeString(eapRelayBase64)
	if err != nil {
		_ = o.sessionStore.Delete(ctx, sessionID)
		return eapFailureResult(session.Identifier), nil
	}

	packet, err := DecodeEapPacket(rawBytes)
	if err != nil {
		slog.Error("Failed to decode EAP packet", "err", err, "sessionId", sessionID)
		_ = o.sessionStore.Delete(ctx, sessionID)
		return eapFailureResult(session.Identifier), nil
	}

	// Handle AUTH_REJECT
	if packet.Subtype == config.AKASubtypeAuthReject {
		slog.Warn("Client sent AUTH_REJECT", "sessionId", sessionID)
		_ = o.sessionStore.Delete(ctx, sessionID)
		return eapFailureResult(session.Identifier), nil
	}

	// Handle SYNC_FAILURE
	if packet.Subtype == config.AKASubtypeSyncFailure {
		return o.handleSyncFailure(ctx, packet, session, sessionID)
	}

	// Expect AKA-Challenge response
	if packet.Subtype != config.AKASubtypeChallenge {
		slog.Warn("Unexpected EAP subtype", "sessionId", sessionID, "subtype", packet.Subtype)
		_ = o.sessionStore.Delete(ctx, sessionID)
		return eapFailureResult(session.Identifier), nil
	}

	atRes := findAttribute(packet.Attributes, config.ATRes)
	atMac := findAttribute(packet.Attributes, config.ATMac)
	if atRes == nil || atMac == nil {
		_ = o.sessionStore.Delete(ctx, sessionID)
		return eapFailureResult(session.Identifier), nil
	}

	// Verify AT_RES == XRES (timing-safe)
	if len(atRes.Value) != len(session.XRES) || subtle.ConstantTimeCompare(atRes.Value, session.XRES) != 1 {
		slog.Warn("AT_RES mismatch", "sessionId", sessionID, "imsi", session.IMSI)
		_ = o.sessionStore.Delete(ctx, sessionID)
		return eapFailureResult(session.Identifier), nil
	}

	// Verify AT_MAC
	macOffset := FindATMACOffset(rawBytes)
	if macOffset < 0 || !VerifyMAC(session.KAut, rawBytes, macOffset+4, atMac.Value) {
		slog.Warn("AT_MAC verification failed", "sessionId", sessionID)
		_ = o.sessionStore.Delete(ctx, sessionID)
		return eapFailureResult(session.Identifier), nil
	}

	// Look up subscriber
	subscriberID, err := o.subscriberLookup(ctx, session.IMSI)
	if err != nil || subscriberID == "" {
		slog.Error("Subscriber not found after successful auth", "imsi", session.IMSI)
		_ = o.sessionStore.Delete(ctx, sessionID)
		return eapFailureResult(session.Identifier), nil
	}

	// Issue re-auth identity
	reauthID, err := GenerateReauthID()
	if err != nil {
		return nil, fmt.Errorf("generate reauth ID: %w", err)
	}
	err = o.reauthStore.Store(ctx, &ReauthState{
		SubscriberID: subscriberID,
		IMSI:         session.IMSI,
		MK:           base64.StdEncoding.EncodeToString(session.MK),
		KAut:         base64.StdEncoding.EncodeToString(session.KAut),
		KEncr:        base64.StdEncoding.EncodeToString(session.KEncr),
		Counter:      1,
		Identity:     reauthID,
	})
	if err != nil {
		return nil, fmt.Errorf("store reauth state: %w", err)
	}

	_ = o.sessionStore.Delete(ctx, sessionID)

	slog.Info("EAP-AKA authentication successful", "imsi", session.IMSI, "sessionId", sessionID, "subscriberId", subscriberID)

	return &AuthResult{
		StatusCode:   200,
		Token:        reauthID,
		SubscriberID: subscriberID,
		EapRelay:     EncodeEapToBase64(EapPacket{Code: config.EAPCodeSuccess, Identifier: session.Identifier}),
	}, nil
}

func (o *Orchestrator) handleSyncFailure(ctx context.Context, packet EapPacket, session *EapSessionData, sessionID string) (*AuthResult, error) {
	slog.Info("Client sent SYNC_FAILURE, attempting resync", "sessionId", sessionID, "imsi", session.IMSI)

	atAuts := findAttribute(packet.Attributes, config.ATAuts)
	if atAuts == nil || len(atAuts.Value) != 14 {
		_ = o.sessionStore.Delete(ctx, sessionID)
		return eapFailureResult(session.Identifier), nil
	}

	result := o.hssClient.ResyncVectors(ctx, session.IMSI, session.RAND, atAuts.Value)
	if !result.Success || result.Vectors == nil {
		slog.Warn("SQN resync failed", "sessionId", sessionID, "error", result.Error)
		_ = o.sessionStore.Delete(ctx, sessionID)
		return eapFailureResult(session.Identifier), nil
	}

	vectors := result.Vectors
	identity := BuildIdentity(session.IMSI)
	mk := DeriveMasterKey(identity, vectors.IK, vectors.CK)
	keys := DeriveKeys(identity, vectors.IK, vectors.CK)
	newIdentifier := nextIdentifier()

	challengePacket := EapPacket{
		Code: config.EAPCodeRequest, Identifier: newIdentifier,
		Type: config.EAPTypeAKA, Subtype: config.AKASubtypeChallenge,
		Attributes: []EapAttribute{
			{Type: config.ATRand, Value: vectors.RAND},
			{Type: config.ATAutn, Value: vectors.AUTN},
			{Type: config.ATMac, Value: make([]byte, 16)},
		},
	}

	packetBytes := EncodeEapPacket(challengePacket)
	mac := ComputeMAC(keys.KAut, packetBytes)
	macOffset := FindATMACOffset(packetBytes)
	copy(packetBytes[macOffset+4:], mac)

	eapRelay := base64.StdEncoding.EncodeToString(packetBytes)

	_ = o.sessionStore.Update(ctx, sessionID, &EapSessionData{
		RAND: vectors.RAND, XRES: vectors.XRES, CK: vectors.CK, IK: vectors.IK,
		Identifier: newIdentifier, KAut: keys.KAut, KEncr: keys.KEncr, MK: mk,
	})

	slog.Info("SQN resync successful, new challenge issued", "sessionId", sessionID, "imsi", session.IMSI)

	return &AuthResult{StatusCode: 401, EapRelay: eapRelay, SessionID: sessionID}, nil
}

// FindATMACOffset finds the byte offset of AT_MAC in raw packet bytes.
func FindATMACOffset(buf []byte) int {
	offset := 8
	for offset+2 <= len(buf) {
		attrType := int(buf[offset])
		attrLen := int(buf[offset+1]) * 4
		if attrType == config.ATMac {
			return offset
		}
		offset += attrLen
	}
	return -1
}

var _identifierCounter uint32

func nextIdentifier() int {
	return int(atomic.AddUint32(&_identifierCounter, 1) % 256)
}

func findAttribute(attributes []EapAttribute, attrType int) *EapAttribute {
	for i := range attributes {
		if attributes[i].Type == attrType {
			return &attributes[i]
		}
	}
	return nil
}

func eapFailureResult(identifier int) *AuthResult {
	return &AuthResult{
		StatusCode: 401,
		EapRelay:   EncodeEapToBase64(EapPacket{Code: config.EAPCodeFailure, Identifier: identifier}),
	}
}
