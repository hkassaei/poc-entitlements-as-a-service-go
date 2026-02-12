package services

import (
	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/protocol"
)

// VoWiFiConfigData holds configData for ap2004.
type VoWiFiConfigData struct {
	Addresses      []protocol.AddressConfig `json:"addresses,omitempty"`
	ServiceFlowURL string                   `json:"serviceFlowUrl,omitempty"`
}

var _defaultVoWiFiAddresses = []protocol.AddressConfig{
	{AddrType: "1", Addr: "epdg.operator.com"},
	{AddrType: "1", Addr: "pcscf.operator.com"},
}

// BuildVoWiFiConfig builds the ApplicationConfig for Wi-Fi Calling (ap2004).
func BuildVoWiFiConfig(status, provStatus, tcStatus int, configData *VoWiFiConfigData) *protocol.ApplicationConfig {
	if configData == nil {
		configData = &VoWiFiConfigData{}
	}

	result := &protocol.ApplicationConfig{
		AppID:             "ap2004",
		EntitlementStatus: status,
		TcStatus:          protocol.IntPtr(tcStatus),
		ProvStatus:        protocol.IntPtr(provStatus),
	}

	if status == protocol.EntitlementStatusEnabled {
		result.AddrStatus = protocol.IntPtr(1)
		if len(configData.Addresses) > 0 {
			result.Addresses = configData.Addresses
		} else {
			result.Addresses = _defaultVoWiFiAddresses
		}
	} else {
		result.AddrStatus = protocol.IntPtr(0)
	}

	if tcStatus == protocol.TcStatusRequiresAcceptance && configData.ServiceFlowURL != "" {
		result.ServiceFlowURL = configData.ServiceFlowURL
	}

	return result
}
