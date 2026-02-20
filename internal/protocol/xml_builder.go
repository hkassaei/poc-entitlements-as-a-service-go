package protocol

import (
	"strconv"
	"strings"
)

// BuildXMLResponse converts a ServiceEntitlementResponse to WAP-Provisioning XML format.
func BuildXMLResponse(response *ServiceEntitlementResponse) string {
	lines := []string{
		`<?xml version="1.0"?>`,
		`<wap-provisioningdoc version="1.1">`,
	}

	// VERS characteristic
	lines = append(lines, `  <characteristic type="VERS">`)
	lines = append(lines, parm("version", response.Version))
	lines = append(lines, parm("validity", strconv.Itoa(response.Validity)))
	lines = append(lines, `  </characteristic>`)

	// TOKEN characteristic
	lines = append(lines, `  <characteristic type="TOKEN">`)
	lines = append(lines, parm("token", response.Token))
	lines = append(lines, `  </characteristic>`)

	// APPLICATION characteristics
	for _, app := range response.Applications {
		lines = append(lines, buildApplicationCharacteristic(&app)...)
	}

	lines = append(lines, `</wap-provisioningdoc>`)
	return strings.Join(lines, "\n")
}

func escapeXML(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	value = strings.ReplaceAll(value, "\"", "&quot;")
	return value
}

func parm(name, value string) string {
	return `    <parm name="` + escapeXML(name) + `" value="` + escapeXML(value) + `"/>`
}

func parmIndent(name, value, indent string) string {
	return indent + `<parm name="` + escapeXML(name) + `" value="` + escapeXML(value) + `"/>`
}

func buildApplicationCharacteristic(app *ApplicationConfig) []string {
	indent := "    "
	lines := []string{
		`  <characteristic type="APPLICATION">`,
		parm("AppID", app.AppID),
		parm("EntitlementStatus", strconv.Itoa(app.EntitlementStatus)),
	}

	if app.AddrStatus != nil {
		lines = append(lines, parm("AddrStatus", strconv.Itoa(*app.AddrStatus)))
	}

	if app.TcStatus != nil {
		lines = append(lines, parm("TC_Status", strconv.Itoa(*app.TcStatus)))
	}

	if app.ProvStatus != nil {
		lines = append(lines, parm("ProvStatus", strconv.Itoa(*app.ProvStatus)))
	}

	if app.ServiceFlowURL != "" {
		lines = append(lines, parm("ServiceFlow_URL", app.ServiceFlowURL))
	}

	for k, v := range app.ExtraParams {
		lines = append(lines, parm(k, v))
	}

	for _, addr := range app.Addresses {
		lines = append(lines, indent+`<characteristic type="ADDR">`)
		lines = append(lines, parmIndent("AddrType", addr.AddrType, indent+"  "))
		lines = append(lines, parmIndent("Addr", addr.Addr, indent+"  "))
		lines = append(lines, indent+`</characteristic>`)
	}

	lines = append(lines, `  </characteristic>`)
	return lines
}
