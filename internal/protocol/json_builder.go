package protocol

import "strconv"

// BuildJSONResponse converts a ServiceEntitlementResponse to the GSMA TS.43 JSON format.
//
// Output format:
//
//	{
//	  "Vers": { "version": "1", "validity": "172800" },
//	  "Token": { "token": "..." },
//	  "ap2004": { "EntitlementStatus": "1", ... }
//	}
func BuildJSONResponse(response *ServiceEntitlementResponse) map[string]interface{} {
	result := map[string]interface{}{
		"Vers": map[string]string{
			"version":  response.Version,
			"validity": strconv.Itoa(response.Validity),
		},
		"Token": map[string]string{
			"token": response.Token,
		},
	}

	for _, app := range response.Applications {
		result[app.AppID] = buildApplicationBlock(&app)
	}

	return result
}

func buildApplicationBlock(app *ApplicationConfig) map[string]interface{} {
	block := map[string]interface{}{
		"EntitlementStatus": strconv.Itoa(app.EntitlementStatus),
	}

	if app.AddrStatus != nil {
		block["AddrStatus"] = strconv.Itoa(*app.AddrStatus)
	}

	if app.TcStatus != nil {
		block["TC_Status"] = strconv.Itoa(*app.TcStatus)
	}

	if app.ProvStatus != nil {
		block["ProvStatus"] = strconv.Itoa(*app.ProvStatus)
	}

	if app.ServiceFlowURL != "" {
		block["ServiceFlow_URL"] = app.ServiceFlowURL
	}

	if len(app.Addresses) > 0 {
		addrBlock := map[string]interface{}{}
		for i, addr := range app.Addresses {
			addrBlock[strconv.Itoa(i+1)] = map[string]string{
				"AddrType": addr.AddrType,
				"Addr":     addr.Addr,
			}
		}
		block["Addr"] = addrBlock
	}

	for k, v := range app.ExtraParams {
		block[k] = v
	}

	return block
}
