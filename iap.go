package govanityurls

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"cloud.google.com/go/auth/credentials/idtoken"
)

// IAP validates IAP JWT tokens.
type IAP struct {
	audience string
	logger   slogLimitedLogger
}

// NewIAP creates a new IAP validator with the specified audience and logger.
func NewIAP(audience string, logger slogLimitedLogger) *IAP {
	if logger == nil {
		logger = slog.Default()
	}
	return &IAP{
		audience: audience,
		logger:   logger,
	}
}

var iapHeader = "x-goog-iap-jwt-assertion"

// IAPHeaderNotExist is returned when the IAP JWT header is not found in the request.
var IAPHeaderNotExist = errors.New("x-goog-iap-jwt-assertion header is not present")

// ValidateIAPToken validates the IAP JWT token in the request header against the configured audience.
func (iap *IAP) ValidateIAPToken(r *http.Request) (*idtoken.Payload, error) {
	iapJWT := r.Header.Get(iapHeader)
	if iapJWT == "" {
		return nil, IAPHeaderNotExist
	}
	return idtoken.Validate(r.Context(), iapJWT, iap.audience)
}

// Middleware returns an HTTP handler that validates the IAP JWT token before forwarding to the next handler.
func (iap *IAP) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := iap.ValidateIAPToken(r)
		if err != nil {
			if errors.Is(err, IAPHeaderNotExist) {
				iap.logger.LogAttrs(r.Context(), slog.LevelInfo, "IAP header is not present")
			} else {
				iap.logger.LogAttrs(r.Context(), slog.LevelInfo, fmt.Sprintf("Failed to validate IAP token: %v. JWT=%s", err.Error(), r.Header.Get(iapHeader)), slog.String("error", err.Error()))
			}
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
