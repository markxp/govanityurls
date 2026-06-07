package iap

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"cloud.google.com/go/auth/credentials/idtoken"
)

type mockLogger struct {
	loggedMsgs []string
}

func (m *mockLogger) LogAttrs(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	m.loggedMsgs = append(m.loggedMsgs, msg)
}

func TestIAP(t *testing.T) {
	audience := "test-audience"

	t.Run("Valid Token", func(t *testing.T) {
		logger := &mockLogger{}
		iap := NewIAP(audience, logger)
		iap.validateJWT = func(ctx context.Context, token, aud string) (*idtoken.Payload, error) {
			if token != "valid-token" {
				return nil, errors.New("invalid token")
			}
			if aud != audience {
				return nil, errors.New("audience mismatch")
			}
			return &idtoken.Payload{Subject: "test-user"}, nil
		}

		req := httptest.NewRequest("GET", "/_admin", nil)
		req.Header.Set("x-goog-iap-jwt-assertion", "valid-token")

		payload, err := iap.ValidateIAPToken(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if payload.Subject != "test-user" {
			t.Errorf("expected subject test-user, got %s", payload.Subject)
		}

		// Test middleware
		called := false
		handler := iap.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		}))

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
		if !called {
			t.Error("expected handler to be called")
		}
	})

	t.Run("Missing Header", func(t *testing.T) {
		logger := &mockLogger{}
		iap := NewIAP(audience, logger)
		req := httptest.NewRequest("GET", "/_admin", nil)

		_, err := iap.ValidateIAPToken(req)
		if !errors.Is(err, IAPHeaderNotExist) {
			t.Errorf("expected error %v, got %v", IAPHeaderNotExist, err)
		}

		handler := iap.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rr.Code)
		}

		if len(logger.loggedMsgs) != 1 || logger.loggedMsgs[0] != "IAP header is not present" {
			t.Errorf("expected log msg 'IAP header is not present', got %v", logger.loggedMsgs)
		}
	})

	t.Run("Invalid Token Signature", func(t *testing.T) {
		logger := &mockLogger{}
		iap := NewIAP(audience, logger)
		iap.validateJWT = func(ctx context.Context, token, aud string) (*idtoken.Payload, error) {
			return nil, errors.New("invalid signature")
		}

		req := httptest.NewRequest("GET", "/_admin", nil)
		req.Header.Set("x-goog-iap-jwt-assertion", "bad-token")

		_, err := iap.ValidateIAPToken(req)
		if err == nil || err.Error() != "invalid signature" {
			t.Errorf("expected signature error, got %v", err)
		}

		handler := iap.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rr.Code)
		}

		if len(logger.loggedMsgs) != 1 || logger.loggedMsgs[0] != "Failed to validate IAP token: invalid signature. JWT=bad-token" {
			t.Errorf("expected validation failure log msg, got %v", logger.loggedMsgs)
		}
	})
}
