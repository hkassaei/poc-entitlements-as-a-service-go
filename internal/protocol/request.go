package protocol

// EntitlementRequest holds the validated fields from an entitlement request.
// Fields are populated from POST body or GET query parameters.
type EntitlementRequest struct {
	App                string `json:"app" validate:"required,oneof=ap2003 ap2004 ap2005 ap2006 ap2009 ap2010 ap2011 ap2012 ap2013 ap2014 ap2015 ap2016"`
	TerminalID         string `json:"terminal_id" validate:"required,min=14,max=16"`
	EntitlementVersion string `json:"entitlement_version" validate:"required,numeric"`
	IMSI               string `json:"imsi,omitempty" validate:"omitempty,len=15,numeric"`
	IMEI               string `json:"imei,omitempty" validate:"omitempty,min=14,max=16,numeric"`
	TerminalVendor     string `json:"terminal_vendor,omitempty"`
	TerminalModel      string `json:"terminal_model,omitempty"`
	TerminalType       string `json:"terminal_type,omitempty"`
	Token              string `json:"token,omitempty"`
	EapRelay           string `json:"eap_relay,omitempty"`
	Operation          string `json:"operation,omitempty" validate:"omitempty,oneof=CheckEligibility ManageSubscription ManageService AcquireConfiguration AcquireTemporaryToken GetOperatorToken AcquirePlan"`
	OperationType      int    `json:"operation_type,omitempty"`
	AcceptContentType  string `json:"accept_content_type,omitempty" validate:"omitempty,oneof=xml json"`
}
