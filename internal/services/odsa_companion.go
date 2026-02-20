package services

import (
	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/protocol"
)

// BuildCompanionConfig builds the ApplicationConfig for ODSA Companion (ap2006).
func BuildCompanionConfig(status, provStatus, tcStatus int, configData *OdsaConfigData, ctx *OdsaContext) *protocol.ApplicationConfig {
	if configData == nil {
		configData = &OdsaConfigData{}
	}

	operation := ""
	if ctx != nil {
		operation = ctx.Operation
	}

	if operation == "" {
		return &protocol.ApplicationConfig{
			AppID:             "ap2006",
			EntitlementStatus: status,
			ProvStatus:        protocol.IntPtr(provStatus),
			TcStatus:          protocol.IntPtr(tcStatus),
		}
	}

	switch operation {
	case protocol.OpCheckEligibility:
		return companionCheckEligibility(provStatus, tcStatus, configData)
	case protocol.OpManageSubscription:
		return companionManageSubscription(status, provStatus, tcStatus, configData)
	case protocol.OpManageService:
		return BuildOdsaBaseConfig("ap2006", status, provStatus, tcStatus, protocol.SubscriptionResultDone, nil)
	case protocol.OpAcquireConfiguration:
		return companionAcquireConfiguration(status, provStatus, tcStatus, configData)
	case protocol.OpAcquireTemporaryToken:
		return BuildOdsaBaseConfig("ap2006", status, provStatus, tcStatus, protocol.SubscriptionResultDone, nil)
	default:
		return &protocol.ApplicationConfig{
			AppID:             "ap2006",
			EntitlementStatus: status,
			ProvStatus:        protocol.IntPtr(provStatus),
			TcStatus:          protocol.IntPtr(tcStatus),
		}
	}
}

func companionCheckEligibility(provStatus, tcStatus int, data *OdsaConfigData) *protocol.ApplicationConfig {
	status := protocol.EntitlementStatusDisabled
	if data.SubscriptionState == "eligible" || data.SubscriptionState == "active" {
		status = protocol.EntitlementStatusEnabled
	}
	return BuildOdsaBaseConfig("ap2006", status, provStatus, tcStatus, protocol.SubscriptionResultDone, nil)
}

func companionManageSubscription(status, provStatus, tcStatus int, data *OdsaConfigData) *protocol.ApplicationConfig {
	if data.SubscriptionState == "eligible" && data.ServiceFlowURL != "" {
		extra := map[string]string{"ServiceFlow_URL": data.ServiceFlowURL}
		if data.ServiceFlowUserData != "" {
			extra["ServiceFlow_UserData"] = data.ServiceFlowUserData
		}
		return BuildOdsaBaseConfig("ap2006", status, provStatus, tcStatus, protocol.SubscriptionResultContinueToWS, extra)
	}

	if data.SubscriptionState == "active" && data.SMDPAddress != "" {
		profileKey := data.ProfileType
		if profileKey == "" {
			profileKey = "companion"
		}
		activation := GetActivationCode(profileKey)
		return BuildOdsaBaseConfig("ap2006", status, provStatus, tcStatus, protocol.SubscriptionResultDownloadProfile, map[string]string{
			"SMDP+Address":        data.SMDPAddress,
			"SMDP+ActivationCode": activation.ActivationCode,
			"ProfileICCID":        activation.ICCID,
		})
	}

	return BuildOdsaBaseConfig("ap2006", status, provStatus, tcStatus, protocol.SubscriptionResultDone, nil)
}

func companionAcquireConfiguration(status, provStatus, tcStatus int, data *OdsaConfigData) *protocol.ApplicationConfig {
	extra := map[string]string{}
	if data.SMDPAddress != "" {
		extra["SMDP+Address"] = data.SMDPAddress
	}
	if data.ProfileICCID != "" {
		extra["ProfileICCID"] = data.ProfileICCID
	}
	return BuildOdsaBaseConfig("ap2006", status, provStatus, tcStatus, protocol.SubscriptionResultDone, extra)
}
