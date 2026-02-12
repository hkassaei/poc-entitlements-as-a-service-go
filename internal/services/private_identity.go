package services

import (
	"github.com/hkassaei/entitlements-as-a-service-poc-go/internal/protocol"
)

// PrivateIdentityConfigData holds configData for ap2013.
type PrivateIdentityConfigData struct {
	Pseudonym    string `json:"pseudonym,omitempty"`
	IdentityType string `json:"identityType,omitempty"`
}

// BuildPrivateIdentityConfig builds the ApplicationConfig for Private User Identity (ap2013).
func BuildPrivateIdentityConfig(status, provStatus, tcStatus int, configData *PrivateIdentityConfigData) *protocol.ApplicationConfig {
	if configData == nil {
		configData = &PrivateIdentityConfigData{}
	}

	result := &protocol.ApplicationConfig{
		AppID:             "ap2013",
		EntitlementStatus: status,
		ProvStatus:        protocol.IntPtr(provStatus),
		TcStatus:          protocol.IntPtr(tcStatus),
	}

	if status == protocol.EntitlementStatusEnabled {
		extra := map[string]string{}
		if configData.Pseudonym != "" {
			extra["Pseudonym"] = configData.Pseudonym
		}
		identityType := configData.IdentityType
		if identityType == "" {
			identityType = "PSEUDONYM"
		}
		extra["IdentityType"] = identityType
		if len(extra) > 0 {
			result.ExtraParams = extra
		}
	}

	return result
}
