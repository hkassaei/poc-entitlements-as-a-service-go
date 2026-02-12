package services

// ActivationCodeResponse holds a canned eSIM activation code.
type ActivationCodeResponse struct {
	ActivationCode string
	ICCID          string
	SMDPAddress    string
	ProfileType    string
	MatchingID     string
}

var profiles = map[string]ActivationCodeResponse{
	"default": {
		ActivationCode: "1$smdp.operator.com$POSTPAID-001",
		ICCID:          "8901010000000000001",
		SMDPAddress:    "smdp.operator.com",
		ProfileType:    "postpaid",
		MatchingID:     "POSTPAID-001",
	},
	"prepaid": {
		ActivationCode: "1$smdp.operator.com$PREPAID-001",
		ICCID:          "8901010000000000002",
		SMDPAddress:    "smdp.operator.com",
		ProfileType:    "prepaid",
		MatchingID:     "PREPAID-001",
	},
	"companion": {
		ActivationCode: "1$smdp.operator.com$COMPANION-001",
		ICCID:          "8901010000000000003",
		SMDPAddress:    "smdp.operator.com",
		ProfileType:    "companion",
		MatchingID:     "COMPANION-001",
	},
}

// GetActivationCode returns an activation code for a profile key.
// Falls back to the "default" profile if the key is unknown.
func GetActivationCode(profileKey string) ActivationCodeResponse {
	if p, ok := profiles[profileKey]; ok {
		return p
	}
	return profiles["default"]
}

// ListAvailableProfiles returns all canned profiles.
func ListAvailableProfiles() []ActivationCodeResponse {
	result := make([]ActivationCodeResponse, 0, len(profiles))
	for _, p := range profiles {
		result = append(result, p)
	}
	return result
}
