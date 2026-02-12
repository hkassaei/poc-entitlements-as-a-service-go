package services

import (
	"strings"

	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/protocol"
)

// SatModeConfigData holds configData for ap2016.
type SatModeConfigData struct {
	PLMNAllow          []string `json:"plmnAllow,omitempty"`
	PLMNBarred         []string `json:"plmnBarred,omitempty"`
	ServiceConstraints string   `json:"serviceConstraints,omitempty"`
}

// BuildSatModeConfig builds the ApplicationConfig for Satellite Mode (ap2016).
func BuildSatModeConfig(status, provStatus, tcStatus int, configData *SatModeConfigData) *protocol.ApplicationConfig {
	if configData == nil {
		configData = &SatModeConfigData{}
	}

	result := &protocol.ApplicationConfig{
		AppID:             "ap2016",
		EntitlementStatus: status,
		ProvStatus:        protocol.IntPtr(provStatus),
		TcStatus:          protocol.IntPtr(tcStatus),
	}

	if status == protocol.EntitlementStatusEnabled {
		extra := map[string]string{}
		if len(configData.PLMNAllow) > 0 {
			extra["PLMNAllow"] = strings.Join(configData.PLMNAllow, ",")
		}
		if len(configData.PLMNBarred) > 0 {
			extra["PLMNBarred"] = strings.Join(configData.PLMNBarred, ",")
		}
		if configData.ServiceConstraints != "" {
			extra["ServiceConstraints"] = configData.ServiceConstraints
		}
		if len(extra) > 0 {
			result.ExtraParams = extra
		}
	}

	return result
}
