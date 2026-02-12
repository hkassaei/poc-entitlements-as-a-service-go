package server

import (
	"log/slog"
	"net/http"

	"github.com/hkassaei/entitlements-as-a-service-poc-go/internal/db"
)

// AuditMiddleware logs requests to the audit_log table after the response is sent.
// Fire-and-forget pattern — does not block the response.
func AuditMiddleware(queries *db.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Wrap the response writer to capture the status code
			wrapped := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(wrapped, r)

			// Fire and forget audit log
			go func() {
				ip := clientIP(r)
				ua := r.Header.Get("User-Agent")
				path := r.URL.Path
				method := r.Method
				code := wrapped.statusCode

				err := queries.InsertAuditLog(r.Context(), &db.AuditLog{
					ClientIP:     &ip,
					UserAgent:    &ua,
					Operation:    &method,
					ResponseCode: &code,
					AppID:        &path,
				})
				if err != nil {
					slog.Error("Failed to write audit log", "err", err)
				}
			}()
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}
