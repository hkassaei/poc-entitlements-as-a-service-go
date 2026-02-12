package services

import (
	"github.com/hkassaei/entitlements-as-a-service-poc-go/internal/protocol"
)

// ServerOdsaConfigData holds configData for ap2011.
type ServerOdsaConfigData struct {
	EnterpriseID        string `json:"enterpriseId,omitempty"`
	SMDPAddress         string `json:"smdpAddress,omitempty"`
	ProfileType         string `json:"profileType,omitempty"`
	SubscriptionState   string `json:"subscriptionState,omitempty"`
	ServiceFlowURL      string `json:"serviceFlowUrl,omitempty"`
	ServiceFlowUserData string `json:"serviceFlowUserData,omitempty"`
}

// BuildServerOdsaConfig builds the ApplicationConfig for Server-Initiated ODSA (ap2011).
func BuildServerOdsaConfig(status, provStatus, tcStatus int, configData *ServerOdsaConfigData, ctx *OdsaContext) *protocol.ApplicationConfig {
	if configData == nil {
		configData = &ServerOdsaConfigData{}
	}

	operation := ""
	if ctx != nil {
		operation = ctx.Operation
	}

	if operation == "" {
		return &protocol.ApplicationConfig{
			AppID:             "ap2011",
			EntitlementStatus: status,
			ProvStatus:        protocol.IntPtr(provStatus),
			TcStatus:          protocol.IntPtr(tcStatus),
		}
	}

	switch operation {
	case protocol.OpCheckEligibility:
		return serverOdsaCheckEligibility(provStatus, tcStatus, configData)
	case protocol.OpManageSubscription:
		return serverOdsaManageSubscription(status, provStatus, tcStatus, configData)
	case protocol.OpManageService:
		return BuildOdsaBaseConfig("ap2011", status, provStatus, tcStatus, protocol.SubscriptionResultDone, nil)
	default:
		return &protocol.ApplicationConfig{
			AppID:             "ap2011",
			EntitlementStatus: status,
			ProvStatus:        protocol.IntPtr(provStatus),
			TcStatus:          protocol.IntPtr(tcStatus),
		}
	}
}

func serverOdsaCheckEligibility(provStatus, tcStatus int, data *ServerOdsaConfigData) *protocol.ApplicationConfig {
	isEligible := data.SubscriptionState == "eligible" || data.SubscriptionState == "active"
	entitlementStatus := protocol.EntitlementStatusDisabled
	if isEligible {
		entitlementStatus = protocol.EntitlementStatusEnabled
	}
	extra := map[string]string{}
	if data.EnterpriseID != "" {
		extra["EnterpriseId"] = data.EnterpriseID
	}
	return BuildOdsaBaseConfig("ap2011", entitlementStatus, provStatus, tcStatus, protocol.SubscriptionResultDone, extra)
}

func serverOdsaManageSubscription(status, provStatus, tcStatus int, data *ServerOdsaConfigData) *protocol.ApplicationConfig {
	if data.SubscriptionState == "eligible" && data.ServiceFlowURL != "" {
		extra := map[string]string{"ServiceFlow_URL": data.ServiceFlowURL}
		if data.ServiceFlowUserData != "" {
			extra["ServiceFlow_UserData"] = data.ServiceFlowUserData
		}
		if data.EnterpriseID != "" {
			extra["EnterpriseId"] = data.EnterpriseID
		}
		return BuildOdsaBaseConfig("ap2011", status, provStatus, tcStatus, protocol.SubscriptionResultContinueToWS, extra)
	}

	if data.SubscriptionState == "active" && data.SMDPAddress != "" {
		profileKey := data.ProfileType
		if profileKey == "" {
			profileKey = "default"
		}
		activation := GetActivationCode(profileKey)
		return BuildOdsaBaseConfig("ap2011", status, provStatus, tcStatus, protocol.SubscriptionResultDownloadProfile, map[string]string{
			"SMDP+Address":        data.SMDPAddress,
			"SMDP+ActivationCode": activation.ActivationCode,
			"ProfileICCID":        activation.ICCID,
		})
	}

	return BuildOdsaBaseConfig("ap2011", status, provStatus, tcStatus, protocol.SubscriptionResultDone, nil)
}
