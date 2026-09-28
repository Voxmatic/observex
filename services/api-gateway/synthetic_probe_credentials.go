package main

// F6.1 synthetic probe credential issuance.
//
// Under G-1 each synthetic probe vantage is its own Deployment. An organization
// administrator calls this endpoint once per vantage and places the returned
// credential in that Deployment's own Secret. The credential establishes the
// vantage identity: the vantage ID is minted here, bound to the administrator's
// organization, and signed. See internal/probetoken for the credential and
// internal/intake/f61 for how the processor will check it.
//
// Security properties kept here:
//   - The organization comes only from the authenticated context. The request
//     body is decoded strictly: an org_id, scopes or any other unknown field is
//     refused rather than ignored.
//   - Issuance is refused (503) unless a dedicated key is configured that is
//     neither the session JWT secret nor the agent token secret.
//   - The credential value is returned once, with Cache-Control: no-store, and
//     is never logged. Logs carry org, vantage ID, actor and expiry only.
//   - Rotation re-issues for the same vantage ID; by itself it does not
//     invalidate the previous credential. Revocation (PROPOSED REVOKE-1, see
//     f61_results.go) sets a per-vantage bound that the processor enforces:
//     credentials issued before it are refused. Rotating a decommissioned
//     vantage, or before a fresh bound has taken effect, is refused (409).
//   - The network zones "local" and "*" are reserved by synthetic_checks
//     locations (PROPOSED LOC-1) and cannot be declared.

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	intakef61 "github.com/observex/platform/internal/intake/f61"
	"github.com/observex/platform/internal/middleware"
	"github.com/observex/platform/internal/probetoken"
)

// syntheticProbeCredentialRequest is the complete set of accepted fields.
type syntheticProbeCredentialRequest struct {
	// VantageID is empty to create a vantage, or an existing vantage ID of
	// this organization to rotate its credential.
	VantageID   string `json:"vantage_id"`
	NetworkZone string `json:"network_zone"`
	ClusterName string `json:"cluster_name"`
	Environment string `json:"environment"`
	HostGroup   string `json:"host_group"`
	// TTLHours is the credential lifetime in hours; 0 means the default.
	TTLHours int `json:"ttl_hours"`
}

var errSyntheticProbeRequest = errors.New("invalid synthetic probe credential request")

func (gw *Gateway) registerSyntheticProbeCredentialRoutes(api fiber.Router, mw *middleware.RBAC) {
	key := gw.cfg.SyntheticProbeKey
	if gw.log != nil {
		gw.log.Info("synthetic probe credential issuance",
			zap.Bool("configured", gw.syntheticProbeKeyUsable()),
			zap.String("source", string(key.Source())),
			zap.String("reason", string(key.Reason())),
			zap.Bool("reuses_session_or_agent_secret", key.SameAs(gw.cfg.JWTSecret) || key.SameAs(gw.cfg.AgentTokenSecret)),
		)
	}
	api.Post("/synthetic/probe-credentials", mw.RequireAdmin(), mw.AuditMiddleware("synthetic_probe_credential"), gw.handleIssueSyntheticProbeCredential)
}

// syntheticProbeKeyUsable is true only for a configured key that is not also
// the session or agent secret.
func (gw *Gateway) syntheticProbeKeyUsable() bool {
	key := gw.cfg.SyntheticProbeKey
	return key.Configured() && !key.SameAs(gw.cfg.JWTSecret) && !key.SameAs(gw.cfg.AgentTokenSecret)
}

func decodeSyntheticProbeCredentialRequest(body []byte) (syntheticProbeCredentialRequest, error) {
	var req syntheticProbeCredentialRequest
	if len(bytes.TrimSpace(body)) == 0 {
		return req, nil
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return syntheticProbeCredentialRequest{}, errSyntheticProbeRequest
	}
	if _, err := dec.Token(); err != io.EOF {
		return syntheticProbeCredentialRequest{}, errSyntheticProbeRequest
	}
	return req, nil
}

func (gw *Gateway) handleIssueSyntheticProbeCredential(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	auth := middleware.GetAuth(c)
	if auth == nil || !auth.IsOrgAdmin || strings.TrimSpace(auth.OrgID) == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "organization admin required"})
	}
	if !gw.syntheticProbeKeyUsable() {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "synthetic probe credential issuance is not configured"})
	}

	req, err := decodeSyntheticProbeCredentialRequest(c.Body())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "request body must be a JSON object with only vantage_id, network_zone, cluster_name, environment, host_group and ttl_hours"})
	}
	maxHours := int(probetoken.MaxTTL / time.Hour)
	if req.TTLHours < 0 || req.TTLHours > maxHours {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "ttl_hours must be between 1 and 720, or omitted"})
	}
	if intakef61.ReservedZone(req.NetworkZone) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": `network_zone "local" and "*" are reserved`})
	}
	now := time.Now()
	if req.VantageID != "" && gw.cfg.SyntheticProbeKey.VantageIDBoundTo(req.VantageID, auth.OrgID) {
		if status, msg := gw.f61IssuanceRefusal(c.UserContext(), auth.OrgID, req.VantageID, now); status != 0 {
			return c.Status(status).JSON(fiber.Map{"error": msg})
		}
	}

	issued, err := gw.cfg.SyntheticProbeKey.Issue(probetoken.IssueRequest{
		OrgID:     auth.OrgID,
		VantageID: req.VantageID,
		Declared: probetoken.Declaration{
			NetworkZone: req.NetworkZone,
			ClusterName: req.ClusterName,
			Environment: req.Environment,
			HostGroup:   req.HostGroup,
		},
		TTL: time.Duration(req.TTLHours) * time.Hour,
	}, now, rand.Reader)
	switch {
	case err == nil:
	case errors.Is(err, probetoken.ErrVantageID):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "vantage_id was not issued for this organization"})
	case errors.Is(err, probetoken.ErrDeclaration):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "vantage declaration fields must be printable text of at most 128 bytes"})
	case errors.Is(err, probetoken.ErrLifetime):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "ttl_hours must be between 1 and 720, or omitted"})
	case errors.Is(err, probetoken.ErrOrg):
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "organization admin required"})
	default:
		gw.log.Error("synthetic probe credential issuance failed", zap.String("org_id", auth.OrgID), zap.Error(err))
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to issue synthetic probe credential"})
	}

	gw.log.Info("synthetic probe credential issued",
		zap.String("org_id", issued.OrgID),
		zap.String("vantage_id", issued.VantageID),
		zap.String("actor_id", auth.UserID),
		zap.Bool("rotation", issued.Rotated),
		zap.Time("expires_at", issued.ExpiresAt),
	)
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{
		"token":        issued.Credential.Reveal(),
		"token_type":   "Bearer",
		"org_id":       issued.OrgID,
		"vantage_id":   issued.VantageID,
		"vantage_kind": intakef61.VantageKind,
		"scopes":       []string{probetoken.Scope},
		"declared": fiber.Map{
			"network_zone": issued.Declared.NetworkZone,
			"cluster_name": issued.Declared.ClusterName,
			"environment":  issued.Declared.Environment,
			"host_group":   issued.Declared.HostGroup,
		},
		"rotated":        issued.Rotated,
		"revocable":      true,
		"issued_at":      issued.IssuedAt,
		"expires_at":     issued.ExpiresAt,
		"expires_in_sec": int(issued.ExpiresAt.Sub(issued.IssuedAt).Seconds()),
	})
}
