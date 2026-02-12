package services

import (
	"github.com/hkassaei/entitlements-as-a-service-poc-go/internal/protocol"
)

// DeviceUserInfoConfigData holds configData for ap2014.
type DeviceUserInfoConfigData struct {
	MSISDN      string `json:"msisdn,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	HomeCarrier string `json:"homeCarrier,omitempty"`
}

// BuildDeviceUserInfoConfig builds the ApplicationConfig for Device/User Info (ap2014).
func BuildDeviceUserInfoConfig(status, provStatus, tcStatus int, configData *DeviceUserInfoConfigData, ctx *OdsaContext) *protocol.ApplicationConfig {
	if configData == nil {
		configData = &DeviceUserInfoConfigData{}
	}

	operation := ""
	if ctx != nil {
		operation = ctx.Operation
	}

	if operation == "" {
		return &protocol.ApplicationConfig{
			AppID:             "ap2014",
			EntitlementStatus: status,
			ProvStatus:        protocol.IntPtr(provStatus),
			TcStatus:          protocol.IntPtr(tcStatus),
		}
	}

	switch operation {
	case "GetPhoneNumber":
		extra := map[string]string{}
		if configData.MSISDN != "" {
			extra["MSISDN"] = configData.MSISDN
		}
		return BuildOdsaBaseConfig("ap2014", status, provStatus, tcStatus, protocol.SubscriptionResultDone, extra)
	case "GetSubscriberInfo":
		extra := map[string]string{}
		if configData.MSISDN != "" {
			extra["MSISDN"] = configData.MSISDN
		}
		if configData.DisplayName != "" {
			extra["DisplayName"] = configData.DisplayName
		}
		if configData.HomeCarrier != "" {
			extra["HomeCarrier"] = configData.HomeCarrier
		}
		return BuildOdsaBaseConfig("ap2014", status, provStatus, tcStatus, protocol.SubscriptionResultDone, extra)
	default:
		return &protocol.ApplicationConfig{
			AppID:             "ap2014",
			EntitlementStatus: status,
			ProvStatus:        protocol.IntPtr(provStatus),
			TcStatus:          protocol.IntPtr(tcStatus),
		}
	}
}
