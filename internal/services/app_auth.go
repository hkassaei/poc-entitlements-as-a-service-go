package services

import (
	"github.com/hkassaei/entitlements-as-a-service-poc-go/internal/protocol"
)

// AppAuthConfigData holds configData for ap2015.
type AppAuthConfigData struct {
	OperatorTokenURL string `json:"operatorTokenUrl,omitempty"`
	AppTokenScope    string `json:"appTokenScope,omitempty"`
}

// BuildAppAuthConfig builds the ApplicationConfig for App Authentication (ap2015).
func BuildAppAuthConfig(status, provStatus, tcStatus int, configData *AppAuthConfigData) *protocol.ApplicationConfig {
	if configData == nil {
		configData = &AppAuthConfigData{}
	}

	result := &protocol.ApplicationConfig{
		AppID:             "ap2015",
		EntitlementStatus: status,
		ProvStatus:        protocol.IntPtr(provStatus),
		TcStatus:          protocol.IntPtr(tcStatus),
	}

	if status == protocol.EntitlementStatusEnabled {
		extra := map[string]string{}
		if configData.OperatorTokenURL != "" {
			extra["OperatorTokenUrl"] = configData.OperatorTokenURL
		}
		if configData.AppTokenScope != "" {
			extra["AppTokenScope"] = configData.AppTokenScope
		}
		if len(extra) > 0 {
			result.ExtraParams = extra
		}
	}

	return result
}
