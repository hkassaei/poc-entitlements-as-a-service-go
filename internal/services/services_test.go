package services

import (
	"strconv"
	"testing"

	"github.com/hkassaei/entitlements-as-a-service-poc-go/internal/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mock SM-DP+
// ---------------------------------------------------------------------------

func TestGetActivationCode_Default(t *testing.T) {
	result := GetActivationCode("default")
	assert.Equal(t, "postpaid", result.ProfileType)
	assert.Equal(t, "smdp.operator.com", result.SMDPAddress)
	assert.Contains(t, result.ActivationCode, "smdp.operator.com")
	assert.NotEmpty(t, result.ICCID)
	assert.NotEmpty(t, result.MatchingID)
}

func TestGetActivationCode_UnknownFallsBackToDefault(t *testing.T) {
	result := GetActivationCode("unknown")
	expected := GetActivationCode("default")
	assert.Equal(t, expected, result)
}

func TestListAvailableProfiles(t *testing.T) {
	profiles := ListAvailableProfiles()
	assert.Len(t, profiles, 3)
	types := make([]string, len(profiles))
	for i, p := range profiles {
		types[i] = p.ProfileType
	}
	assert.Contains(t, types, "postpaid")
	assert.Contains(t, types, "prepaid")
	assert.Contains(t, types, "companion")
}

// ---------------------------------------------------------------------------
// VoWiFi (ap2004)
// ---------------------------------------------------------------------------

func TestVoWiFi_EnabledReturnsAddresses(t *testing.T) {
	config := BuildVoWiFiConfig(1, 3, 1, nil)
	assert.Equal(t, "ap2004", config.AppID)
	assert.Equal(t, 1, config.EntitlementStatus)
	assert.Equal(t, 1, *config.AddrStatus)
	require.NotEmpty(t, config.Addresses)
	assert.True(t, len(config.Addresses) > 0)
}

func TestVoWiFi_DisabledOmitsAddresses(t *testing.T) {
	config := BuildVoWiFiConfig(0, 0, 0, nil)
	assert.Equal(t, 0, *config.AddrStatus)
	assert.Empty(t, config.Addresses)
}

func TestVoWiFi_CustomAddresses(t *testing.T) {
	configData := &VoWiFiConfigData{
		Addresses: []protocol.AddressConfig{{AddrType: "2", Addr: "10.0.0.1"}},
	}
	config := BuildVoWiFiConfig(1, 3, 1, configData)
	assert.Equal(t, []protocol.AddressConfig{{AddrType: "2", Addr: "10.0.0.1"}}, config.Addresses)
}

func TestVoWiFi_ServiceFlowURLWhenTCRequiresAcceptance(t *testing.T) {
	configData := &VoWiFiConfigData{ServiceFlowURL: "https://operator.com/terms"}
	config := BuildVoWiFiConfig(0, 1, 2, configData)
	assert.Equal(t, "https://operator.com/terms", config.ServiceFlowURL)
}

// ---------------------------------------------------------------------------
// VoLTE (ap2003)
// ---------------------------------------------------------------------------

func TestVoLTE_EnabledWithExtraParams(t *testing.T) {
	config := BuildVoLTEConfig(1, 3, 1, nil)
	assert.Equal(t, "ap2003", config.AppID)
	assert.Equal(t, "1", config.ExtraParams["VoLTE_Entitled"])
	assert.Equal(t, "1", config.ExtraParams["VoNR_Entitled"])
	require.NotEmpty(t, config.Addresses)
}

func TestVoLTE_DisabledZerosExtraParams(t *testing.T) {
	config := BuildVoLTEConfig(0, 0, 0, nil)
	assert.Equal(t, "0", config.ExtraParams["VoLTE_Entitled"])
	assert.Equal(t, "0", config.ExtraParams["VoNR_Entitled"])
}

// ---------------------------------------------------------------------------
// SMSoIP (ap2005)
// ---------------------------------------------------------------------------

func TestSmsOip_EnabledReturnsAddresses(t *testing.T) {
	config := BuildSmsOipConfig(1, 0, 0, nil)
	assert.Equal(t, "ap2005", config.AppID)
	assert.Equal(t, 1, *config.AddrStatus)
	require.NotEmpty(t, config.Addresses)
}

func TestSmsOip_DisabledOmitsAddresses(t *testing.T) {
	config := BuildSmsOipConfig(0, 0, 0, nil)
	assert.Equal(t, 0, *config.AddrStatus)
	assert.Empty(t, config.Addresses)
}

// ---------------------------------------------------------------------------
// ODSA Companion (ap2006)
// ---------------------------------------------------------------------------

func TestCompanion_CheckEligibility_Eligible(t *testing.T) {
	config := BuildCompanionConfig(1, 0, 0, &OdsaConfigData{SubscriptionState: "eligible"}, &OdsaContext{Operation: "CheckEligibility"})
	assert.Equal(t, "ap2006", config.AppID)
	assert.Equal(t, 1, config.EntitlementStatus)
	assert.Equal(t, strconv.Itoa(protocol.SubscriptionResultDone), config.ExtraParams["SubscriptionResult"])
}

func TestCompanion_CheckEligibility_NoState(t *testing.T) {
	config := BuildCompanionConfig(1, 0, 0, &OdsaConfigData{}, &OdsaContext{Operation: "CheckEligibility"})
	assert.Equal(t, 0, config.EntitlementStatus)
	assert.Equal(t, strconv.Itoa(protocol.SubscriptionResultDone), config.ExtraParams["SubscriptionResult"])
}

func TestCompanion_ManageSubscription_ServiceFlow(t *testing.T) {
	config := BuildCompanionConfig(1, 0, 0, &OdsaConfigData{
		SubscriptionState: "eligible",
		ServiceFlowURL:    "https://operator.com/setup",
	}, &OdsaContext{Operation: "ManageSubscription"})
	assert.Equal(t, strconv.Itoa(protocol.SubscriptionResultContinueToWS), config.ExtraParams["SubscriptionResult"])
	assert.Equal(t, "https://operator.com/setup", config.ExtraParams["ServiceFlow_URL"])
}

func TestCompanion_ManageSubscription_DownloadProfile(t *testing.T) {
	config := BuildCompanionConfig(1, 0, 0, &OdsaConfigData{
		SubscriptionState: "active",
		SMDPAddress:       "smdp.operator.com",
		ProfileType:       "companion",
	}, &OdsaContext{Operation: "ManageSubscription"})
	assert.Equal(t, strconv.Itoa(protocol.SubscriptionResultDownloadProfile), config.ExtraParams["SubscriptionResult"])
	assert.Equal(t, "smdp.operator.com", config.ExtraParams["SMDP+Address"])
	assert.Contains(t, config.ExtraParams["SMDP+ActivationCode"], "smdp.operator.com")
	assert.NotEmpty(t, config.ExtraParams["ProfileICCID"])
}

func TestCompanion_ManageSubscription_ActiveNoSMDP(t *testing.T) {
	config := BuildCompanionConfig(1, 0, 0, &OdsaConfigData{SubscriptionState: "active"}, &OdsaContext{Operation: "ManageSubscription"})
	assert.Equal(t, strconv.Itoa(protocol.SubscriptionResultDone), config.ExtraParams["SubscriptionResult"])
}

func TestCompanion_ManageService(t *testing.T) {
	config := BuildCompanionConfig(1, 3, 1, &OdsaConfigData{}, &OdsaContext{Operation: "ManageService"})
	assert.Equal(t, strconv.Itoa(protocol.SubscriptionResultDone), config.ExtraParams["SubscriptionResult"])
	assert.Equal(t, 1, config.EntitlementStatus)
}

func TestCompanion_AcquireConfiguration_SMDPAddress(t *testing.T) {
	config := BuildCompanionConfig(1, 0, 0, &OdsaConfigData{SMDPAddress: "smdp.operator.com"}, &OdsaContext{Operation: "AcquireConfiguration"})
	assert.Equal(t, "smdp.operator.com", config.ExtraParams["SMDP+Address"])
}

func TestCompanion_NoOperation(t *testing.T) {
	config := BuildCompanionConfig(1, 0, 0, &OdsaConfigData{}, nil)
	assert.Equal(t, "ap2006", config.AppID)
	assert.Equal(t, 1, config.EntitlementStatus)
	assert.Nil(t, config.ExtraParams)
}

// ---------------------------------------------------------------------------
// ODSA Primary (ap2009)
// ---------------------------------------------------------------------------

func TestPrimary_CheckEligibility_Eligible(t *testing.T) {
	config := BuildPrimaryConfig(1, 0, 0, &OdsaConfigData{SubscriptionState: "eligible"}, &OdsaContext{Operation: "CheckEligibility"})
	assert.Equal(t, "ap2009", config.AppID)
	assert.Equal(t, 1, config.EntitlementStatus)
	assert.Equal(t, strconv.Itoa(protocol.SubscriptionResultDone), config.ExtraParams["SubscriptionResult"])
}

func TestPrimary_CheckEligibility_NoState(t *testing.T) {
	config := BuildPrimaryConfig(1, 0, 0, &OdsaConfigData{}, &OdsaContext{Operation: "CheckEligibility"})
	assert.Equal(t, 0, config.EntitlementStatus)
}

func TestPrimary_ManageSubscription_DownloadProfile(t *testing.T) {
	config := BuildPrimaryConfig(1, 0, 0, &OdsaConfigData{
		SubscriptionState: "active",
		SMDPAddress:       "smdp.operator.com",
	}, &OdsaContext{Operation: "ManageSubscription"})
	assert.Equal(t, strconv.Itoa(protocol.SubscriptionResultDownloadProfile), config.ExtraParams["SubscriptionResult"])
	assert.Equal(t, "smdp.operator.com", config.ExtraParams["SMDP+Address"])
}

func TestPrimary_AcquirePlan_WithServiceFlowURL(t *testing.T) {
	config := BuildPrimaryConfig(1, 0, 0, &OdsaConfigData{
		ServiceFlowURL: "https://operator.com/plans",
		PlanID:         "PLAN-001",
		PlanName:       "Unlimited",
	}, &OdsaContext{Operation: "AcquirePlan"})
	assert.Equal(t, strconv.Itoa(protocol.SubscriptionResultContinueToWS), config.ExtraParams["SubscriptionResult"])
	assert.Equal(t, "https://operator.com/plans", config.ExtraParams["ServiceFlow_URL"])
	assert.Equal(t, "PLAN-001", config.ExtraParams["PlanId"])
	assert.Equal(t, "Unlimited", config.ExtraParams["PlanName"])
}

func TestPrimary_AcquirePlan_WithoutServiceFlowURL(t *testing.T) {
	config := BuildPrimaryConfig(1, 0, 0, &OdsaConfigData{
		PlanID:   "PLAN-002",
		PlanName: "Basic",
	}, &OdsaContext{Operation: "AcquirePlan"})
	assert.Equal(t, strconv.Itoa(protocol.SubscriptionResultDone), config.ExtraParams["SubscriptionResult"])
	assert.Equal(t, "PLAN-002", config.ExtraParams["PlanId"])
	assert.Equal(t, "Basic", config.ExtraParams["PlanName"])
}

func TestPrimary_NoOperation(t *testing.T) {
	config := BuildPrimaryConfig(1, 0, 0, &OdsaConfigData{}, nil)
	assert.Equal(t, "ap2009", config.AppID)
	assert.Nil(t, config.ExtraParams)
}

// ---------------------------------------------------------------------------
// Data Plan (ap2010)
// ---------------------------------------------------------------------------

func TestDataPlan_NoOperation(t *testing.T) {
	config := BuildDataPlanConfig(1, 3, 1, nil, nil)
	assert.Equal(t, "ap2010", config.AppID)
	assert.Equal(t, 1, config.EntitlementStatus)
	assert.Nil(t, config.ExtraParams)
}

func TestDataPlan_CheckEligibility_WithPlanID(t *testing.T) {
	config := BuildDataPlanConfig(1, 0, 0, &DataPlanConfigData{PlanID: "PLAN-001"}, &OdsaContext{Operation: "CheckEligibility"})
	assert.Equal(t, 1, config.EntitlementStatus)
	assert.Equal(t, strconv.Itoa(protocol.SubscriptionResultDone), config.ExtraParams["SubscriptionResult"])
}

func TestDataPlan_CheckEligibility_NoPlan(t *testing.T) {
	config := BuildDataPlanConfig(1, 0, 0, &DataPlanConfigData{}, &OdsaContext{Operation: "CheckEligibility"})
	assert.Equal(t, 0, config.EntitlementStatus)
}

func TestDataPlan_AcquirePlan_WithServiceFlowURL(t *testing.T) {
	config := BuildDataPlanConfig(1, 0, 0, &DataPlanConfigData{
		ServiceFlowURL: "https://operator.com/plans",
		PlanID:         "PLAN-001",
		PlanName:       "Unlimited",
	}, &OdsaContext{Operation: "AcquirePlan"})
	assert.Equal(t, strconv.Itoa(protocol.SubscriptionResultContinueToWS), config.ExtraParams["SubscriptionResult"])
	assert.Equal(t, "https://operator.com/plans", config.ExtraParams["ServiceFlow_URL"])
	assert.Equal(t, "PLAN-001", config.ExtraParams["PlanId"])
	assert.Equal(t, "Unlimited", config.ExtraParams["PlanName"])
}

func TestDataPlan_AcquirePlan_WithoutServiceFlowURL(t *testing.T) {
	config := BuildDataPlanConfig(1, 0, 0, &DataPlanConfigData{PlanID: "PLAN-002"}, &OdsaContext{Operation: "AcquirePlan"})
	assert.Equal(t, strconv.Itoa(protocol.SubscriptionResultDone), config.ExtraParams["SubscriptionResult"])
	assert.Equal(t, "PLAN-002", config.ExtraParams["PlanId"])
}

func TestDataPlan_GetPlanDetails(t *testing.T) {
	allowance := int64(107374182400)
	used := int64(21474836480)
	boost := true
	config := BuildDataPlanConfig(1, 0, 0, &DataPlanConfigData{
		PlanID:             "PLAN-001",
		PlanName:           "Unlimited Plus",
		DataAllowanceBytes: &allowance,
		DataUsedBytes:      &used,
		BillingCycleEnd:    "2026-03-01",
		AccessType:         "5G",
		DataType:           "metered",
		BoostEligible:      &boost,
	}, &OdsaContext{Operation: "GetPlanDetails"})
	assert.Equal(t, "PLAN-001", config.ExtraParams["PlanId"])
	assert.Equal(t, "Unlimited Plus", config.ExtraParams["PlanName"])
	assert.Equal(t, "107374182400", config.ExtraParams["DataAllowanceBytes"])
	assert.Equal(t, "21474836480", config.ExtraParams["DataUsedBytes"])
	assert.Equal(t, "2026-03-01", config.ExtraParams["BillingCycleEnd"])
	assert.Equal(t, "5G", config.ExtraParams["AccessType"])
	assert.Equal(t, "metered", config.ExtraParams["DataType"])
	assert.Equal(t, "true", config.ExtraParams["BoostEligible"])
}

func TestDataPlan_GetPlanDetails_MissingData(t *testing.T) {
	config := BuildDataPlanConfig(1, 0, 0, nil, &OdsaContext{Operation: "GetPlanDetails"})
	assert.Equal(t, strconv.Itoa(protocol.SubscriptionResultDone), config.ExtraParams["SubscriptionResult"])
	assert.Empty(t, config.ExtraParams["PlanId"])
}

// ---------------------------------------------------------------------------
// Server ODSA (ap2011)
// ---------------------------------------------------------------------------

func TestServerOdsa_NoOperation(t *testing.T) {
	config := BuildServerOdsaConfig(1, 0, 0, nil, nil)
	assert.Equal(t, "ap2011", config.AppID)
	assert.Nil(t, config.ExtraParams)
}

func TestServerOdsa_CheckEligibility_Eligible(t *testing.T) {
	config := BuildServerOdsaConfig(1, 0, 0, &ServerOdsaConfigData{
		SubscriptionState: "eligible",
		EnterpriseID:      "ENT-001",
	}, &OdsaContext{Operation: "CheckEligibility"})
	assert.Equal(t, 1, config.EntitlementStatus)
	assert.Equal(t, "ENT-001", config.ExtraParams["EnterpriseId"])
	assert.Equal(t, strconv.Itoa(protocol.SubscriptionResultDone), config.ExtraParams["SubscriptionResult"])
}

func TestServerOdsa_CheckEligibility_Pending(t *testing.T) {
	config := BuildServerOdsaConfig(1, 0, 0, &ServerOdsaConfigData{SubscriptionState: "pending"}, &OdsaContext{Operation: "CheckEligibility"})
	assert.Equal(t, 0, config.EntitlementStatus)
}

func TestServerOdsa_ManageSubscription_ServiceFlow(t *testing.T) {
	config := BuildServerOdsaConfig(1, 0, 0, &ServerOdsaConfigData{
		SubscriptionState: "eligible",
		ServiceFlowURL:    "https://operator.com/enterprise/setup",
		EnterpriseID:      "ENT-001",
	}, &OdsaContext{Operation: "ManageSubscription"})
	assert.Equal(t, strconv.Itoa(protocol.SubscriptionResultContinueToWS), config.ExtraParams["SubscriptionResult"])
	assert.Equal(t, "https://operator.com/enterprise/setup", config.ExtraParams["ServiceFlow_URL"])
	assert.Equal(t, "ENT-001", config.ExtraParams["EnterpriseId"])
}

func TestServerOdsa_ManageSubscription_DownloadProfile(t *testing.T) {
	config := BuildServerOdsaConfig(1, 0, 0, &ServerOdsaConfigData{
		SubscriptionState: "active",
		SMDPAddress:       "smdp.operator.com",
	}, &OdsaContext{Operation: "ManageSubscription"})
	assert.Equal(t, strconv.Itoa(protocol.SubscriptionResultDownloadProfile), config.ExtraParams["SubscriptionResult"])
	assert.Equal(t, "smdp.operator.com", config.ExtraParams["SMDP+Address"])
	assert.NotEmpty(t, config.ExtraParams["SMDP+ActivationCode"])
	assert.NotEmpty(t, config.ExtraParams["ProfileICCID"])
}

func TestServerOdsa_ManageService(t *testing.T) {
	config := BuildServerOdsaConfig(1, 3, 1, nil, &OdsaContext{Operation: "ManageService"})
	assert.Equal(t, strconv.Itoa(protocol.SubscriptionResultDone), config.ExtraParams["SubscriptionResult"])
}

// ---------------------------------------------------------------------------
// Direct Carrier Billing (ap2012)
// ---------------------------------------------------------------------------

func TestDcb_Enabled(t *testing.T) {
	config := BuildDcbConfig(1, 0, 1, nil)
	assert.Equal(t, "ap2012", config.AppID)
	assert.Equal(t, 1, config.EntitlementStatus)
	assert.Nil(t, config.ExtraParams)
}

func TestDcb_Incompatible(t *testing.T) {
	config := BuildDcbConfig(2, 0, 0, nil)
	assert.Equal(t, 2, config.EntitlementStatus)
	assert.Contains(t, config.ExtraParams["Message"], "not available")
}

func TestDcb_DisabledRequiresAcceptance(t *testing.T) {
	config := BuildDcbConfig(0, 0, 2, &DcbConfigData{ServiceFlowURL: "https://operator.com/terms/dcb"})
	assert.Equal(t, 0, config.EntitlementStatus)
	assert.Equal(t, "https://operator.com/terms/dcb", config.ServiceFlowURL)
}

func TestDcb_DisabledNoRequiresAcceptance(t *testing.T) {
	config := BuildDcbConfig(0, 0, 0, &DcbConfigData{ServiceFlowURL: "https://operator.com/terms/dcb"})
	assert.Empty(t, config.ServiceFlowURL)
}

// ---------------------------------------------------------------------------
// Private User Identity (ap2013)
// ---------------------------------------------------------------------------

func TestPrivateIdentity_EnabledWithPseudonym(t *testing.T) {
	config := BuildPrivateIdentityConfig(1, 0, 0, &PrivateIdentityConfigData{
		Pseudonym:    "anon-abc123",
		IdentityType: "PSEUDONYM",
	})
	assert.Equal(t, "ap2013", config.AppID)
	assert.Equal(t, "anon-abc123", config.ExtraParams["Pseudonym"])
	assert.Equal(t, "PSEUDONYM", config.ExtraParams["IdentityType"])
}

func TestPrivateIdentity_EnabledDefaultIdentityType(t *testing.T) {
	config := BuildPrivateIdentityConfig(1, 0, 0, &PrivateIdentityConfigData{})
	assert.Equal(t, "PSEUDONYM", config.ExtraParams["IdentityType"])
}

func TestPrivateIdentity_Disabled(t *testing.T) {
	config := BuildPrivateIdentityConfig(0, 0, 0, &PrivateIdentityConfigData{Pseudonym: "should-not-appear"})
	assert.Nil(t, config.ExtraParams)
}

// ---------------------------------------------------------------------------
// Device/User Info (ap2014)
// ---------------------------------------------------------------------------

func TestDeviceUserInfo_NoOperation(t *testing.T) {
	config := BuildDeviceUserInfoConfig(1, 0, 0, nil, nil)
	assert.Equal(t, "ap2014", config.AppID)
	assert.Nil(t, config.ExtraParams)
}

func TestDeviceUserInfo_GetPhoneNumber(t *testing.T) {
	config := BuildDeviceUserInfoConfig(1, 0, 0, &DeviceUserInfoConfigData{MSISDN: "+15551234567"}, &OdsaContext{Operation: "GetPhoneNumber"})
	assert.Equal(t, "+15551234567", config.ExtraParams["MSISDN"])
	assert.Equal(t, strconv.Itoa(protocol.SubscriptionResultDone), config.ExtraParams["SubscriptionResult"])
}

func TestDeviceUserInfo_GetSubscriberInfo(t *testing.T) {
	config := BuildDeviceUserInfoConfig(1, 0, 0, &DeviceUserInfoConfigData{
		MSISDN:      "+15551234567",
		DisplayName: "Alice",
		HomeCarrier: "Test Operator",
	}, &OdsaContext{Operation: "GetSubscriberInfo"})
	assert.Equal(t, "+15551234567", config.ExtraParams["MSISDN"])
	assert.Equal(t, "Alice", config.ExtraParams["DisplayName"])
	assert.Equal(t, "Test Operator", config.ExtraParams["HomeCarrier"])
}

func TestDeviceUserInfo_GetPhoneNumber_NoMSISDN(t *testing.T) {
	config := BuildDeviceUserInfoConfig(1, 0, 0, &DeviceUserInfoConfigData{}, &OdsaContext{Operation: "GetPhoneNumber"})
	assert.Empty(t, config.ExtraParams["MSISDN"])
	assert.Equal(t, strconv.Itoa(protocol.SubscriptionResultDone), config.ExtraParams["SubscriptionResult"])
}

// ---------------------------------------------------------------------------
// App Authentication (ap2015)
// ---------------------------------------------------------------------------

func TestAppAuth_EnabledWithTokenInfo(t *testing.T) {
	config := BuildAppAuthConfig(1, 0, 0, &AppAuthConfigData{
		OperatorTokenURL: "https://auth.operator.com/token",
		AppTokenScope:    "carrier.entitlement",
	})
	assert.Equal(t, "ap2015", config.AppID)
	assert.Equal(t, "https://auth.operator.com/token", config.ExtraParams["OperatorTokenUrl"])
	assert.Equal(t, "carrier.entitlement", config.ExtraParams["AppTokenScope"])
}

func TestAppAuth_EnabledNoConfig(t *testing.T) {
	config := BuildAppAuthConfig(1, 0, 0, &AppAuthConfigData{})
	assert.Nil(t, config.ExtraParams)
}

func TestAppAuth_DisabledNoExtraParams(t *testing.T) {
	config := BuildAppAuthConfig(0, 0, 0, &AppAuthConfigData{OperatorTokenURL: "https://auth.operator.com/token"})
	assert.Nil(t, config.ExtraParams)
}

// ---------------------------------------------------------------------------
// Satellite Mode (ap2016)
// ---------------------------------------------------------------------------

func TestSatMode_EnabledWithPLMNLists(t *testing.T) {
	config := BuildSatModeConfig(1, 0, 0, &SatModeConfigData{
		PLMNAllow:          []string{"00101", "00102"},
		PLMNBarred:         []string{"99999"},
		ServiceConstraints: "sos-only",
	})
	assert.Equal(t, "ap2016", config.AppID)
	assert.Equal(t, "00101,00102", config.ExtraParams["PLMNAllow"])
	assert.Equal(t, "99999", config.ExtraParams["PLMNBarred"])
	assert.Equal(t, "sos-only", config.ExtraParams["ServiceConstraints"])
}

func TestSatMode_EnabledEmptyArrays(t *testing.T) {
	config := BuildSatModeConfig(1, 0, 0, &SatModeConfigData{PLMNAllow: []string{}, PLMNBarred: []string{}})
	assert.Nil(t, config.ExtraParams)
}

func TestSatMode_DisabledNoExtraParams(t *testing.T) {
	config := BuildSatModeConfig(0, 0, 0, &SatModeConfigData{PLMNAllow: []string{"00101"}})
	assert.Nil(t, config.ExtraParams)
}

func TestSatMode_EnabledNoConfig(t *testing.T) {
	config := BuildSatModeConfig(1, 0, 0, nil)
	assert.Nil(t, config.ExtraParams)
}
