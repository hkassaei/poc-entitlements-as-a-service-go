package services

import (
	"strconv"

	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/protocol"
)

// OdsaConfigData holds the configData JSONB for ODSA services (ap2006/ap2009).
type OdsaConfigData struct {
	SubscriptionState   string `json:"subscriptionState,omitempty"`
	SMDPAddress         string `json:"smdpAddress,omitempty"`
	ProfileICCID        string `json:"profileIccid,omitempty"`
	ProfileType         string `json:"profileType,omitempty"`
	CompanionDeviceIMEI string `json:"companionDeviceImei,omitempty"`
	PrimaryDeviceIMSI   string `json:"primaryDeviceImsi,omitempty"`
	PlanID              string `json:"planId,omitempty"`
	PlanName            string `json:"planName,omitempty"`
	ServiceFlowURL      string `json:"serviceFlowUrl,omitempty"`
	ServiceFlowUserData string `json:"serviceFlowUserData,omitempty"`
}

// OdsaContext holds the operation context from the request.
type OdsaContext struct {
	Operation     string
	OperationType int
}

// BuildOdsaBaseConfig builds a base ODSA ApplicationConfig with SubscriptionResult in extraParams.
func BuildOdsaBaseConfig(appID string, status, provStatus, tcStatus, subscriptionResult int, extraFields map[string]string) *protocol.ApplicationConfig {
	extra := map[string]string{
		"SubscriptionResult": strconv.Itoa(subscriptionResult),
	}
	for k, v := range extraFields {
		extra[k] = v
	}

	return &protocol.ApplicationConfig{
		AppID:             appID,
		EntitlementStatus: status,
		ProvStatus:        protocol.IntPtr(provStatus),
		TcStatus:          protocol.IntPtr(tcStatus),
		ExtraParams:       extra,
	}
}
