package protocol

// AddressConfig represents a network address in a TS.43 response.
type AddressConfig struct {
	AddrType string // "1" = FQDN, "2" = IPv4, "3" = IPv6
	Addr     string
}

// ApplicationConfig represents a single application's entitlement configuration.
type ApplicationConfig struct {
	AppID             string
	EntitlementStatus int
	AddrStatus        *int // nil = omitted
	TcStatus          *int
	ProvStatus        *int
	ServiceFlowURL    string
	Addresses         []AddressConfig
	ExtraParams       map[string]string
}

// ServiceEntitlementResponse is the internal representation of an entitlement response.
type ServiceEntitlementResponse struct {
	Version      string
	Validity     int // TTL in seconds
	Token        string
	Applications []ApplicationConfig
}

// IntPtr returns a pointer to an int. Convenience helper for optional fields.
func IntPtr(v int) *int {
	return &v
}
