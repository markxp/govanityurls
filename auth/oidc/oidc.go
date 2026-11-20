package oidc

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"cloud.google.com/go/auth/credentials/idtoken"
)

type slogLimitedLogger interface {
	LogAttrs(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr)
}

var (
	// ErrMissingToken is returned when the Authorization Bearer token is missing or malformed.
	ErrMissingToken = errors.New("missing or malformed bearer token")
	// ErrInvalidIssuer is returned when the token issuer is not Google Accounts.
	ErrInvalidIssuer = errors.New("invalid oidc token issuer")
	// ErrUnauthorizedEmail is returned when the token's email claim is not in the allowlist.
	ErrUnauthorizedEmail = errors.New("token email is not authorized")
	// ErrMissingAudience is returned when the validator is configured without an expected audience.
	ErrMissingAudience = errors.New("oidc audience is not configured")
)

// Validator validates Google OIDC ID tokens from Authorization Bearer headers (e.g. Cloud Tasks).
type Validator struct {
	audience      string
	allowedEmails []string
	logger        slogLimitedLogger
	validateJWT   func(ctx context.Context, token, audience string) (*idtoken.Payload, error)
}

// Option configures a Validator.
type Option func(*Validator)

// WithLogger sets a custom logger for the Validator.
func WithLogger(logger slogLimitedLogger) Option {
	return func(v *Validator) {
		if logger != nil {
			v.logger = logger
		}
	}
}

// WithJWTValidator overrides the JWT validation function (primarily for testing).
func WithJWTValidator(fn func(ctx context.Context, token, audience string) (*idtoken.Payload, error)) Option {
	return func(v *Validator) {
		if fn != nil {
			v.validateJWT = fn
		}
	}
}

// NewValidator creates a Validator that checks Google-signed OIDC tokens against audience and allowedEmails.
// Both audience and at least one allowed email are required; requests fail closed if either is empty.
func NewValidator(audience string, allowedEmails []string, opts ...Option) *Validator {
	var normalizedEmails []string
	for _, email := range allowedEmails {
		if trimmed := strings.TrimSpace(email); trimmed != "" {
			normalizedEmails = append(normalizedEmails, trimmed)
		}
	}

	v := &Validator{
		audience:      strings.TrimSpace(audience),
		allowedEmails: normalizedEmails,
		logger:        slog.Default(),
		validateJWT:   idtoken.Validate,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(v)
		}
	}
	return v
}

// ValidateRequest extracts and validates the Bearer OIDC token from the HTTP request.
func (v *Validator) ValidateRequest(r *http.Request) (*idtoken.Payload, error) {
	if v.audience == "" {
		return nil, ErrMissingAudience
	}

	authHeader := r.Header.Get("Authorization")
	if len(authHeader) <= 7 || !strings.EqualFold(authHeader[:7], "Bearer ") {
		return nil, ErrMissingToken
	}
	token := strings.TrimSpace(authHeader[7:])
	if token == "" || strings.ContainsAny(token, " \t\r\n") {
		return nil, ErrMissingToken
	}

	payload, err := v.validateJWT(r.Context(), token, v.audience)
	if err != nil {
		return nil, err
	}
	if payload == nil || (payload.Issuer != "https://accounts.google.com" && payload.Issuer != "accounts.google.com") {
		return nil, ErrInvalidIssuer
	}

	email, _ := payload.Claims["email"].(string)
	emailVerified, _ := payload.Claims["email_verified"].(bool)
	if email == "" || !emailVerified || !slices.Contains(v.allowedEmails, email) {
		return nil, ErrUnauthorizedEmail
	}

	return payload, nil
}

// Middleware returns an HTTP middleware that enforces valid OIDC Bearer tokens before calling next.
func (v *Validator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := v.ValidateRequest(r); err != nil {
			v.logger.LogAttrs(r.Context(), slog.LevelWarn, "OIDC token validation failed", slog.String("error", err.Error()))
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
