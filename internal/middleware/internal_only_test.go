package middleware

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/observex/platform/internal/servicetoken"
)

// Test token values are built at run time and never printed by these tests.
var (
	internalTestToken = strings.Repeat("m", servicetoken.MinLength) + "-middleware-test"
	otherTestToken    = strings.Repeat("n", servicetoken.MinLength) + "-middleware-other"
)

func loadTestToken(value string) servicetoken.Token {
	env := map[string]string{servicetoken.EnvVar: value}
	return servicetoken.LoadFrom(
		func(k string) (string, bool) { v, ok := env[k]; return v, ok },
		func(string) ([]byte, error) { return nil, io.EOF },
	)
}

func newInternalOnlyApp(token servicetoken.Token) *fiber.App {
	app := fiber.New()
	mw := New(Config{InternalToken: token})
	app.Get("/internal/probe", mw.InternalOnly(), func(c *fiber.Ctx) error {
		return c.SendString("reached")
	})
	return app
}

func TestInternalOnly(t *testing.T) {
	configured := loadTestToken(internalTestToken)
	if !configured.Configured() {
		t.Fatal("test token not configured")
	}
	tests := []struct {
		name       string
		token      servicetoken.Token
		headers    map[string]string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "TokenUnset_503",
			token:      servicetoken.Token{},
			wantStatus: fiber.StatusServiceUnavailable,
			wantBody:   `{"error":"internal authentication not configured"}`,
		},
		{
			name:       "TokenUnset_WithHeader_Still503",
			token:      servicetoken.Token{},
			headers:    map[string]string{servicetoken.Header: internalTestToken},
			wantStatus: fiber.StatusServiceUnavailable,
			wantBody:   `{"error":"internal authentication not configured"}`,
		},
		{
			name:       "TokenTooShort_503",
			token:      loadTestToken("short"),
			headers:    map[string]string{servicetoken.Header: "short"},
			wantStatus: fiber.StatusServiceUnavailable,
			wantBody:   `{"error":"internal authentication not configured"}`,
		},
		{
			name:       "HeaderMissing_401",
			token:      configured,
			wantStatus: fiber.StatusUnauthorized,
			wantBody:   `{"error":"unauthorized"}`,
		},
		{
			name:       "HeaderEmpty_401",
			token:      configured,
			headers:    map[string]string{servicetoken.Header: ""},
			wantStatus: fiber.StatusUnauthorized,
			wantBody:   `{"error":"unauthorized"}`,
		},
		{
			name:       "WrongToken_401",
			token:      configured,
			headers:    map[string]string{servicetoken.Header: otherTestToken},
			wantStatus: fiber.StatusUnauthorized,
			wantBody:   `{"error":"unauthorized"}`,
		},
		{
			name:       "LegacyHeaderNotAccepted_401",
			token:      configured,
			headers:    map[string]string{"X-Internal-Token": internalTestToken},
			wantStatus: fiber.StatusUnauthorized,
			wantBody:   `{"error":"unauthorized"}`,
		},
		{
			name:       "BearerHeaderNotAccepted_401",
			token:      configured,
			headers:    map[string]string{"Authorization": "Bearer " + internalTestToken},
			wantStatus: fiber.StatusUnauthorized,
			wantBody:   `{"error":"unauthorized"}`,
		},
		{
			name:       "CorrectToken_Passes",
			token:      configured,
			headers:    map[string]string{servicetoken.Header: internalTestToken},
			wantStatus: fiber.StatusOK,
			wantBody:   "reached",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newInternalOnlyApp(tt.token)
			req := httptest.NewRequest("GET", "/internal/probe", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tt.wantStatus)
			}
			if string(body) != tt.wantBody {
				t.Fatalf("unexpected response body (length %d)", len(body))
			}
			for _, secret := range []string{internalTestToken, otherTestToken} {
				if strings.Contains(string(body), secret) {
					t.Fatal("response body contains a token value")
				}
				for k, vals := range resp.Header {
					for _, v := range vals {
						if strings.Contains(v, secret) {
							t.Fatalf("response header %q contains a token value", k)
						}
					}
				}
			}
		})
	}
}
