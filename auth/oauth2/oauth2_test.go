package oauth2

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

type mockLogger struct {
	loggedMsgs []string
}

func (m *mockLogger) LogAttrs(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	m.loggedMsgs = append(m.loggedMsgs, msg)
}

func signState(secret []byte, nonce string) string {
	redirectB64 := base64.RawURLEncoding.EncodeToString([]byte(""))
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(nonce + ":" + redirectB64))
	sig := hex.EncodeToString(h.Sum(nil))
	return nonce + "." + redirectB64 + "." + sig
}

func TestOAuth2_LoginRedirect(t *testing.T) {
	oauthConfig := &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "https://example.com/_admin/callback",
	}

	logger := &mockLogger{}
	secret := []byte("test-secret-key-32-bytes-long-!")
	_, loginHandler, _ := New(oauthConfig, secret, "/_admin/login", "/_admin", WithLogger(logger))

	req := httptest.NewRequest("GET", "/_admin/login", nil)
	rr := httptest.NewRecorder()

	loginHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTemporaryRedirect {
		t.Errorf("expected status 307, got %d", rr.Code)
	}

	loc := rr.Header().Get("Location")
	if loc == "" || !locContains(loc, "client_id=test-client-id") {
		t.Errorf("unexpected redirect Location: %s", loc)
	}

	// Verify state cookie is set
	cookies := rr.Result().Cookies()
	var stateCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "oauth_state" {
			stateCookie = c
			break
		}
	}

	if stateCookie == nil {
		t.Fatal("expected oauth_state cookie to be set")
	}
	if stateCookie.Value == "" {
		t.Error("expected oauth_state cookie to have a value")
	}
	if !stateCookie.HttpOnly {
		t.Error("expected oauth_state cookie to be HttpOnly")
	}
}

func TestOAuth2_CallbackSuccess(t *testing.T) {
	oauthConfig := &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "https://example.com/_admin/callback",
	}
	logger := &mockLogger{}
	secret := []byte("test-secret-key-32-bytes-long-!")

	exchangeMock := func(ctx context.Context, code string) (*oauth2.Token, error) {
		if code != "auth-code" {
			return nil, errors.New("invalid code")
		}
		tk := &oauth2.Token{}
		tk = tk.WithExtra(map[string]any{
			"id_token": "valid-id-token",
		})
		return tk, nil
	}

	validateJWTMock := func(ctx context.Context, token, audience string) (*Payload, error) {
		if token != "valid-id-token" {
			return nil, errors.New("invalid token")
		}
		if audience != "test-client-id" {
			return nil, errors.New("audience mismatch")
		}
		return &Payload{
			Subject: "user-123",
			Claims: map[string]any{
				"email": "admin@example.com",
			},
		}, nil
	}

	_, _, callbackHandler := New(
		oauthConfig,
		secret,
		"/_admin/login",
		"/_admin",
		WithLogger(logger),
		WithAllowedEmails([]string{"admin@example.com"}),
		WithTokenExchanger(exchangeMock),
		WithJWTValidator(validateJWTMock),
	)

	stateVal := signState(secret, "mystate")
	req := httptest.NewRequest("GET", "/_admin/callback?code=auth-code&state="+stateVal, nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: stateVal})

	rr := httptest.NewRecorder()
	callbackHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected status 303, got %d", rr.Code)
	}

	loc := rr.Header().Get("Location")
	if loc != "/_admin" {
		t.Errorf("expected redirect to /_admin, got %s", loc)
	}

	// Verify session cookie is set
	cookies := rr.Result().Cookies()
	var sessionCookie *http.Cookie
	var deletedStateCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session" {
			sessionCookie = c
		}
		if c.Name == "oauth_state" {
			deletedStateCookie = c
		}
	}

	if sessionCookie == nil {
		t.Fatal("expected session cookie to be set")
	}
	if sessionCookie.Value != "valid-id-token" {
		t.Errorf("expected session cookie value 'valid-id-token', got %s", sessionCookie.Value)
	}
	if !sessionCookie.HttpOnly {
		t.Error("expected session cookie to be HttpOnly")
	}

	if deletedStateCookie == nil || deletedStateCookie.MaxAge != -1 {
		t.Errorf("expected oauth_state cookie to be deleted, got: %+v", deletedStateCookie)
	}
}

func TestOAuth2_CallbackUnauthorizedEmail(t *testing.T) {
	oauthConfig := &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "https://example.com/_admin/callback",
	}
	logger := &mockLogger{}
	secret := []byte("test-secret-key-32-bytes-long-!")

	exchangeMock := func(ctx context.Context, code string) (*oauth2.Token, error) {
		tk := &oauth2.Token{}
		tk = tk.WithExtra(map[string]any{"id_token": "valid-id-token"})
		return tk, nil
	}

	validateJWTMock := func(ctx context.Context, token, audience string) (*Payload, error) {
		return &Payload{
			Subject: "user-456",
			Claims: map[string]any{
				"email": "stranger@example.com",
			},
		}, nil
	}

	_, _, callbackHandler := New(
		oauthConfig,
		secret,
		"/_admin/login",
		"/_admin",
		WithLogger(logger),
		WithAllowedEmails([]string{"admin@example.com"}),
		WithTokenExchanger(exchangeMock),
		WithJWTValidator(validateJWTMock),
	)

	stateVal := signState(secret, "mystate")
	req := httptest.NewRequest("GET", "/_admin/callback?code=auth-code&state="+stateVal, nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: stateVal})

	rr := httptest.NewRecorder()
	callbackHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rr.Code)
	}
}

func TestOAuth2_CallbackStateMismatch(t *testing.T) {
	oauthConfig := &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "https://example.com/_admin/callback",
	}
	logger := &mockLogger{}
	secret := []byte("test-secret-key-32-bytes-long-!")

	_, _, callbackHandler := New(oauthConfig, secret, "/_admin/login", "/_admin", WithLogger(logger))

	stateVal := signState(secret, "mystate")
	req := httptest.NewRequest("GET", "/_admin/callback?code=auth-code&state=wrongstate", nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: stateVal})

	rr := httptest.NewRecorder()
	callbackHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
}

func TestOAuth2_CallbackInvalidSignature(t *testing.T) {
	oauthConfig := &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "https://example.com/_admin/callback",
	}
	logger := &mockLogger{}
	secret := []byte("test-secret-key-32-bytes-long-!")

	_, _, callbackHandler := New(oauthConfig, secret, "/_admin/login", "/_admin", WithLogger(logger))

	stateVal := "mystate.invalidsignaturehere"
	req := httptest.NewRequest("GET", "/_admin/callback?code=auth-code&state="+stateVal, nil)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: stateVal})

	rr := httptest.NewRecorder()
	callbackHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
}

func TestOAuth2_SessionCookieAuthSuccess(t *testing.T) {
	oauthConfig := &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "https://example.com/_admin/callback",
	}
	logger := &mockLogger{}
	validateJWTMock := func(ctx context.Context, token, audience string) (*Payload, error) {
		if token != "session-token" {
			return nil, errors.New("invalid session token")
		}
		return &Payload{
			Claims: map[string]any{
				"email": "employee@company.com",
			},
		}, nil
	}

	middleware, _, _ := New(
		oauthConfig,
		[]byte("test-secret-key-32-bytes-long-!"),
		"/_admin/login",
		"/_admin",
		WithLogger(logger),
		WithAllowedDomains([]string{"company.com"}),
		WithJWTValidator(validateJWTMock),
	)

	called := false
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("GET", "/_admin/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "session-token"})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if !called {
		t.Error("expected handler to be called")
	}
}

func TestOAuth2_SessionCookieAuthDomainDenied(t *testing.T) {
	oauthConfig := &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "https://example.com/_admin/callback",
	}
	logger := &mockLogger{}
	validateJWTMock := func(ctx context.Context, token, audience string) (*Payload, error) {
		return &Payload{
			Claims: map[string]any{
				"email": "outsider@other.com",
			},
		}, nil
	}

	middleware, _, _ := New(
		oauthConfig,
		[]byte("test-secret-key-32-bytes-long-!"),
		"/_admin/login",
		"/_admin",
		WithLogger(logger),
		WithAllowedDomains([]string{"company.com"}),
		WithJWTValidator(validateJWTMock),
	)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest("GET", "/_admin/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "session-token"})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rr.Code)
	}
}

func TestOAuth2_SessionCookieExpiredRedirect(t *testing.T) {
	oauthConfig := &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "https://example.com/_admin/callback",
	}
	logger := &mockLogger{}
	validateJWTMock := func(ctx context.Context, token, audience string) (*Payload, error) {
		return nil, errors.New("token is expired")
	}

	middleware, _, _ := New(
		oauthConfig,
		[]byte("test-secret-key-32-bytes-long-!"),
		"/_admin/login",
		"/_admin",
		WithLogger(logger),
		WithJWTValidator(validateJWTMock),
	)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest("GET", "/_admin/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "expired-token"})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTemporaryRedirect {
		t.Errorf("expected status 307 redirect, got %d", rr.Code)
	}

	loc := rr.Header().Get("Location")
	if loc != "/_admin/login?redirect=%2F_admin%2Fdashboard" {
		t.Errorf("expected redirect to /_admin/login?redirect=%%2F_admin%%2Fdashboard, got %s", loc)
	}

	// Verify session cookie is deleted
	cookies := rr.Result().Cookies()
	var deletedSessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session" {
			deletedSessionCookie = c
			break
		}
	}
	if deletedSessionCookie == nil || deletedSessionCookie.MaxAge != -1 {
		t.Error("expected session cookie to be deleted")
	}
}

func TestOAuth2_BearerTokenAuthSuccess(t *testing.T) {
	oauthConfig := &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "https://example.com/_admin/callback",
	}
	logger := &mockLogger{}
	validateJWTMock := func(ctx context.Context, token, audience string) (*Payload, error) {
		if token != "service-account-id-token" {
			return nil, errors.New("invalid id token")
		}
		return &Payload{
			Claims: map[string]any{
				"email": "sa@project.iam.gserviceaccount.com",
			},
		}, nil
	}

	middleware, _, _ := New(
		oauthConfig,
		[]byte("test-secret-key-32-bytes-long-!"),
		"/_admin/login",
		"/_admin",
		WithLogger(logger),
		WithAllowedEmails([]string{"sa@project.iam.gserviceaccount.com"}),
		WithJWTValidator(validateJWTMock),
	)

	called := false
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("POST", "/_admin/api", nil)
	req.Header.Set("Authorization", "Bearer service-account-id-token")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if !called {
		t.Error("expected handler to be called")
	}
}

func TestOAuth2_CustomAuthCheck(t *testing.T) {
	oauthConfig := &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "https://example.com/_admin/callback",
	}
	logger := &mockLogger{}
	validateJWTMock := func(ctx context.Context, token, audience string) (*Payload, error) {
		return &Payload{
			Claims: map[string]any{
				"role": "super-admin",
			},
		}, nil
	}

	middleware, _, _ := New(
		oauthConfig,
		[]byte("test-secret-key-32-bytes-long-!"),
		"/_admin/login",
		"/_admin",
		WithLogger(logger),
		WithAuthCheck(func(payload *Payload) bool {
			role, _ := payload.Claims["role"].(string)
			return role == "super-admin"
		}),
		WithJWTValidator(validateJWTMock),
	)

	called := false
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest("GET", "/_admin/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "token"})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if !called {
		t.Error("expected handler to be called")
	}
}

func TestOAuth2_CustomAuthCheckDenied(t *testing.T) {
	oauthConfig := &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "https://example.com/_admin/callback",
	}
	logger := &mockLogger{}
	validateJWTMock := func(ctx context.Context, token, audience string) (*Payload, error) {
		return &Payload{
			Claims: map[string]any{
				"role": "normal-user",
			},
		}, nil
	}

	middleware, _, _ := New(
		oauthConfig,
		[]byte("test-secret-key-32-bytes-long-!"),
		"/_admin/login",
		"/_admin",
		WithLogger(logger),
		WithAuthCheck(func(payload *Payload) bool {
			role, _ := payload.Claims["role"].(string)
			return role == "super-admin"
		}),
		WithJWTValidator(validateJWTMock),
	)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest("GET", "/_admin/dashboard", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "token"})

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rr.Code)
	}
}

func TestOAuth2_CustomPaths(t *testing.T) {
	oauthConfig := &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "https://example.com/custom/callback",
	}
	logger := &mockLogger{}
	secret := []byte("test-secret-key-32-bytes-long-!")

	exchangeMock := func(ctx context.Context, code string) (*oauth2.Token, error) {
		tok := &oauth2.Token{}
		tok = tok.WithExtra(map[string]any{"id_token": "token"})
		return tok, nil
	}
	validateJWTMock := func(ctx context.Context, token, audience string) (*Payload, error) {
		return &Payload{
			Claims: map[string]any{},
		}, nil
	}

	_, loginHandler, callbackHandler := New(
		oauthConfig,
		secret,
		"/custom/login",
		"/custom",
		WithLogger(logger),
		WithTokenExchanger(exchangeMock),
		WithJWTValidator(validateJWTMock),
	)

	req := httptest.NewRequest("GET", "/custom/login", nil)
	rr := httptest.NewRecorder()
	loginHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTemporaryRedirect {
		t.Errorf("expected status 307 redirect, got %d", rr.Code)
	}

	stateVal := signState(secret, "mystate")
	req2 := httptest.NewRequest("GET", "/custom/callback?code=code&state="+stateVal, nil)
	req2.AddCookie(&http.Cookie{Name: "oauth_state", Value: stateVal})
	rr2 := httptest.NewRecorder()
	callbackHandler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusSeeOther {
		t.Errorf("expected status 303, got %d", rr2.Code)
	}
	loc := rr2.Header().Get("Location")
	if loc != "/custom" {
		t.Errorf("expected redirect to parent path /custom, got %s", loc)
	}
}

func TestOAuth2_DynamicRedirect(t *testing.T) {
	oauthConfig := &oauth2.Config{
		ClientID: "test-client-id",
	}
	logger := &mockLogger{}
	secret := []byte("test-secret-key-32-bytes-long-!")

	exchangeMock := func(ctx context.Context, code string) (*oauth2.Token, error) {
		tok := &oauth2.Token{}
		tok = tok.WithExtra(map[string]any{"id_token": "token"})
		return tok, nil
	}
	validateJWTMock := func(ctx context.Context, token, audience string) (*Payload, error) {
		return &Payload{
			Claims: map[string]any{},
		}, nil
	}

	middleware, loginHandler, callbackHandler := New(
		oauthConfig,
		secret,
		"/_admin/login",
		"/_admin",
		WithLogger(logger),
		WithTokenExchanger(exchangeMock),
		WithJWTValidator(validateJWTMock),
	)

	// 1. Visit a protected URL, middleware should redirect to login path carrying destination URL in the query string
	req := httptest.NewRequest("GET", "/_admin/dashboard/repos", nil)
	rr := httptest.NewRecorder()
	middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(rr, req)

	if rr.Code != http.StatusTemporaryRedirect {
		t.Fatalf("expected status 307 redirect, got %d", rr.Code)
	}
	redirectLoc := rr.Header().Get("Location")
	expectedLocation := "/_admin/login?redirect=%2F_admin%2Fdashboard%2Frepos"
	if redirectLoc != expectedLocation {
		t.Errorf("expected login redirect Location to be %q, got %q", expectedLocation, redirectLoc)
	}

	// 2. Call the login handler with the redirect param. It should encode this destination in the state parameter
	loginReq := httptest.NewRequest("GET", redirectLoc, nil)
	loginRR := httptest.NewRecorder()
	loginHandler.ServeHTTP(loginRR, loginReq)

	stateCookie := getCookie(loginRR.Result().Cookies(), "oauth_state")
	if stateCookie == nil {
		t.Fatal("expected oauth_state cookie to be set")
	}

	// 3. Call the callback handler with the signed state parameter
	callbackReq := httptest.NewRequest("GET", "/_admin/callback?code=auth-code&state="+stateCookie.Value, nil)
	callbackReq.AddCookie(stateCookie)
	callbackRR := httptest.NewRecorder()
	callbackHandler.ServeHTTP(callbackRR, callbackReq)

	if callbackRR.Code != http.StatusSeeOther {
		t.Errorf("expected status 303, got %d", callbackRR.Code)
	}
	finalLoc := callbackRR.Header().Get("Location")
	if finalLoc != "/_admin/dashboard/repos" {
		t.Errorf("expected final redirect to /_admin/dashboard/repos, got %q", finalLoc)
	}

	// 4. Test with invalid/malicious (open redirect) target - should fallback to defaultLandingPath
	maliciousDest := "//malicious.com"
	redirectB64 := base64.RawURLEncoding.EncodeToString([]byte(maliciousDest))
	h := hmac.New(sha256.New, secret)
	h.Write([]byte("badstate:" + redirectB64))
	sig := hex.EncodeToString(h.Sum(nil))
	badStateVal := "badstate." + redirectB64 + "." + sig

	maliciousReq := httptest.NewRequest("GET", "/_admin/callback?code=auth-code&state="+badStateVal, nil)
	maliciousReq.AddCookie(&http.Cookie{Name: "oauth_state", Value: badStateVal})
	maliciousRR := httptest.NewRecorder()
	callbackHandler.ServeHTTP(maliciousRR, maliciousReq)

	if maliciousRR.Code != http.StatusSeeOther {
		t.Errorf("expected status 303, got %d", maliciousRR.Code)
	}
	fallbackLoc := maliciousRR.Header().Get("Location")
	if fallbackLoc != "/_admin" {
		t.Errorf("expected open redirect to fall back to defaultLandingPath /_admin, got %q", fallbackLoc)
	}
}

func getCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func locContains(loc, sub string) bool {
	return strings.Contains(loc, sub)
}
