package protocol

// AddressConfig represents a network address in a TS.43 response.
type AddressConfig struct {
	AddrType string `json:"addrType"` // "1" = FQDN, "2" = IPv4, "3" = IPv6
	Addr     string `json:"addr"`
}

// ApplicationConfig represents a single application's entitlement configuration.
type ApplicationConfig struct {
	AppID             string            `json:"appId"`
	EntitlementStatus int               `json:"entitlementStatus"`
	AddrStatus        *int              `json:"addrStatus,omitempty"` // nil = omitted
	TcStatus          *int              `json:"tcStatus,omitempty"`
	ProvStatus        *int              `json:"provStatus,omitempty"`
	ServiceFlowURL    string            `json:"serviceFlowUrl,omitempty"`
	Addresses         []AddressConfig   `json:"addresses,omitempty"`
	ExtraParams       map[string]string `json:"extraParams,omitempty"`
}

// ServiceEntitlementResponse is the internal representation of an entitlement response.
type ServiceEntitlementResponse struct {
	Version      string              `json:"version"`
	Validity     int                 `json:"validity"` // TTL in seconds
	Token        string              `json:"token"`
	Applications []ApplicationConfig `json:"applications"`
}

// IntPtr returns a pointer to an int. Convenience helper for optional fields.
func IntPtr(v int) *int {
	return &v
}
