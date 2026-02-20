package protocol

// Entitlement status (per-service).
const (
	// EntitlementStatusDisabled is the entitlement status indicating the service is disabled per GSMA TS.43.
	EntitlementStatusDisabled = 0
	// EntitlementStatusEnabled is the entitlement status indicating the service is enabled per GSMA TS.43.
	EntitlementStatusEnabled = 1
	// EntitlementStatusIncompatible is the entitlement status indicating the device is incompatible with the service per GSMA TS.43.
	EntitlementStatusIncompatible = 2
	// EntitlementStatusProvisioning is the entitlement status indicating the service is being provisioned per GSMA TS.43.
	EntitlementStatusProvisioning = 3
)

// Service status.
const (
	// ServiceStatusNotConfigured is the service status indicating the service has not been configured per GSMA TS.43.
	ServiceStatusNotConfigured = 0
	// ServiceStatusEnabled is the service status indicating the service is active and configured per GSMA TS.43.
	ServiceStatusEnabled = 1
	// ServiceStatusDisabled is the service status indicating the service is configured but disabled per GSMA TS.43.
	ServiceStatusDisabled = 2
	// ServiceStatusNotSubscribed is the service status indicating the subscriber is not subscribed to the service per GSMA TS.43.
	ServiceStatusNotSubscribed = 3
)

// ODSA subscription result (TS.43 numeric codes).
const (
	// SubscriptionResultContinueToWS is the ODSA result code directing the client to continue via a web session per GSMA TS.43.
	SubscriptionResultContinueToWS = 1
	// SubscriptionResultDownloadProfile is the ODSA result code directing the client to download an eSIM profile per GSMA TS.43.
	SubscriptionResultDownloadProfile = 2
	// SubscriptionResultDone is the ODSA result code indicating the operation completed successfully per GSMA TS.43.
	SubscriptionResultDone = 3
	// SubscriptionResultDelayedDownload is the ODSA result code indicating the profile download is deferred per GSMA TS.43.
	SubscriptionResultDelayedDownload = 4
	// SubscriptionResultDeleteProfileInUse is the ODSA result code directing the client to delete the active profile per GSMA TS.43.
	SubscriptionResultDeleteProfileInUse = 6
	// SubscriptionResultRequiresUserInput is the ODSA result code indicating additional user input is needed per GSMA TS.43.
	SubscriptionResultRequiresUserInput = 7
)

// Terms & Conditions status.
const (
	// TcStatusNotProvided is the T&C status indicating no terms and conditions were provided per GSMA TS.43.
	TcStatusNotProvided = 0
	// TcStatusAccepted is the T&C status indicating the subscriber accepted the terms and conditions per GSMA TS.43.
	TcStatusAccepted = 1
	// TcStatusRequiresAcceptance is the T&C status indicating the subscriber must accept the terms and conditions per GSMA TS.43.
	TcStatusRequiresAcceptance = 2
	// TcStatusRejected is the T&C status indicating the subscriber rejected the terms and conditions per GSMA TS.43.
	TcStatusRejected = 3
)

// Provisioning status.
const (
	// ProvStatusNotNeeded is the provisioning status indicating no provisioning is required per GSMA TS.43.
	ProvStatusNotNeeded = 0
	// ProvStatusRequired is the provisioning status indicating provisioning is required before the service can be used per GSMA TS.43.
	ProvStatusRequired = 1
	// ProvStatusInProgress is the provisioning status indicating provisioning is currently in progress per GSMA TS.43.
	ProvStatusInProgress = 2
	// ProvStatusComplete is the provisioning status indicating provisioning has completed successfully per GSMA TS.43.
	ProvStatusComplete = 3
)
