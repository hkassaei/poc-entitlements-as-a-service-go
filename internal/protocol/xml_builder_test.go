package protocol

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildXMLResponse_ValidWAPProvisioning(t *testing.T) {
	xml := BuildXMLResponse(sampleResponse)

	assert.Contains(t, xml, `<?xml version="1.0"?>`)
	assert.Contains(t, xml, `<wap-provisioningdoc version="1.1">`)
	assert.Contains(t, xml, `</wap-provisioningdoc>`)
}

func TestBuildXMLResponse_VERSCharacteristic(t *testing.T) {
	xml := BuildXMLResponse(sampleResponse)

	assert.Contains(t, xml, `<characteristic type="VERS">`)
	assert.Contains(t, xml, `name="version" value="1"`)
	assert.Contains(t, xml, `name="validity" value="172800"`)
}

func TestBuildXMLResponse_TOKENCharacteristic(t *testing.T) {
	xml := BuildXMLResponse(sampleResponse)

	assert.Contains(t, xml, `<characteristic type="TOKEN">`)
	assert.Contains(t, xml, `name="token" value="test-token-abc123"`)
}

func TestBuildXMLResponse_APPLICATIONCharacteristic(t *testing.T) {
	xml := BuildXMLResponse(sampleResponse)

	assert.Contains(t, xml, `<characteristic type="APPLICATION">`)
	assert.Contains(t, xml, `name="AppID" value="ap2004"`)
	assert.Contains(t, xml, `name="EntitlementStatus" value="1"`)
	assert.Contains(t, xml, `name="TC_Status" value="0"`)
	assert.Contains(t, xml, `name="ProvStatus" value="3"`)
}

func TestBuildXMLResponse_ADDRSubCharacteristics(t *testing.T) {
	xml := BuildXMLResponse(sampleResponse)

	assert.Contains(t, xml, `<characteristic type="ADDR">`)
	assert.Contains(t, xml, `name="AddrType" value="1"`)
	assert.Contains(t, xml, `name="Addr" value="epdg.operator.com"`)
	assert.Contains(t, xml, `name="Addr" value="pcscf.operator.com"`)
}

func TestBuildXMLResponse_EscapesSpecialChars(t *testing.T) {
	response := &ServiceEntitlementResponse{
		Version:      "1",
		Validity:     172800,
		Token:        `token&with<special>"chars`,
		Applications: []ApplicationConfig{},
	}

	xml := BuildXMLResponse(response)

	assert.Contains(t, xml, `token&amp;with&lt;special&gt;&quot;chars`)
	assert.False(t, strings.Contains(xml, `token&with<special>"chars`))
}
