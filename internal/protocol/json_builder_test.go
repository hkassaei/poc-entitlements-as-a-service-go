package protocol

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var sampleResponse = &ServiceEntitlementResponse{
	Version:  "1",
	Validity: 172800,
	Token:    "test-token-abc123",
	Applications: []ApplicationConfig{
		{
			AppID:             "ap2004",
			EntitlementStatus: 1,
			AddrStatus:        IntPtr(1),
			TcStatus:          IntPtr(0),
			ProvStatus:        IntPtr(3),
			Addresses: []AddressConfig{
				{AddrType: "1", Addr: "epdg.operator.com"},
				{AddrType: "1", Addr: "pcscf.operator.com"},
			},
		},
	},
}

func TestBuildJSONResponse_EnvelopeStructure(t *testing.T) {
	result := BuildJSONResponse(sampleResponse)

	vers := result["Vers"].(map[string]string)
	assert.Equal(t, "1", vers["version"])
	assert.Equal(t, "172800", vers["validity"])

	tok := result["Token"].(map[string]string)
	assert.Equal(t, "test-token-abc123", tok["token"])

	assert.NotNil(t, result["ap2004"])
}

func TestBuildJSONResponse_ApplicationBlock(t *testing.T) {
	result := BuildJSONResponse(sampleResponse)
	app := result["ap2004"].(map[string]interface{})

	assert.Equal(t, "1", app["EntitlementStatus"])
	assert.Equal(t, "1", app["AddrStatus"])
	assert.Equal(t, "0", app["TC_Status"])
	assert.Equal(t, "3", app["ProvStatus"])
}

func TestBuildJSONResponse_AddressArray(t *testing.T) {
	result := BuildJSONResponse(sampleResponse)
	app := result["ap2004"].(map[string]interface{})
	addr := app["Addr"].(map[string]interface{})

	addr1 := addr["1"].(map[string]string)
	assert.Equal(t, "1", addr1["AddrType"])
	assert.Equal(t, "epdg.operator.com", addr1["Addr"])

	addr2 := addr["2"].(map[string]string)
	assert.Equal(t, "1", addr2["AddrType"])
	assert.Equal(t, "pcscf.operator.com", addr2["Addr"])
}

func TestBuildJSONResponse_OmitsAddressesWhenNotPresent(t *testing.T) {
	response := &ServiceEntitlementResponse{
		Version:  "1",
		Validity: 172800,
		Token:    "test-token",
		Applications: []ApplicationConfig{
			{AppID: "ap2004", EntitlementStatus: 0, AddrStatus: IntPtr(0)},
		},
	}

	result := BuildJSONResponse(response)
	app := result["ap2004"].(map[string]interface{})

	_, hasAddr := app["Addr"]
	assert.False(t, hasAddr)
}

func TestBuildJSONResponse_ExtraParams(t *testing.T) {
	response := &ServiceEntitlementResponse{
		Version:  "1",
		Validity: 172800,
		Token:    "test-token",
		Applications: []ApplicationConfig{
			{
				AppID:             "ap2003",
				EntitlementStatus: 1,
				ExtraParams:       map[string]string{"VoLTE_Entitled": "1", "VoNR_Entitled": "1"},
			},
		},
	}

	result := BuildJSONResponse(response)
	app := result["ap2003"].(map[string]interface{})

	assert.Equal(t, "1", app["VoLTE_Entitled"])
	assert.Equal(t, "1", app["VoNR_Entitled"])
}

func TestBuildJSONResponse_ServiceFlowURL(t *testing.T) {
	response := &ServiceEntitlementResponse{
		Version:  "1",
		Validity: 172800,
		Token:    "test-token",
		Applications: []ApplicationConfig{
			{
				AppID:             "ap2004",
				EntitlementStatus: 0,
				ServiceFlowURL:    "https://operator.com/terms",
			},
		},
	}

	result := BuildJSONResponse(response)
	app := result["ap2004"].(map[string]interface{})

	require.Equal(t, "https://operator.com/terms", app["ServiceFlow_URL"])
}
