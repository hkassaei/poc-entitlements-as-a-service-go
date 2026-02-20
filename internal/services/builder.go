package services

import (
	"context"
	"encoding/json"
	"fmt"
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
		return nil, fmt.Errorf("find entitlements: %w", err)
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

// unmarshalConfig unmarshals configData into a typed struct, logging on error.
func unmarshalConfig[T any](configData json.RawMessage, appID string) *T {
	var cd T
	if err := json.Unmarshal(configData, &cd); err != nil {
		slog.Warn("failed to unmarshal config data", "appId", appID, "err", err)
	}
	return &cd
}

func buildAppConfig(appID string, status, provStatus, tcStatus int, configData json.RawMessage, odsaCtx *OdsaContext) *protocol.ApplicationConfig {
	switch appID {
	case config.AppIDVoWiFi:
		return BuildVoWiFiConfig(status, provStatus, tcStatus, unmarshalConfig[VoWiFiConfigData](configData, appID))
	case config.AppIDVoLTE:
		return BuildVoLTEConfig(status, provStatus, tcStatus, unmarshalConfig[VoLTEConfigData](configData, appID))
	case config.AppIDSMSoIP:
		return BuildSmsOipConfig(status, provStatus, tcStatus, unmarshalConfig[SmsOipConfigData](configData, appID))
	case config.AppIDODSACompanion:
		return BuildCompanionConfig(status, provStatus, tcStatus, unmarshalConfig[OdsaConfigData](configData, appID), odsaCtx)
	case config.AppIDODSAPrimary:
		return BuildPrimaryConfig(status, provStatus, tcStatus, unmarshalConfig[OdsaConfigData](configData, appID), odsaCtx)
	case config.AppIDDataPlanInfo:
		return BuildDataPlanConfig(status, provStatus, tcStatus, unmarshalConfig[DataPlanConfigData](configData, appID), odsaCtx)
	case config.AppIDServerInitiatedODSA:
		return BuildServerOdsaConfig(status, provStatus, tcStatus, unmarshalConfig[ServerOdsaConfigData](configData, appID), odsaCtx)
	case config.AppIDDirectCarrierBilling:
		return BuildDcbConfig(status, provStatus, tcStatus, unmarshalConfig[DcbConfigData](configData, appID))
	case config.AppIDPrivateUserIdentity:
		return BuildPrivateIdentityConfig(status, provStatus, tcStatus, unmarshalConfig[PrivateIdentityConfigData](configData, appID))
	case config.AppIDDeviceUserInfo:
		return BuildDeviceUserInfoConfig(status, provStatus, tcStatus, unmarshalConfig[DeviceUserInfoConfigData](configData, appID), odsaCtx)
	case config.AppIDAppAuthentication:
		return BuildAppAuthConfig(status, provStatus, tcStatus, unmarshalConfig[AppAuthConfigData](configData, appID))
	case config.AppIDSatelliteMode:
		return BuildSatModeConfig(status, provStatus, tcStatus, unmarshalConfig[SatModeConfigData](configData, appID))
	default:
		return &protocol.ApplicationConfig{
			AppID:             appID,
			EntitlementStatus: status,
			ProvStatus:        protocol.IntPtr(provStatus),
			TcStatus:          protocol.IntPtr(tcStatus),
		}
	}
}
