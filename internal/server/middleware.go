package server

import (
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/config"
)

// ParsedUserAgent holds the parsed TS.43 User-Agent fields.
type ParsedUserAgent struct {
	Version    string
	Vendor     string
	Model      string
	ClientType string
	OS         string
}

// TS.43 User-Agent format: PRD-TS43/<version> (<vendor>; <model>; <client_type>; <OS>)
var _uaPattern = regexp.MustCompile(`^PRD-TS43/(\S+)\s+\(([^;]+);\s*([^;]+);\s*([^;]+);\s*([^)]+)\)$`)

// UserAgentParser middleware parses the TS.43 User-Agent header.
func UserAgentParser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if ua != "" {
			match := _uaPattern.FindStringSubmatch(ua)
			if match != nil {
				parsed := &ParsedUserAgent{
					Version:    match[1],
					Vendor:     strings.TrimSpace(match[2]),
					Model:      strings.TrimSpace(match[3]),
					ClientType: strings.TrimSpace(match[4]),
					OS:         strings.TrimSpace(match[5]),
				}
				r = r.WithContext(withParsedUA(r.Context(), parsed))
			}
		}
		next.ServeHTTP(w, r)
	})
}

// VersionCheck middleware validates entitlement_version against supported versions.
func VersionCheck(cfg config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var version string
			if r.Method == http.MethodGet {
				version = r.URL.Query().Get("entitlement_version")
			}
			// For POST, version check happens after body parsing in the route handler.
			// GET version check happens here.
			if version != "" {
				supported := false
				for _, v := range cfg.SupportedVersions {
					if v == version {
						supported = true
						break
					}
				}
				if !supported {
					writeJSON(w, http.StatusNotAcceptable, map[string]interface{}{
						"error":   "Not Acceptable",
						"message": "Unsupported entitlement_version: " + version + ". Supported: " + strings.Join(cfg.SupportedVersions, ", "),
					})
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RecoverMiddleware catches panics and returns 500.
func RecoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("Panic recovered", "err", err, "method", r.Method, "url", r.URL.String())
				writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
					"error":      "Internal Server Error",
					"message":    "An unexpected error occurred",
					"statusCode": 500,
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// RequestLogger logs incoming requests.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("Request", "method", r.Method, "path", r.URL.Path, "remoteAddr", r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}
