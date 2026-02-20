package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/hkassaei/poc-entitlements-as-a-service-go/internal/db"
)

// Compile-time interface compliance check.
var _ http.ResponseWriter = (*statusRecorder)(nil)

// AuditMiddleware logs requests to the audit_log table after the response is sent.
// Uses a detached context so audit writes are not canceled when the request ends.
func AuditMiddleware(queries *db.Queries) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Wrap the response writer to capture the status code
			wrapped := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(wrapped, r)

			// Capture values before launching goroutine (r is not safe after handler returns)
			ip := clientIP(r)
			ua := r.Header.Get("User-Agent")
			path := r.URL.Path
			method := r.Method
			code := wrapped.statusCode

			go func() {
				bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				err := queries.InsertAuditLog(bgCtx, &db.AuditLog{
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

func (sr *statusRecorder) WriteHeader(code int) {
	sr.statusCode = code
	sr.ResponseWriter.WriteHeader(code)
}
