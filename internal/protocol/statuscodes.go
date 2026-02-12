package protocol

// Entitlement status (per-service).
const (
	EntitlementStatusDisabled     = 0
	EntitlementStatusEnabled      = 1
	EntitlementStatusIncompatible = 2
	EntitlementStatusProvisioning = 3
)

// Service status.
const (
	ServiceStatusNotConfigured = 0
	ServiceStatusEnabled       = 1
	ServiceStatusDisabled      = 2
	ServiceStatusNotSubscribed = 3
)

// ODSA subscription result (TS.43 numeric codes).
const (
	SubscriptionResultContinueToWS       = 1
	SubscriptionResultDownloadProfile    = 2
	SubscriptionResultDone               = 3
	SubscriptionResultDelayedDownload    = 4
	SubscriptionResultDeleteProfileInUse = 6
	SubscriptionResultRequiresUserInput  = 7
)

// Terms & Conditions status.
const (
	TcStatusNotProvided        = 0
	TcStatusAccepted           = 1
	TcStatusRequiresAcceptance = 2
	TcStatusRejected           = 3
)

// Provisioning status.
const (
	ProvStatusNotNeeded  = 0
	ProvStatusRequired   = 1
	ProvStatusInProgress = 2
	ProvStatusComplete   = 3
)
