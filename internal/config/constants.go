package config

// EAP codes (RFC 3748)
const (
	EAPCodeRequest  = 1
	EAPCodeResponse = 2
	EAPCodeSuccess  = 3
	EAPCodeFailure  = 4
)

// EAP type
const EAPTypeAKA = 23

// AKA subtypes (RFC 4187)
const (
	AKASubtypeChallenge        = 1
	AKASubtypeAuthReject       = 2
	AKASubtypeSyncFailure      = 4
	AKASubtypeIdentity         = 5
	AKASubtypeReauthentication = 13
)

// EAP-AKA attribute types (RFC 4187 Section 11)
const (
	ATRand            = 1
	ATAutn            = 2
	ATRes             = 3
	ATAuts            = 4
	ATPadding         = 6
	ATMac             = 11
	ATNextReauthID    = 14
	ATCounter         = 19
	ATCounterTooSmall = 20
	ATNonceS          = 21
	ATIV              = 129
	ATEncrData        = 130
)

// EAP-AKA protocol constants
const (
	SessionTTLSeconds       = 90
	ReauthSessionTTLSeconds = 90
	IdempotencyTTLSeconds   = 90
	ReauthStoreTTLSeconds   = 172800 // 48 hours
	MaxReauthCounter        = 65535
)

// EAP-AKA session states
const (
	EAPStateIdle          = "IDLE"
	EAPStateChallengeSent = "CHALLENGE_SENT"
	EAPStateReauthSent    = "REAUTH_SENT"
	EAPStateSuccess       = "SUCCESS"
	EAPStateFailure       = "FAILURE"
	EAPStateSyncFailure   = "SYNC_FAILURE"
)

// Token types
const (
	TokenTypeAuth     = "auth"
	TokenTypeFastAuth = "fast_auth"
	TokenTypeTemp     = "temporary"
	TokenTypeOperator = "operator"
)

// AppID values from GSMA TS.43
const (
	AppIDVoWiFi               = "ap2004"
	AppIDVoLTE                = "ap2003"
	AppIDSMSoIP               = "ap2005"
	AppIDODSACompanion        = "ap2006"
	AppIDODSAPrimary          = "ap2009"
	AppIDDataPlanInfo         = "ap2010"
	AppIDServerInitiatedODSA  = "ap2011"
	AppIDDirectCarrierBilling = "ap2012"
	AppIDPrivateUserIdentity  = "ap2013"
	AppIDDeviceUserInfo       = "ap2014"
	AppIDAppAuthentication    = "ap2015"
	AppIDSatelliteMode        = "ap2016"
)

// AllAppIDs lists all supported application IDs.
var AllAppIDs = []string{
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
