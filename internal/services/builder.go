package services

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/config"
	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/db"
	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/protocol"
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
		if err := json.Unmarshal(configData, &cd); err != nil {
			slog.Warn("failed to unmarshal config data", "appId", appID, "err", err)
		}
		return BuildVoWiFiConfig(status, provStatus, tcStatus, &cd)
	case config.AppIDVoLTE:
		var cd VoLTEConfigData
		if err := json.Unmarshal(configData, &cd); err != nil {
			slog.Warn("failed to unmarshal config data", "appId", appID, "err", err)
		}
		return BuildVoLTEConfig(status, provStatus, tcStatus, &cd)
	case config.AppIDSMSoIP:
		var cd SmsOipConfigData
		if err := json.Unmarshal(configData, &cd); err != nil {
			slog.Warn("failed to unmarshal config data", "appId", appID, "err", err)
		}
		return BuildSmsOipConfig(status, provStatus, tcStatus, &cd)
	case config.AppIDODSACompanion:
		var cd OdsaConfigData
		if err := json.Unmarshal(configData, &cd); err != nil {
			slog.Warn("failed to unmarshal config data", "appId", appID, "err", err)
		}
		return BuildCompanionConfig(status, provStatus, tcStatus, &cd, odsaCtx)
	case config.AppIDODSAPrimary:
		var cd OdsaConfigData
		if err := json.Unmarshal(configData, &cd); err != nil {
			slog.Warn("failed to unmarshal config data", "appId", appID, "err", err)
		}
		return BuildPrimaryConfig(status, provStatus, tcStatus, &cd, odsaCtx)
	case config.AppIDDataPlanInfo:
		var cd DataPlanConfigData
		if err := json.Unmarshal(configData, &cd); err != nil {
			slog.Warn("failed to unmarshal config data", "appId", appID, "err", err)
		}
		return BuildDataPlanConfig(status, provStatus, tcStatus, &cd, odsaCtx)
	case config.AppIDServerInitiatedODSA:
		var cd ServerOdsaConfigData
		if err := json.Unmarshal(configData, &cd); err != nil {
			slog.Warn("failed to unmarshal config data", "appId", appID, "err", err)
		}
		return BuildServerOdsaConfig(status, provStatus, tcStatus, &cd, odsaCtx)
	case config.AppIDDirectCarrierBilling:
		var cd DcbConfigData
		if err := json.Unmarshal(configData, &cd); err != nil {
			slog.Warn("failed to unmarshal config data", "appId", appID, "err", err)
		}
		return BuildDcbConfig(status, provStatus, tcStatus, &cd)
	case config.AppIDPrivateUserIdentity:
		var cd PrivateIdentityConfigData
		if err := json.Unmarshal(configData, &cd); err != nil {
			slog.Warn("failed to unmarshal config data", "appId", appID, "err", err)
		}
		return BuildPrivateIdentityConfig(status, provStatus, tcStatus, &cd)
	case config.AppIDDeviceUserInfo:
		var cd DeviceUserInfoConfigData
		if err := json.Unmarshal(configData, &cd); err != nil {
			slog.Warn("failed to unmarshal config data", "appId", appID, "err", err)
		}
		return BuildDeviceUserInfoConfig(status, provStatus, tcStatus, &cd, odsaCtx)
	case config.AppIDAppAuthentication:
		var cd AppAuthConfigData
		if err := json.Unmarshal(configData, &cd); err != nil {
			slog.Warn("failed to unmarshal config data", "appId", appID, "err", err)
		}
		return BuildAppAuthConfig(status, provStatus, tcStatus, &cd)
	case config.AppIDSatelliteMode:
		var cd SatModeConfigData
		if err := json.Unmarshal(configData, &cd); err != nil {
			slog.Warn("failed to unmarshal config data", "appId", appID, "err", err)
		}
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
