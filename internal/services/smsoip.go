package services

import (
	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/protocol"
)

// SmsOipConfigData holds configData for ap2005.
type SmsOipConfigData struct {
	Addresses []protocol.AddressConfig `json:"addresses,omitempty"`
}

var _defaultSmsOipAddresses = []protocol.AddressConfig{
	{AddrType: "1", Addr: "smsc.operator.com"},
}

// BuildSmsOipConfig builds the ApplicationConfig for SMSoIP (ap2005).
func BuildSmsOipConfig(status, provStatus, tcStatus int, configData *SmsOipConfigData) *protocol.ApplicationConfig {
	if configData == nil {
		configData = &SmsOipConfigData{}
	}

	result := &protocol.ApplicationConfig{
		AppID:             "ap2005",
		EntitlementStatus: status,
		TcStatus:          protocol.IntPtr(tcStatus),
		ProvStatus:        protocol.IntPtr(provStatus),
	}

	if status != protocol.EntitlementStatusEnabled {
		result.AddrStatus = protocol.IntPtr(0)
		return result
	}

	result.AddrStatus = protocol.IntPtr(1)
	if len(configData.Addresses) > 0 {
		result.Addresses = configData.Addresses
	} else {
		result.Addresses = _defaultSmsOipAddresses
	}

	return result
}
