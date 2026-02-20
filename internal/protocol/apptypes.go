package protocol

// ODSA operation names per GSMA TS.43.
const (
	// OpCheckEligibility is the ODSA operation name for checking device/subscription eligibility per GSMA TS.43.
	OpCheckEligibility = "CheckEligibility"
	// OpManageSubscription is the ODSA operation name for managing eSIM subscription lifecycle per GSMA TS.43.
	OpManageSubscription = "ManageSubscription"
	// OpManageService is the ODSA operation name for enabling or disabling a service per GSMA TS.43.
	OpManageService = "ManageService"
	// OpAcquireConfiguration is the ODSA operation name for retrieving service configuration per GSMA TS.43.
	OpAcquireConfiguration = "AcquireConfiguration"
	// OpAcquireTemporaryToken is the ODSA operation name for obtaining a temporary token per GSMA TS.43.
	OpAcquireTemporaryToken = "AcquireTemporaryToken"
	// OpGetOperatorToken is the ODSA operation name for obtaining an operator-scoped token per GSMA TS.43.
	OpGetOperatorToken = "GetOperatorToken"
	// OpAcquirePlan is the ODSA operation name for acquiring a data plan per GSMA TS.43.
	OpAcquirePlan = "AcquirePlan"
)
