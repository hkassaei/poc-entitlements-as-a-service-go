package services

import (
	"context"
	"encoding/json"

	"github.com/hkassaei/entitlements-as-a-service-poc-go/internal/config"
	"github.com/hkassaei/entitlements-as-a-service-poc-go/internal/db"
	"github.com/hkassaei/entitlements-as-a-service-poc-go/internal/protocol"
)

// FormattedResponse holds the formatted response body and content type.
type FormattedResponse struct {
	Body        interface{} // map[string]interface{} for JSON, string for XML
	ContentType string
}

// ResponseBuilder builds entitlement responses by looking up DB state
// and routing to the appropriate service handler.
type ResponseBuilder struct {
	queries *db.Queries
	cfg     config.Config
}

// NewResponseBuilder creates a new ResponseBuilder.
func NewResponseBuilder(queries *db.Queries, cfg config.Config) *ResponseBuilder {
	return &ResponseBuilder{queries: queries, cfg: cfg}
}

// BuildEntitlementResponse builds a complete entitlement response for a subscriber and app.
func (b *ResponseBuilder) BuildEntitlementResponse(
	ctx context.Context,
	token, subscriberID, appID, acceptContentType string,
	odsaCtx *OdsaContext,
) (*FormattedResponse, error) {
	ents, err := b.queries.FindEntitlementsBySubscriber(ctx, subscriberID)
	if err != nil {
		return nil, err
	}

	var entitlement *db.Entitlement
	for i := range ents {
		if ents[i].AppID == appID {
			entitlement = &ents[i]
			break
		}
	}

	status := 1
	provStatus := 0
	tcStatus := 0
	var configData json.RawMessage
	if entitlement != nil {
		status = entitlement.Status
		provStatus = entitlement.ProvStatus
		tcStatus = entitlement.TcStatus
		configData = entitlement.ConfigData
	}

	appConfig := buildAppConfig(appID, status, provStatus, tcStatus, configData, odsaCtx)

	response := &protocol.ServiceEntitlementResponse{
		Version:      "1",
		Validity:     b.cfg.DefaultConfigValidity,
		Token:        token,
		Applications: []protocol.ApplicationConfig{*appConfig},
	}

	if acceptContentType == "xml" {
		return &FormattedResponse{
			Body:        protocol.BuildXMLResponse(response),
			ContentType: "application/xml",
		}, nil
	}

	return &FormattedResponse{
		Body:        protocol.BuildJSONResponse(response),
		ContentType: "application/json",
	}, nil
}

func buildAppConfig(appID string, status, provStatus, tcStatus int, configData json.RawMessage, odsaCtx *OdsaContext) *protocol.ApplicationConfig {
	switch appID {
	case config.AppIDVoWiFi:
		var cd VoWiFiConfigData
		_ = json.Unmarshal(configData, &cd)
		return BuildVoWiFiConfig(status, provStatus, tcStatus, &cd)
	case config.AppIDVoLTE:
		var cd VoLTEConfigData
		_ = json.Unmarshal(configData, &cd)
		return BuildVoLTEConfig(status, provStatus, tcStatus, &cd)
	case config.AppIDSMSoIP:
		var cd SmsOipConfigData
		_ = json.Unmarshal(configData, &cd)
		return BuildSmsOipConfig(status, provStatus, tcStatus, &cd)
	case config.AppIDODSACompanion:
		var cd OdsaConfigData
		_ = json.Unmarshal(configData, &cd)
		return BuildCompanionConfig(status, provStatus, tcStatus, &cd, odsaCtx)
	case config.AppIDODSAPrimary:
		var cd OdsaConfigData
		_ = json.Unmarshal(configData, &cd)
		return BuildPrimaryConfig(status, provStatus, tcStatus, &cd, odsaCtx)
	case config.AppIDDataPlanInfo:
		var cd DataPlanConfigData
		_ = json.Unmarshal(configData, &cd)
		return BuildDataPlanConfig(status, provStatus, tcStatus, &cd, odsaCtx)
	case config.AppIDServerInitiatedODSA:
		var cd ServerOdsaConfigData
		_ = json.Unmarshal(configData, &cd)
		return BuildServerOdsaConfig(status, provStatus, tcStatus, &cd, odsaCtx)
	case config.AppIDDirectCarrierBilling:
		var cd DcbConfigData
		_ = json.Unmarshal(configData, &cd)
		return BuildDcbConfig(status, provStatus, tcStatus, &cd)
	case config.AppIDPrivateUserIdentity:
		var cd PrivateIdentityConfigData
		_ = json.Unmarshal(configData, &cd)
		return BuildPrivateIdentityConfig(status, provStatus, tcStatus, &cd)
	case config.AppIDDeviceUserInfo:
		var cd DeviceUserInfoConfigData
		_ = json.Unmarshal(configData, &cd)
		return BuildDeviceUserInfoConfig(status, provStatus, tcStatus, &cd, odsaCtx)
	case config.AppIDAppAuthentication:
		var cd AppAuthConfigData
		_ = json.Unmarshal(configData, &cd)
		return BuildAppAuthConfig(status, provStatus, tcStatus, &cd)
	case config.AppIDSatelliteMode:
		var cd SatModeConfigData
		_ = json.Unmarshal(configData, &cd)
		return BuildSatModeConfig(status, provStatus, tcStatus, &cd)
	default:
		return &protocol.ApplicationConfig{
			AppID:             appID,
			EntitlementStatus: status,
			ProvStatus:        protocol.IntPtr(provStatus),
			TcStatus:          protocol.IntPtr(tcStatus),
		}
	}
}
