package config

// EAP codes (RFC 3748)
const (
	// EAPCodeRequest is the EAP Request code per RFC 3748 Section 4.
	EAPCodeRequest = 1
	// EAPCodeResponse is the EAP Response code per RFC 3748 Section 4.
	EAPCodeResponse = 2
	// EAPCodeSuccess is the EAP Success code per RFC 3748 Section 4.
	EAPCodeSuccess = 3
	// EAPCodeFailure is the EAP Failure code per RFC 3748 Section 4.
	EAPCodeFailure = 4
)

// EAPTypeAKA is the EAP method type for AKA (23) per RFC 4187 Section 11.
const EAPTypeAKA = 23

// AKA subtypes (RFC 4187)
const (
	// AKASubtypeChallenge is the AKA-Challenge subtype (1) per RFC 4187 Section 1.
	AKASubtypeChallenge = 1
	// AKASubtypeAuthReject is the AKA-Authentication-Reject subtype (2) per RFC 4187 Section 1.
	AKASubtypeAuthReject = 2
	// AKASubtypeSyncFailure is the AKA-Synchronization-Failure subtype (4) per RFC 4187 Section 1.
	AKASubtypeSyncFailure = 4
	// AKASubtypeIdentity is the AKA-Identity subtype (5) per RFC 4187 Section 1.
	AKASubtypeIdentity = 5
	// AKASubtypeReauthentication is the AKA-Reauthentication subtype (13) per RFC 4187 Section 1.
	AKASubtypeReauthentication = 13
)

// EAP-AKA attribute types (RFC 4187 Section 11)
const (
	// ATRand is the AT_RAND attribute type (1) carrying the RAND challenge per RFC 4187 Section 11.
	ATRand = 1
	// ATAutn is the AT_AUTN attribute type (2) carrying the authentication token per RFC 4187 Section 11.
	ATAutn = 2
	// ATRes is the AT_RES attribute type (3) carrying the authentication response per RFC 4187 Section 11.
	ATRes = 3
	// ATAuts is the AT_AUTS attribute type (4) carrying the synchronization token per RFC 4187 Section 11.
	ATAuts = 4
	// ATPadding is the AT_PADDING attribute type (6) for alignment padding per RFC 4187 Section 11.
	ATPadding = 6
	// ATMac is the AT_MAC attribute type (11) carrying the message authentication code per RFC 4187 Section 11.
	ATMac = 11
	// ATNextReauthID is the AT_NEXT_REAUTH_ID attribute type (14) carrying the next fast re-authentication identity per RFC 4187 Section 11.
	ATNextReauthID = 14
	// ATCounter is the AT_COUNTER attribute type (19) carrying the re-authentication counter per RFC 4187 Section 11.
	ATCounter = 19
	// ATCounterTooSmall is the AT_COUNTER_TOO_SMALL attribute type (20) indicating the server counter is stale per RFC 4187 Section 11.
	ATCounterTooSmall = 20
	// ATNonceS is the AT_NONCE_S attribute type (21) carrying the server nonce for re-authentication per RFC 4187 Section 11.
	ATNonceS = 21
	// ATIV is the AT_IV attribute type (129) carrying the initialization vector for encrypted data per RFC 4187 Section 11.
	ATIV = 129
	// ATEncrData is the AT_ENCR_DATA attribute type (130) carrying encrypted attribute data per RFC 4187 Section 11.
	ATEncrData = 130
)

// EAP-AKA protocol constants
const (
	// SessionTTLSeconds is the time-to-live in seconds for EAP-AKA challenge sessions.
	SessionTTLSeconds = 90
	// ReauthSessionTTLSeconds is the time-to-live in seconds for fast re-authentication sessions.
	ReauthSessionTTLSeconds = 90
	// IdempotencyTTLSeconds is the time-to-live in seconds for idempotency cache entries.
	IdempotencyTTLSeconds = 90
	// ReauthStoreTTLSeconds is the time-to-live in seconds for re-authentication key material (48 hours).
	ReauthStoreTTLSeconds = 172800
	// MaxReauthCounter is the maximum value for the re-authentication counter per RFC 4187 Section 5.6.
	MaxReauthCounter = 65535
)

// EAP-AKA session states
const (
	// EAPStateIdle is the initial session state before authentication begins.
	EAPStateIdle = "IDLE"
	// EAPStateChallengeSent is the session state after an AKA-Challenge has been sent to the peer.
	EAPStateChallengeSent = "CHALLENGE_SENT"
	// EAPStateReauthSent is the session state after a re-authentication request has been sent to the peer.
	EAPStateReauthSent = "REAUTH_SENT"
	// EAPStateSuccess is the session state after successful EAP-AKA authentication.
	EAPStateSuccess = "SUCCESS"
	// EAPStateFailure is the session state after a failed EAP-AKA authentication.
	EAPStateFailure = "FAILURE"
	// EAPStateSyncFailure is the session state after an AKA synchronization failure per RFC 4187 Section 6.3.1.
	EAPStateSyncFailure = "SYNC_FAILURE"
)

// Token types
const (
	// TokenTypeAuth is the token type for standard EAP-AKA authentication tokens.
	TokenTypeAuth = "auth"
	// TokenTypeFastAuth is the token type for fast re-authentication tokens per RFC 4187.
	TokenTypeFastAuth = "fast_auth"
	// TokenTypeTemp is the token type for temporary tokens per GSMA TS.43 AcquireTemporaryToken.
	TokenTypeTemp = "temporary"
	// TokenTypeOperator is the token type for operator tokens per GSMA TS.43 GetOperatorToken.
	TokenTypeOperator = "operator"
)

// AppID values from GSMA TS.43
const (
	// AppIDVoWiFi identifies the Wi-Fi Calling (VoWiFi) entitlement service per GSMA TS.43.
	AppIDVoWiFi = "ap2004"
	// AppIDVoLTE identifies the Voice over LTE (VoLTE) entitlement service per GSMA TS.43.
	AppIDVoLTE = "ap2003"
	// AppIDSMSoIP identifies the SMS over IP (SMSoIP) entitlement service per GSMA TS.43.
	AppIDSMSoIP = "ap2005"
	// AppIDODSACompanion identifies the ODSA companion device entitlement service per GSMA TS.43.
	AppIDODSACompanion = "ap2006"
	// AppIDODSAPrimary identifies the ODSA primary device entitlement service per GSMA TS.43.
	AppIDODSAPrimary = "ap2009"
	// AppIDDataPlanInfo identifies the data plan information service per GSMA TS.43.
	AppIDDataPlanInfo = "ap2010"
	// AppIDServerInitiatedODSA identifies the server-initiated ODSA service per GSMA TS.43.
	AppIDServerInitiatedODSA = "ap2011"
	// AppIDDirectCarrierBilling identifies the direct carrier billing entitlement service per GSMA TS.43.
	AppIDDirectCarrierBilling = "ap2012"
	// AppIDPrivateUserIdentity identifies the private user identity service per GSMA TS.43.
	AppIDPrivateUserIdentity = "ap2013"
	// AppIDDeviceUserInfo identifies the device and user information service per GSMA TS.43.
	AppIDDeviceUserInfo = "ap2014"
	// AppIDAppAuthentication identifies the application-level authentication service per GSMA TS.43.
	AppIDAppAuthentication = "ap2015"
	// AppIDSatelliteMode identifies the satellite mode entitlement service per GSMA TS.43.
	AppIDSatelliteMode = "ap2016"
)

// AllAppIDs returns a copy of all supported application IDs.
func AllAppIDs() []string {
	return []string{
		AppIDVoLTE,
		AppIDVoWiFi,
		AppIDSMSoIP,
		AppIDODSACompanion,
		AppIDODSAPrimary,
		AppIDDataPlanInfo,
		AppIDServerInitiatedODSA,
		AppIDDirectCarrierBilling,
		AppIDPrivateUserIdentity,
		AppIDDeviceUserInfo,
		AppIDAppAuthentication,
		AppIDSatelliteMode,
	}
}
