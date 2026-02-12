package services

import (
	"strconv"

	"github.com/hkassaei/entitlements-as-a-service-poc-go/internal/protocol"
)

// DataPlanConfigData holds configData for ap2010.
type DataPlanConfigData struct {
	PlanName           string `json:"planName,omitempty"`
	PlanID             string `json:"planId,omitempty"`
	DataAllowanceBytes *int64 `json:"dataAllowanceBytes,omitempty"`
	DataUsedBytes      *int64 `json:"dataUsedBytes,omitempty"`
	BillingCycleEnd    string `json:"billingCycleEnd,omitempty"`
	AccessType         string `json:"accessType,omitempty"`
	DataType           string `json:"dataType,omitempty"`
	BoostEligible      *bool  `json:"boostEligible,omitempty"`
	ServiceFlowURL     string `json:"serviceFlowUrl,omitempty"`
}

// BuildDataPlanConfig builds the ApplicationConfig for Data Plan (ap2010).
func BuildDataPlanConfig(status, provStatus, tcStatus int, configData *DataPlanConfigData, ctx *OdsaContext) *protocol.ApplicationConfig {
	if configData == nil {
		configData = &DataPlanConfigData{}
	}

	operation := ""
	if ctx != nil {
		operation = ctx.Operation
	}

	if operation == "" {
		return &protocol.ApplicationConfig{
			AppID:             "ap2010",
			EntitlementStatus: status,
			ProvStatus:        protocol.IntPtr(provStatus),
			TcStatus:          protocol.IntPtr(tcStatus),
		}
	}

	switch operation {
	case protocol.OpCheckEligibility:
		return dataPlanCheckEligibility(provStatus, tcStatus, configData)
	case protocol.OpAcquirePlan:
		return dataPlanAcquirePlan(status, provStatus, tcStatus, configData)
	case "GetPlanDetails":
		return dataPlanGetPlanDetails(status, provStatus, tcStatus, configData)
	default:
		return &protocol.ApplicationConfig{
			AppID:             "ap2010",
			EntitlementStatus: status,
			ProvStatus:        protocol.IntPtr(provStatus),
			TcStatus:          protocol.IntPtr(tcStatus),
		}
	}
}

func dataPlanCheckEligibility(provStatus, tcStatus int, data *DataPlanConfigData) *protocol.ApplicationConfig {
	isEligible := (data.BoostEligible != nil && *data.BoostEligible) || data.PlanID != ""
	entitlementStatus := protocol.EntitlementStatusDisabled
	if isEligible {
		entitlementStatus = protocol.EntitlementStatusEnabled
	}
	return BuildOdsaBaseConfig("ap2010", entitlementStatus, provStatus, tcStatus, protocol.SubscriptionResultDone, nil)
}

func dataPlanAcquirePlan(status, provStatus, tcStatus int, data *DataPlanConfigData) *protocol.ApplicationConfig {
	if data.ServiceFlowURL != "" {
		extra := map[string]string{"ServiceFlow_URL": data.ServiceFlowURL}
		if data.PlanID != "" {
			extra["PlanId"] = data.PlanID
		}
		if data.PlanName != "" {
			extra["PlanName"] = data.PlanName
		}
		return BuildOdsaBaseConfig("ap2010", status, provStatus, tcStatus, protocol.SubscriptionResultContinueToWS, extra)
	}

	extra := map[string]string{}
	if data.PlanID != "" {
		extra["PlanId"] = data.PlanID
	}
	if data.PlanName != "" {
		extra["PlanName"] = data.PlanName
	}
	return BuildOdsaBaseConfig("ap2010", status, provStatus, tcStatus, protocol.SubscriptionResultDone, extra)
}

func dataPlanGetPlanDetails(status, provStatus, tcStatus int, data *DataPlanConfigData) *protocol.ApplicationConfig {
	extra := map[string]string{}
	if data.PlanID != "" {
		extra["PlanId"] = data.PlanID
	}
	if data.PlanName != "" {
		extra["PlanName"] = data.PlanName
	}
	if data.DataAllowanceBytes != nil {
		extra["DataAllowanceBytes"] = strconv.FormatInt(*data.DataAllowanceBytes, 10)
	}
	if data.DataUsedBytes != nil {
		extra["DataUsedBytes"] = strconv.FormatInt(*data.DataUsedBytes, 10)
	}
	if data.BillingCycleEnd != "" {
		extra["BillingCycleEnd"] = data.BillingCycleEnd
	}
	if data.AccessType != "" {
		extra["AccessType"] = data.AccessType
	}
	if data.DataType != "" {
		extra["DataType"] = data.DataType
	}
	if data.BoostEligible != nil {
		extra["BoostEligible"] = strconv.FormatBool(*data.BoostEligible)
	}
	return BuildOdsaBaseConfig("ap2010", status, provStatus, tcStatus, protocol.SubscriptionResultDone, extra)
}
