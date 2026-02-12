package services

import (
	"github.com/hkassaei/entitlements-as-a-service-poc-go/internal/protocol"
)

// VoLTEConfigData holds configData for ap2003.
type VoLTEConfigData struct {
	Addresses     []protocol.AddressConfig `json:"addresses,omitempty"`
	VoLTEEntitled string                   `json:"volteEntitled,omitempty"`
	VoNREntitled  string                   `json:"vonrEntitled,omitempty"`
}

var defaultVoLTEAddresses = []protocol.AddressConfig{
	{AddrType: "1", Addr: "pcscf.operator.com"},
}

// BuildVoLTEConfig builds the ApplicationConfig for VoLTE (ap2003).
func BuildVoLTEConfig(status, provStatus, tcStatus int, configData *VoLTEConfigData) *protocol.ApplicationConfig {
	if configData == nil {
		configData = &VoLTEConfigData{}
	}

	result := &protocol.ApplicationConfig{
		AppID:             "ap2003",
		EntitlementStatus: status,
		TcStatus:          protocol.IntPtr(tcStatus),
		ProvStatus:        protocol.IntPtr(provStatus),
		ExtraParams:       map[string]string{},
	}

	if status == protocol.EntitlementStatusEnabled {
		result.AddrStatus = protocol.IntPtr(1)
		if len(configData.Addresses) > 0 {
			result.Addresses = configData.Addresses
		} else {
			result.Addresses = defaultVoLTEAddresses
		}
		volte := configData.VoLTEEntitled
		if volte == "" {
			volte = "1"
		}
		vonr := configData.VoNREntitled
		if vonr == "" {
			vonr = "1"
		}
		result.ExtraParams["VoLTE_Entitled"] = volte
		result.ExtraParams["VoNR_Entitled"] = vonr
	} else {
		result.AddrStatus = protocol.IntPtr(0)
		result.ExtraParams["VoLTE_Entitled"] = "0"
		result.ExtraParams["VoNR_Entitled"] = "0"
	}

	return result
}
