package oidc

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"cloud.google.com/go/auth/credentials/idtoken"
)

func TestValidatorMiddleware(t *testing.T) {
	const testAudience = "https://go.example.com/_internal/registerRepo"
	const allowedSA = "cloud-tasks@my-project.iam.gserviceaccount.com"
	const secondAllowedSA = "cloud-scheduler@my-project.iam.gserviceaccount.com"

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	validValidateFn := func(issuer, email string) func(ctx context.Context, token, audience string) (*idtoken.Payload, error) {
		return func(ctx context.Context, token, audience string) (*idtoken.Payload, error) {
			if token != "valid-token" || audience != testAudience {
				return nil, errors.New("unexpected token or audience")
			}
			return &idtoken.Payload{
				Issuer:   issuer,
				Audience: audience,
				Claims: map[string]any{
					"email":          email,
					"email_verified": true,
				},
			}, nil
		}
	}

	tests := []struct {
		name          string
		audience      string
		allowedEmails []string
		authHeader    string
		validateFn    func(ctx context.Context, token, audience string) (*idtoken.Payload, error)
		wantStatus    int
	}{
		{
			name:          "valid token and allowed service account (https issuer)",
			audience:      testAudience,
			allowedEmails: []string{allowedSA},
			authHeader:    "Bearer valid-token",
			validateFn:    validValidateFn("https://accounts.google.com", allowedSA),
			wantStatus:    http.StatusOK,
		},
		{
			name:          "valid token with legacy accounts.google.com issuer",
			audience:      testAudience,
			allowedEmails: []string{allowedSA},
			authHeader:    "Bearer valid-token",
			validateFn:    validValidateFn("accounts.google.com", allowedSA),
			wantStatus:    http.StatusOK,
		},
		{
			name:          "empty issuer rejected",
			audience:      testAudience,
			allowedEmails: []string{allowedSA},
			authHeader:    "Bearer valid-token",
			validateFn:    validValidateFn("", allowedSA),
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "IAP issuer rejected",
			audience:      testAudience,
			allowedEmails: []string{allowedSA},
			authHeader:    "Bearer valid-token",
			validateFn:    validValidateFn("https://cloud.google.com/iap", allowedSA),
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "untrusted issuer rejected",
			audience:      testAudience,
			allowedEmails: []string{allowedSA},
			authHeader:    "Bearer valid-token",
			validateFn:    validValidateFn("https://evil.example.com", allowedSA),
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "lowercase bearer prefix accepted",
			audience:      testAudience,
			allowedEmails: []string{allowedSA},
			authHeader:    "bearer valid-token",
			validateFn:    validValidateFn("https://accounts.google.com", allowedSA),
			wantStatus:    http.StatusOK,
		},
		{
			name:          "uppercase BEARER prefix accepted",
			audience:      testAudience,
			allowedEmails: []string{allowedSA},
			authHeader:    "BEARER valid-token",
			validateFn:    validValidateFn("https://accounts.google.com", allowedSA),
			wantStatus:    http.StatusOK,
		},
		{
			name:          "leading and trailing token spaces trimmed and accepted",
			audience:      testAudience,
			allowedEmails: []string{allowedSA},
			authHeader:    "Bearer   valid-token  ",
			validateFn:    validValidateFn("https://accounts.google.com", allowedSA),
			wantStatus:    http.StatusOK,
		},
		{
			name:          "missing authorization header",
			audience:      testAudience,
			allowedEmails: []string{allowedSA},
			authHeader:    "",
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "bare Bearer without space or token rejected",
			audience:      testAudience,
			allowedEmails: []string{allowedSA},
			authHeader:    "Bearer",
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "whitespace-only Bearer token rejected",
			audience:      testAudience,
			allowedEmails: []string{allowedSA},
			authHeader:    "Bearer    ",
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "internal whitespace in Bearer token rejected",
			audience:      testAudience,
			allowedEmails: []string{allowedSA},
			authHeader:    "Bearer tok1 tok2",
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "non-Bearer scheme rejected",
			audience:      testAudience,
			allowedEmails: []string{allowedSA},
			authHeader:    "Basic dXNlcjpwYXNz",
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "empty audience fails closed",
			audience:      "",
			allowedEmails: []string{allowedSA},
			authHeader:    "Bearer valid-token",
			validateFn:    validValidateFn("https://accounts.google.com", allowedSA),
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "whitespace-only audience fails closed",
			audience:      "   ",
			allowedEmails: []string{allowedSA},
			authHeader:    "Bearer valid-token",
			validateFn:    validValidateFn("https://accounts.google.com", allowedSA),
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "whitespace-padded audience and allowedEmails normalized and accepted",
			audience:      "  " + testAudience + "  ",
			allowedEmails: []string{"  " + allowedSA + "  "},
			authHeader:    "Bearer valid-token",
			validateFn:    validValidateFn("https://accounts.google.com", allowedSA),
			wantStatus:    http.StatusOK,
		},
		{
			name:          "empty allowedEmails fails closed",
			audience:      testAudience,
			allowedEmails: nil,
			authHeader:    "Bearer valid-token",
			validateFn:    validValidateFn("https://accounts.google.com", allowedSA),
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "whitespace-only allowedEmails fails closed",
			audience:      testAudience,
			allowedEmails: []string{"", "   "},
			authHeader:    "Bearer valid-token",
			validateFn:    validValidateFn("https://accounts.google.com", allowedSA),
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "multiple allowed service accounts accepts second SA",
			audience:      testAudience,
			allowedEmails: []string{allowedSA, secondAllowedSA},
			authHeader:    "Bearer valid-token",
			validateFn:    validValidateFn("https://accounts.google.com", secondAllowedSA),
			wantStatus:    http.StatusOK,
		},
		{
			name:          "unauthorized email rejected",
			audience:      testAudience,
			allowedEmails: []string{allowedSA},
			authHeader:    "Bearer valid-token",
			validateFn:    validValidateFn("https://accounts.google.com", "other-sa@other-project.iam.gserviceaccount.com"),
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "nil Claims map rejected",
			audience:      testAudience,
			allowedEmails: []string{allowedSA},
			authHeader:    "Bearer valid-token",
			validateFn: func(ctx context.Context, token, audience string) (*idtoken.Payload, error) {
				return &idtoken.Payload{
					Issuer:   "https://accounts.google.com",
					Audience: audience,
					Claims:   nil,
				}, nil
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:          "non-string email claim rejected",
			audience:      testAudience,
			allowedEmails: []string{allowedSA},
			authHeader:    "Bearer valid-token",
			validateFn: func(ctx context.Context, token, audience string) (*idtoken.Payload, error) {
				return &idtoken.Payload{
					Issuer:   "https://accounts.google.com",
					Audience: audience,
					Claims: map[string]any{
						"email":          123,
						"email_verified": true,
					},
				}, nil
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:          "string 'true' for email_verified rejected",
			audience:      testAudience,
			allowedEmails: []string{allowedSA},
			authHeader:    "Bearer valid-token",
			validateFn: func(ctx context.Context, token, audience string) (*idtoken.Payload, error) {
				return &idtoken.Payload{
					Issuer:   "https://accounts.google.com",
					Audience: audience,
					Claims: map[string]any{
						"email":          allowedSA,
						"email_verified": "true",
					},
				}, nil
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:          "unverified email rejected",
			audience:      testAudience,
			allowedEmails: []string{allowedSA},
			authHeader:    "Bearer valid-token",
			validateFn: func(ctx context.Context, token, audience string) (*idtoken.Payload, error) {
				return &idtoken.Payload{
					Issuer:   "https://accounts.google.com",
					Audience: audience,
					Claims: map[string]any{
						"email":          allowedSA,
						"email_verified": false,
					},
				}, nil
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:          "missing email_verified claim rejected",
			audience:      testAudience,
			allowedEmails: []string{allowedSA},
			authHeader:    "Bearer valid-token",
			validateFn: func(ctx context.Context, token, audience string) (*idtoken.Payload, error) {
				return &idtoken.Payload{
					Issuer:   "https://accounts.google.com",
					Audience: audience,
					Claims:   map[string]any{"email": allowedSA},
				}, nil
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:          "jwt validation error rejected",
			audience:      testAudience,
			allowedEmails: []string{allowedSA},
			authHeader:    "Bearer bad-token",
			validateFn: func(ctx context.Context, token, audience string) (*idtoken.Payload, error) {
				return nil, errors.New("invalid signature")
			},
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := NewValidator(tc.audience, tc.allowedEmails, WithJWTValidator(tc.validateFn))
			req := httptest.NewRequest(http.MethodPost, "/_internal/registerRepo", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rec := httptest.NewRecorder()
			v.Middleware(okHandler).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("got status %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

func TestAllowedEmailsSliceCloneIsolation(t *testing.T) {
	const testAudience = "https://go.example.com/_internal/registerRepo"
	callerSlice := []string{"initial@my-project.iam.gserviceaccount.com"}

	v := NewValidator(
		testAudience,
		callerSlice,
		WithJWTValidator(func(ctx context.Context, token, audience string) (*idtoken.Payload, error) {
			return &idtoken.Payload{
				Issuer:   "https://accounts.google.com",
				Audience: audience,
				Claims: map[string]any{
					"email":          "attacker@evil.iam.gserviceaccount.com",
					"email_verified": true,
				},
			}, nil
		}),
	)

	// Mutate caller's slice after NewValidator returns.
	callerSlice[0] = "attacker@evil.iam.gserviceaccount.com"

	req := httptest.NewRequest(http.MethodPost, "/_internal/registerRepo", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	if _, err := v.ValidateRequest(req); !errors.Is(err, ErrUnauthorizedEmail) {
		t.Fatalf("expected ErrUnauthorizedEmail after mutating caller slice, got %v", err)
	}
}

type capturedLog struct {
	level slog.Level
	msg   string
	attrs map[string]string
}

type mockLogger struct {
	records []capturedLog
}

func (l *mockLogger) LogAttrs(_ context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	m := make(map[string]string, len(attrs))
	for _, a := range attrs {
		m[a.Key] = a.Value.String()
	}
	l.records = append(l.records, capturedLog{level: level, msg: msg, attrs: m})
}

func TestOptionsAndLoggingAndErrors(t *testing.T) {
	const testAudience = "https://go.example.com/_internal/registerRepo"
	const allowedSA = "cloud-tasks@my-project.iam.gserviceaccount.com"

	t.Run("WithLogger(nil) and WithJWTValidator(nil) are safe no-ops", func(t *testing.T) {
		v := NewValidator(testAudience, []string{allowedSA}, WithLogger(nil), WithJWTValidator(nil), nil)
		if v.logger == nil {
			t.Fatal("expected non-nil default logger")
		}
		if v.validateJWT == nil {
			t.Fatal("expected non-nil default validateJWT")
		}

		// Middleware should not panic when logging a rejection with WithLogger(nil).
		req := httptest.NewRequest(http.MethodPost, "/_internal/registerRepo", nil)
		rec := httptest.NewRecorder()
		v.Middleware(http.NotFoundHandler()).ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("Custom logger captures warning attributes on rejection", func(t *testing.T) {
		logger := &mockLogger{}
		v := NewValidator(testAudience, []string{allowedSA}, WithLogger(logger))

		req := httptest.NewRequest(http.MethodPost, "/_internal/registerRepo", nil)
		rec := httptest.NewRecorder()
		v.Middleware(http.NotFoundHandler()).ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if len(logger.records) != 1 {
			t.Fatalf("logged %d records, want 1", len(logger.records))
		}
		recLog := logger.records[0]
		if recLog.level != slog.LevelWarn {
			t.Errorf("log level = %v, want WARN", recLog.level)
		}
		if recLog.msg != "OIDC token validation failed" {
			t.Errorf("log msg = %q, want %q", recLog.msg, "OIDC token validation failed")
		}
		if recLog.attrs["error"] != ErrMissingToken.Error() {
			t.Errorf("log error attr = %q, want %q", recLog.attrs["error"], ErrMissingToken.Error())
		}
	})

	t.Run("Context propagation and cancellation into validateJWT", func(t *testing.T) {
		type ctxKey struct{}
		ctx, cancel := context.WithCancel(context.WithValue(context.Background(), ctxKey{}, "trace-val"))
		cancel()

		var sawVal any
		var sawCanceled bool
		v := NewValidator(
			testAudience,
			[]string{allowedSA},
			WithJWTValidator(func(gotCtx context.Context, token, audience string) (*idtoken.Payload, error) {
				sawVal = gotCtx.Value(ctxKey{})
				sawCanceled = errors.Is(gotCtx.Err(), context.Canceled)
				return nil, gotCtx.Err()
			}),
		)

		req := httptest.NewRequest(http.MethodPost, "/_internal/registerRepo", nil).WithContext(ctx)
		req.Header.Set("Authorization", "Bearer valid-token")
		_, err := v.ValidateRequest(req)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
		if sawVal != "trace-val" || !sawCanceled {
			t.Errorf("context not propagated: val=%v, canceled=%v", sawVal, sawCanceled)
		}
	})

	t.Run("ValidateRequest sentinel errors via errors.Is", func(t *testing.T) {
		// 1. ErrMissingAudience
		vNoAud := NewValidator("", []string{allowedSA})
		req := httptest.NewRequest(http.MethodPost, "/_internal/registerRepo", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		if _, err := vNoAud.ValidateRequest(req); !errors.Is(err, ErrMissingAudience) {
			t.Errorf("expected ErrMissingAudience, got %v", err)
		}

		// 2. ErrMissingToken
		vMissingTok := NewValidator(testAudience, []string{allowedSA})
		reqNoTok := httptest.NewRequest(http.MethodPost, "/_internal/registerRepo", nil)
		if _, err := vMissingTok.ValidateRequest(reqNoTok); !errors.Is(err, ErrMissingToken) {
			t.Errorf("expected ErrMissingToken, got %v", err)
		}

		// 3. ErrInvalidIssuer
		vBadIss := NewValidator(
			testAudience,
			[]string{allowedSA},
			WithJWTValidator(func(ctx context.Context, token, audience string) (*idtoken.Payload, error) {
				return &idtoken.Payload{
					Issuer:   "https://cloud.google.com/iap",
					Audience: audience,
					Claims:   map[string]any{"email": allowedSA, "email_verified": true},
				}, nil
			}),
		)
		if _, err := vBadIss.ValidateRequest(req); !errors.Is(err, ErrInvalidIssuer) {
			t.Errorf("expected ErrInvalidIssuer, got %v", err)
		}

		// 4. ErrUnauthorizedEmail
		vUnauthEmail := NewValidator(
			testAudience,
			[]string{allowedSA},
			WithJWTValidator(func(ctx context.Context, token, audience string) (*idtoken.Payload, error) {
				return &idtoken.Payload{
					Issuer:   "https://accounts.google.com",
					Audience: audience,
					Claims:   map[string]any{"email": "wrong@example.com", "email_verified": true},
				}, nil
			}),
		)
		if _, err := vUnauthEmail.ValidateRequest(req); !errors.Is(err, ErrUnauthorizedEmail) {
			t.Errorf("expected ErrUnauthorizedEmail, got %v", err)
		}
	})
}
