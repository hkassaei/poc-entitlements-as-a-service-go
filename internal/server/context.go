package server

import "context"

type contextKey string

const (
	parsedUAKey contextKey = "parsedUA"
)

func withParsedUA(ctx context.Context, ua *ParsedUserAgent) context.Context {
	return context.WithValue(ctx, parsedUAKey, ua)
}

// GetParsedUA retrieves the parsed User-Agent from context.
func GetParsedUA(ctx context.Context) *ParsedUserAgent {
	ua, _ := ctx.Value(parsedUAKey).(*ParsedUserAgent)
	return ua
}
