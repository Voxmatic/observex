package main

// F6.1: a synthetic probe credential must never be accepted by the ingestor,
// on any path, even when a misconfigured deployment signs it with a key equal
// to the agent token secret. This file adds tests only; the ingestor's
// authentication code is unchanged. Credential values are never printed.

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/observex/platform/internal/probetoken"
)

var f61AgentSecret = strings.Repeat("s", 40) + "-ingestor-agent-secret"

func f61ProbeCredential(t *testing.T, keyValue string) string {
	t.Helper()
	out, err := probetoken.NewKey(keyValue).Issue(probetoken.IssueRequest{OrgID: "org-a"}, time.Now(), bytes.NewReader(bytes.Repeat([]byte{2}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	return out.Credential.Reveal()
}

func TestF61ProbeCredentialRejectedByParseAgentJWT(t *testing.T) {
	for _, keyValue := range []string{f61AgentSecret, strings.Repeat("k", 40)} {
		cred := f61ProbeCredential(t, keyValue)
		for _, presented := range []string{cred, strings.TrimPrefix(cred, probetoken.TokenPrefix), "oxat_" + strings.TrimPrefix(cred, probetoken.TokenPrefix)} {
			if _, err := parseAgentJWT(presented, f61AgentSecret); err == nil {
				t.Fatal("ingestor accepted a synthetic probe credential")
			}
		}
	}
}

func TestF61ProbeCredentialRejectedByAgentAuthMiddleware(t *testing.T) {
	ing := &Ingestor{cfg: Config{AgentTokenSecret: f61AgentSecret, RequireAgentAuth: true}, logger: zap.NewNop()}
	app := fiber.New()
	app.Use(ing.agentAuthMiddleware())
	ok := func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) }
	app.Post("/v1/metrics", ok)
	app.Post("/v1/f61-unscoped-test", ok) // requiredAgentScope returns "" here

	status := func(path, token string) int {
		req := httptest.NewRequest("POST", path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	// Control: a genuine agent install token passes the unscoped path, so a
	// 401 below is the credential being refused, not the harness.
	agent, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"typ": "observex_agent_install", "org_id": "org-a", "scopes": []string{"metrics:write"},
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(f61AgentSecret))
	if err != nil {
		t.Fatal(err)
	}
	if got := status("/v1/f61-unscoped-test", "oxat_"+agent); got != fiber.StatusNoContent {
		t.Fatalf("control agent token: status %d", got)
	}

	cred := f61ProbeCredential(t, f61AgentSecret)
	for _, path := range []string{"/v1/metrics", "/v1/f61-unscoped-test"} {
		for _, presented := range []string{cred, strings.TrimPrefix(cred, probetoken.TokenPrefix)} {
			if got := status(path, presented); got != fiber.StatusUnauthorized {
				t.Fatalf("%s: probe credential got status %d, want 401", path, got)
			}
		}
	}
}
