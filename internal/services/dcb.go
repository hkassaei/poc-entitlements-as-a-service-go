package services

import (
	"github.com/hkassaei/entitlements-as-a-service-poc-go/internal/protocol"
)

// DcbConfigData holds configData for ap2012.
type DcbConfigData struct {
	ServiceFlowURL string `json:"serviceFlowUrl,omitempty"`
}

// BuildDcbConfig builds the ApplicationConfig for Direct Carrier Billing (ap2012).
func BuildDcbConfig(status, provStatus, tcStatus int, configData *DcbConfigData) *protocol.ApplicationConfig {
	if configData == nil {
		configData = &DcbConfigData{}
	}

	result := &protocol.ApplicationConfig{
		AppID:             "ap2012",
		EntitlementStatus: status,
		ProvStatus:        protocol.IntPtr(provStatus),
		TcStatus:          protocol.IntPtr(tcStatus),
	}

	if status == protocol.EntitlementStatusIncompatible {
		result.ExtraParams = map[string]string{
			"Message": "Direct Carrier Billing is not available for this device.",
		}
		return result
	}

	if status == protocol.EntitlementStatusDisabled && tcStatus == protocol.TcStatusRequiresAcceptance && configData.ServiceFlowURL != "" {
		result.ServiceFlowURL = configData.ServiceFlowURL
	}

	return result
}
