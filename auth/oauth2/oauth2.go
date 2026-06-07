package oauth2

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"cloud.google.com/go/auth/credentials/idtoken"
	"golang.org/x/oauth2"
)

type slogLimitedLogger interface {
	LogAttrs(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr)
}

// Payload represents the claims extracted from an ID token.
// oauth2.go
type Payload struct {
	Issuer   string
	Audience string
	Subject  string
	Claims   map[string]any
}

// options contains the configuration for the OAuth2 middleware.
type options struct {
	AllowedEmails  map[string]bool
	AllowedDomains map[string]bool
	AuthCheck      func(payload *Payload) bool
	CookieName     string
	SecureCookie   bool
	StateSecret    []byte
	Logger         slogLimitedLogger
	validateJWT    func(ctx context.Context, token, audience string) (*Payload, error)
	exchange       func(ctx context.Context, code string) (*oauth2.Token, error)
}

// Option configures the OAuth2 middleware.
type Option func(*options)

// WithAllowedEmails restricts access to specific email addresses.
func WithAllowedEmails(emails []string) Option {
	return func(o *options) {
		o.AllowedEmails = make(map[string]bool, len(emails))
		for _, e := range emails {
			o.AllowedEmails[e] = true
		}
	}
}

// WithAllowedDomains restricts access to specific email domains.
func WithAllowedDomains(domains []string) Option {
	return func(o *options) {
		o.AllowedDomains = make(map[string]bool, len(domains))
		for _, d := range domains {
			o.AllowedDomains[d] = true
		}
	}
}

// WithAuthCheck specifies a custom authorization check callback.
func WithAuthCheck(check func(payload *Payload) bool) Option {
	return func(o *options) {
		o.AuthCheck = check
	}
}

// WithCookieName overrides the session cookie name.
func WithCookieName(name string) Option {
	return func(o *options) {
		o.CookieName = name
	}
}

// WithSecureCookie forces the secure cookie flag.
func WithSecureCookie(secure bool) Option {
	return func(o *options) {
		o.SecureCookie = secure
	}
}

// WithLogger configures a custom logger.
func WithLogger(logger slogLimitedLogger) Option {
	return func(o *options) {
		o.Logger = logger
	}
}

// WithJWTValidator configures a custom JWT validator (primarily for testing or other OIDC providers).
func WithJWTValidator(validator func(ctx context.Context, token, audience string) (*Payload, error)) Option {
	return func(o *options) {
		o.validateJWT = validator
	}
}

// WithTokenExchanger configures a custom token exchanger (primarily for testing).
func WithTokenExchanger(exchanger func(ctx context.Context, code string) (*oauth2.Token, error)) Option {
	return func(o *options) {
		o.exchange = exchanger
	}
}

// Middleware is an HTTP middleware.
type Middleware = func(http.Handler) http.Handler

// oauthState represents the components of the OAuth2 state parameter.
//
// The state is formatted as a dot-separated string: "<nonce>.<redirectB64>.<signature>".
//   - <nonce> is a 32-character hex-encoded string of 16 cryptographically random bytes,
//     ensuring request uniqueness and protection against CSRF (Cross-Site Request Forgery).
//   - <redirectB64> is the URL-safe base64 raw encoded landing path that the user originally requested
//     prior to authentication redirect.
//   - <signature> is the hex-encoded HMAC-SHA256 signature generated over the string "<nonce>:<redirectB64>"
//     using the configured StateSecret.
//
// The state parameter is passed as the "state" query parameter in the authorization redirect flow,
// and it is stored in the "oauth_state" cookie on the client browser. Upon callback execution,
// the cookie value must match the "state" query parameter exactly, and the signature is validated to prevent
// tampering or open redirect attacks.
type oauthState struct {
	nonce       string // 16-byte hex-encoded random nonce
	redirectB64 string // Raw URLEncoding base64 of the original landing/redirect path
	signature   string // HMAC-SHA256 signature of "<nonce>:<redirectB64>"
}

// New creates a new OAuth2 middleware handler, along with login and callback handlers.
// The stateSecret parameter must be a cryptographically secure key (ideally 32 bytes)
// used to sign and verify OAuth2 state tokens.
func New(oauthConfig *oauth2.Config, stateSecret []byte, loginPath, defaultLandingPath string, opts ...Option) (m Middleware, login http.Handler, callback http.Handler) {
	if oauthConfig == nil {
		panic("oauth2: oauthConfig must not be nil")
	}
	if len(stateSecret) == 0 {
		panic("oauth2: stateSecret must not be empty")
	}

	// default values
	o := &options{
		CookieName:  "session",
		StateSecret: stateSecret,
		Logger:      slog.Default(),
		validateJWT: func(ctx context.Context, token, audience string) (*Payload, error) {
			googlePayload, err := idtoken.Validate(ctx, token, audience)
			if err != nil {
				return nil, err
			}
			return &Payload{
				Issuer:   googlePayload.Issuer,
				Audience: googlePayload.Audience,
				Subject:  googlePayload.Subject,
				Claims:   googlePayload.Claims,
			}, nil
		},
		exchange: func(ctx context.Context, code string) (*oauth2.Token, error) {
			return oauthConfig.Exchange(ctx, code)
		},
	}

	for _, opt := range opts {
		opt(o)
	}

	// generateState constructs a signed state string containing a random, one-time use nonce
	// and the original request redirect destination path, formatted as described in the oauthState type.
	generateState := func(redirect string) (string, error) {
		nonceBytes := make([]byte, 16)
		if _, err := rand.Read(nonceBytes); err != nil {
			return "", err
		}
		nonce := hex.EncodeToString(nonceBytes)

		redirectB64 := base64.RawURLEncoding.EncodeToString([]byte(redirect))

		h := hmac.New(sha256.New, o.StateSecret)
		// We sign both the random nonce and the destination path together. If we only signed
		// the redirect path, the signature would be static and deterministic for a given destination,
		// rendering the CSRF protection (which relies on the random nonce) ineffective against replay attacks.
		h.Write([]byte(nonce + ":" + redirectB64))
		signature := hex.EncodeToString(h.Sum(nil))

		return nonce + "." + redirectB64 + "." + signature, nil
	}

	// verifyState parses, checks, and validates the signature of the state token.
	// It reads the 16-byte hex-encoded nonce, the base64-encoded destination, and verifies the HMAC-SHA256 signature.
	// If the signature matches, it returns the decoded original redirect path and true.
	verifyState := func(state string) (string, bool) {
		parts := strings.Split(state, ".")
		if len(parts) != 3 {
			return "", false
		}
		nonce, redirectB64, signature := parts[0], parts[1], parts[2]

		h := hmac.New(sha256.New, o.StateSecret)
		// Verify signature over both the nonce and the redirect path to ensure they haven't been tampered with
		// or replayed under a different context.
		h.Write([]byte(nonce + ":" + redirectB64))
		expectedSignature := hex.EncodeToString(h.Sum(nil))

		if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
			return "", false
		}

		redirectBytes, err := base64.RawURLEncoding.DecodeString(redirectB64)
		if err != nil {
			return "", false
		}
		return string(redirectBytes), true
	}

	isAuthorized := func(payload *Payload) bool {
		if o.AuthCheck != nil {
			return o.AuthCheck(payload)
		}

		email, _ := payload.Claims["email"].(string)
		if email == "" {
			return len(o.AllowedEmails) == 0 && len(o.AllowedDomains) == 0
		}

		if len(o.AllowedEmails) > 0 && o.AllowedEmails[email] {
			return true
		}

		if len(o.AllowedDomains) > 0 {
			if idx := strings.LastIndex(email, "@"); idx != -1 {
				domain := email[idx+1:]
				if o.AllowedDomains[domain] {
					return true
				}
			}
		}

		if len(o.AllowedEmails) > 0 || len(o.AllowedDomains) > 0 {
			return false
		}

		return true
	}

	login = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirect := r.URL.Query().Get("redirect")
		state, err := generateState(redirect)
		if err != nil {
			o.Logger.LogAttrs(r.Context(), slog.LevelError, "Failed to generate oauth state", slog.String("error", err.Error()))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		secure := o.SecureCookie
		if r.URL.Scheme == "https" || r.Header.Get("X-Forwarded-Proto") == "https" {
			secure = true
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "oauth_state",
			Value:    state,
			Path:     "/",
			MaxAge:   300, // 5 minutes
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})

		url := oauthConfig.AuthCodeURL(state)
		http.Redirect(w, r, url, http.StatusTemporaryRedirect)
	})

	callback = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("oauth_state")
		if err != nil || cookie.Value == "" {
			o.Logger.LogAttrs(r.Context(), slog.LevelWarn, "OAuth callback failed: state cookie missing")
			http.Error(w, "State invalid or expired", http.StatusBadRequest)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:   "oauth_state",
			Value:  "",
			Path:   "/",
			MaxAge: -1,
		})

		redirect, ok := verifyState(cookie.Value)
		if !ok {
			o.Logger.LogAttrs(r.Context(), slog.LevelWarn, "OAuth callback failed: cookie state signature invalid")
			http.Error(w, "State invalid or expired", http.StatusBadRequest)
			return
		}

		stateParam := r.FormValue("state")
		if stateParam != cookie.Value {
			o.Logger.LogAttrs(r.Context(), slog.LevelWarn, "OAuth callback failed: state parameter mismatch")
			http.Error(w, "State invalid or expired", http.StatusBadRequest)
			return
		}

		code := r.FormValue("code")
		if code == "" {
			http.Error(w, "CodeloginFn is missing", http.StatusBadRequest)
			return
		}

		token, err := o.exchange(r.Context(), code)
		if err != nil {
			o.Logger.LogAttrs(r.Context(), slog.LevelError, "OAuth code exchange failed", slog.String("error", err.Error()))
			http.Error(w, "Code exchange failed", http.StatusInternalServerError)
			return
		}

		idTokenRaw, ok := token.Extra("id_token").(string)
		if !ok || idTokenRaw == "" {
			o.Logger.LogAttrs(r.Context(), slog.LevelError, "OAuth token response missing id_token")
			http.Error(w, "Missing ID token in exchange", http.StatusInternalServerError)
			return
		}

		payload, err := o.validateJWT(r.Context(), idTokenRaw, oauthConfig.ClientID)
		if err != nil {
			o.Logger.LogAttrs(r.Context(), slog.LevelError, "OAuth callback token validation failed", slog.String("error", err.Error()))
			http.Error(w, "Invalid ID token", http.StatusUnauthorized)
			return
		}

		if !isAuthorized(payload) {
			o.Logger.LogAttrs(r.Context(), slog.LevelWarn, "OAuth access denied during callback: unauthorized identity", slog.String("sub", payload.Subject))
			http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
			return
		}

		secure := o.SecureCookie
		if r.URL.Scheme == "https" || r.Header.Get("X-Forwarded-Proto") == "https" {
			secure = true
		}

		http.SetCookie(w, &http.Cookie{
			Name:     o.CookieName,
			Value:    idTokenRaw,
			Path:     "/",
			MaxAge:   3600 * 24, // 24 hours
			HttpOnly: true,
			Secure:   secure,
			SameSite: http.SameSiteLaxMode,
		})

		redirectPath := defaultLandingPath
		if redirect != "" && strings.HasPrefix(redirect, "/") && !strings.HasPrefix(redirect, "//") {
			redirectPath = redirect
		}
		http.Redirect(w, r, redirectPath, http.StatusSeeOther)
	})

	m = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var tokenStr string
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
			}

			isCookieSession := false
			if tokenStr == "" {
				cookie, err := r.Cookie(o.CookieName)
				if err == nil && cookie.Value != "" {
					tokenStr = cookie.Value
					isCookieSession = true
				}
			}

			if tokenStr == "" {
				if r.Method == http.MethodGet {
					loginRedirect := loginPath
					if r.URL.Path != "" {
						loginRedirect += "?redirect=" + url.QueryEscape(r.URL.Path)
					}
					http.Redirect(w, r, loginRedirect, http.StatusTemporaryRedirect)
					return
				}
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			payload, err := o.validateJWT(r.Context(), tokenStr, oauthConfig.ClientID)
			if err != nil {
				o.Logger.LogAttrs(r.Context(), slog.LevelInfo, "OAuth token validation failed", slog.String("error", err.Error()), slog.String("path", r.URL.Path))
				if isCookieSession {
					http.SetCookie(w, &http.Cookie{
						Name:   o.CookieName,
						Value:  "",
						Path:   "/",
						MaxAge: -1,
					})
				}
				if r.Method == http.MethodGet {
					loginRedirect := loginPath
					if r.URL.Path != "" {
						loginRedirect += "?redirect=" + url.QueryEscape(r.URL.Path)
					}
					http.Redirect(w, r, loginRedirect, http.StatusTemporaryRedirect)
					return
				}
				http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
				return
			}

			if !isAuthorized(payload) {
				o.Logger.LogAttrs(r.Context(), slog.LevelWarn, "OAuth access denied: unauthorized identity", slog.String("sub", payload.Subject))
				http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}

	return m, login, callback
}
