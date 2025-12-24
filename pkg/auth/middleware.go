package auth

import (
	"log"
	"net/http"
	"strings"
)

// AuthMiddleware returns HTTP middleware that validates Bearer tokens.
// Requires Authorization: Bearer <key> header on all requests.
func AuthMiddleware(store *Store, limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. Extract Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeAuthError(w, http.StatusUnauthorized, "Missing Authorization header")
				return
			}

			// 2. Parse Bearer token
			key := extractBearerToken(authHeader)
			if key == "" {
				writeAuthError(w, http.StatusUnauthorized, "Invalid Authorization header format. Expected: Bearer <key>")
				return
			}

			// 3. Validate key in database
			keyData, err := store.ValidateKey(key)
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, "Invalid or inactive API key")
				return
			}

			// 4. Check rate limit
			if !limiter.Allow(key, keyData.RequestsPerMinute) {
				w.Header().Set("X-RateLimit-Limit", string(rune(keyData.RequestsPerMinute)))
				w.Header().Set("Retry-After", "60") // Retry after 1 minute
				writeAuthError(w, http.StatusTooManyRequests, "Rate limit exceeded")

				// Log rate limit event
				store.LogRequest(key, r.URL.Path, http.StatusTooManyRequests)
				return
			}

			// 5. Log successful request (best-effort, don't fail if logging fails)
			go func() {
				if err := store.LogRequest(key, r.URL.Path, http.StatusOK); err != nil {
					log.Printf("Warning: Failed to log request: %v", err)
				}
			}()

			// 6. Request is authorized - continue to handler
			next.ServeHTTP(w, r)
		})
	}
}

// extractBearerToken extracts the token from Authorization: Bearer <token> header.
// Returns empty string if header format is invalid.
func extractBearerToken(authHeader string) string {
	// Expected format: "Bearer agsk_abc123..."
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 {
		return ""
	}

	if parts[0] != "Bearer" {
		return ""
	}

	return strings.TrimSpace(parts[1])
}

// writeAuthError writes a JSON error response for auth failures.
func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", "Bearer")
	w.WriteHeader(status)

	// Simple JSON error
	w.Write([]byte(`{"error":"` + message + `"}`))
}
