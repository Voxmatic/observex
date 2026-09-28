package main

// F6.1 TLS certificate results and probe vantage revocation (gateway side).
//
//	GET  /api/v1/synthetic/tls-certificates[?namespace=]        results of the caller's organization
//	GET  /api/v1/synthetic/checks/:id/tls-certificate           one result with per-vantage detail
//	GET  /api/v1/synthetic/tls-certificates/events[?after=&limit=]  result events (outbox), oldest first
//	POST /api/v1/synthetic/probe-credentials/revoke             org admin: refuse a vantage's credentials
//
// The results are written by the processor (internal/result/f61, PROPOSED
// DEST-1); the gateway only reads them. The organization always comes from the
// authenticated context. A caller who is not an org admin sees only results in
// namespaces they hold read or write access to; a result in any other
// namespace, of another organization or that does not exist is answered with
// the same 404. This is stricter than the older synthetic check routes, which
// scope by organization only.
//
// Revocation (PROPOSED REVOKE-1) records a per-vantage bound: the processor
// refuses every credential of that vantage issued before it. Decommissioning
// sets the bound to infinity, so nothing issued for the vantage is accepted
// again. The bound only moves forward.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/observex/platform/internal/db/models"
	"github.com/observex/platform/internal/db/store"
	"github.com/observex/platform/internal/middleware"
	resultf61 "github.com/observex/platform/internal/result/f61"
)

const f61EventsMaxLimit = 500

func (gw *Gateway) registerF61ResultRoutes(api fiber.Router, mw *middleware.RBAC) {
	api.Get("/synthetic/tls-certificates", mw.RequireNamespaceRead(), gw.handleF61ListResults)
	api.Get("/synthetic/tls-certificates/events", mw.RequireNamespaceRead(), gw.handleF61Events)
	api.Get("/synthetic/checks/:id/tls-certificate", gw.handleF61GetResult)
	api.Post("/synthetic/probe-credentials/revoke", mw.RequireAdmin(), mw.AuditMiddleware("synthetic_probe_credential_revocation"),
		gw.handleF61Revoke)
}

// f61Results is nil when the gateway has no database.
func (gw *Gateway) f61Results() *resultf61.Store {
	if gw.db == nil || gw.db.Pool == nil {
		return nil
	}
	return resultf61.New(gw.db.Pool)
}

func f61Readable(auth *store.AuthContext, namespace string) bool {
	if auth.IsOrgAdmin {
		return true
	}
	a := auth.NamespaceAccess[namespace]
	return a == models.AccessRead || a == models.AccessWrite
}

// f61Caller returns the authenticated caller with an organization, or writes
// the refusal.
func f61Caller(c *fiber.Ctx) (*store.AuthContext, bool) {
	auth := middleware.GetAuth(c)
	if auth == nil {
		_ = c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		return nil, false
	}
	if strings.TrimSpace(auth.OrgID) == "" {
		_ = c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "organization required"})
		return nil, false
	}
	return auth, true
}

func f61Unavailable(c *fiber.Ctx) error {
	return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "TLS certificate results are unavailable"})
}

func (gw *Gateway) handleF61ListResults(c *fiber.Ctx) error {
	auth, ok := f61Caller(c)
	if !ok {
		return nil
	}
	s := gw.f61Results()
	if s == nil {
		return f61Unavailable(c)
	}
	ns := c.Query("namespace", "")
	list, err := s.List(c.UserContext(), auth.OrgID, ns, time.Now())
	if err != nil {
		gw.log.Error("f61 result list failed", zap.String("org_id", auth.OrgID), zap.Error(err))
		return f61Unavailable(c)
	}
	out := make([]resultf61.Result, 0, len(list))
	for _, r := range list {
		if f61Readable(auth, r.Namespace) {
			out = append(out, r)
		}
	}
	return c.JSON(fiber.Map{"results": out, "total": len(out)})
}

func (gw *Gateway) handleF61GetResult(c *fiber.Ctx) error {
	auth, ok := f61Caller(c)
	if !ok {
		return nil
	}
	s := gw.f61Results()
	if s == nil {
		return f61Unavailable(c)
	}
	r, err := s.Get(c.UserContext(), auth.OrgID, c.Params("id"), time.Now())
	if errors.Is(err, resultf61.ErrNotFound) || (err == nil && !f61Readable(auth, r.Namespace)) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "no TLS certificate result for this check"})
	}
	if err != nil {
		gw.log.Error("f61 result read failed", zap.String("org_id", auth.OrgID), zap.Error(err))
		return f61Unavailable(c)
	}
	return c.JSON(r)
}

func (gw *Gateway) handleF61Events(c *fiber.Ctx) error {
	auth, ok := f61Caller(c)
	if !ok {
		return nil
	}
	s := gw.f61Results()
	if s == nil {
		return f61Unavailable(c)
	}
	after, err := strconv.ParseInt(c.Query("after", "0"), 10, 64)
	if err != nil || after < 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "after must be a non-negative event id"})
	}
	limit, err := strconv.Atoi(c.Query("limit", "100"))
	if err != nil || limit < 1 || limit > f61EventsMaxLimit {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "limit must be between 1 and 500"})
	}
	ns := c.Query("namespace", "")
	page, err := s.Events(c.UserContext(), auth.OrgID, after, limit)
	if err != nil {
		gw.log.Error("f61 result events read failed", zap.String("org_id", auth.OrgID), zap.Error(err))
		return f61Unavailable(c)
	}
	// The cursor advances over the whole page, including events the caller
	// may not see, so paging never stalls.
	next := after
	out := make([]resultf61.Event, 0, len(page))
	for _, e := range page {
		next = e.ID
		if f61Readable(auth, e.Namespace) && (ns == "" || ns == e.Namespace) {
			out = append(out, e)
		}
	}
	return c.JSON(fiber.Map{"events": out, "next_after": next, "more": len(page) == limit})
}

// f61RevokeRequest is the complete set of accepted fields.
type f61RevokeRequest struct {
	VantageID string `json:"vantage_id"`
	// NotBefore defaults to now. Credentials of the vantage issued before it
	// are refused. It may not be in the future.
	NotBefore *time.Time `json:"not_before"`
	// Decommission refuses every credential of the vantage, forever.
	Decommission bool   `json:"decommission"`
	Reason       string `json:"reason"`
}

var errF61RevokeRequest = errors.New("invalid revocation request")

func decodeF61RevokeRequest(body []byte) (f61RevokeRequest, error) {
	var req f61RevokeRequest
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return f61RevokeRequest{}, errF61RevokeRequest
	}
	if _, err := dec.Token(); err != io.EOF {
		return f61RevokeRequest{}, errF61RevokeRequest
	}
	if req.VantageID == "" || len(req.Reason) > 256 {
		return f61RevokeRequest{}, errF61RevokeRequest
	}
	for _, r := range req.Reason {
		if !unicode.IsPrint(r) {
			return f61RevokeRequest{}, errF61RevokeRequest
		}
	}
	return req, nil
}

// f61RevocationBound is the effective bound for a revocation requested at
// now: the next whole second, so every credential issued up to now (their
// issue times have whole-second precision) is refused.
func f61RevocationBound(now time.Time) time.Time {
	return now.UTC().Truncate(time.Second).Add(time.Second)
}

func (gw *Gateway) handleF61Revoke(c *fiber.Ctx) error {
	c.Set("Cache-Control", "no-store")
	auth := middleware.GetAuth(c)
	if auth == nil || !auth.IsOrgAdmin || strings.TrimSpace(auth.OrgID) == "" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "organization admin required"})
	}
	if !gw.syntheticProbeKeyUsable() {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "synthetic probe credentials are not configured"})
	}
	s := gw.f61Results()
	if s == nil {
		return f61Unavailable(c)
	}
	req, err := decodeF61RevokeRequest(c.Body())
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "request body must be a JSON object with vantage_id and optionally not_before, decommission and a printable reason of at most 256 bytes"})
	}
	// A vantage ID is MAC-bound to the organization it was issued for; one of
	// another organization, or a made-up one, is not found.
	if !gw.cfg.SyntheticProbeKey.VantageIDBoundTo(req.VantageID, auth.OrgID) {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "vantage not found"})
	}
	now := time.Now()
	bound := f61RevocationBound(now)
	if req.NotBefore != nil {
		if req.NotBefore.After(bound) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "not_before may not be in the future"})
		}
		bound = req.NotBefore.UTC()
	}
	effective, err := s.Revoke(c.UserContext(), auth.OrgID, req.VantageID, bound, req.Decommission, req.Reason, auth.UserID)
	if err != nil {
		gw.log.Error("f61 vantage revocation failed", zap.String("org_id", auth.OrgID), zap.Error(err))
		return f61Unavailable(c)
	}
	decommissioned := effective.Equal(resultf61.Decommissioned)
	gw.log.Info("synthetic probe vantage revoked",
		zap.String("org_id", auth.OrgID), zap.String("vantage_id", req.VantageID), zap.String("actor_id", auth.UserID),
		zap.Bool("decommissioned", decommissioned), zap.Time("not_before", effective))
	resp := fiber.Map{"vantage_id": req.VantageID, "decommissioned": decommissioned}
	if !decommissioned {
		resp["not_before"] = effective
	}
	return c.JSON(resp)
}

// f61IssuanceRefusal checks, for a rotation, that the vantage is not
// decommissioned and that a fresh revocation bound has passed, so a
// replacement credential is not refused on arrival. It returns a status and
// message, or 0 when issuance may proceed. Without a database the processor
// still enforces revocation; this is an operator guard.
func (gw *Gateway) f61IssuanceRefusal(ctx context.Context, orgID, vantageID string, now time.Time) (int, string) {
	s := gw.f61Results()
	if s == nil || vantageID == "" {
		return 0, ""
	}
	bound, revoked, err := s.RevokedBefore(ctx, orgID, vantageID)
	if err != nil {
		gw.log.Error("f61 revocation lookup failed", zap.String("org_id", orgID), zap.Error(err))
		return fiber.StatusServiceUnavailable, "synthetic probe credential issuance is unavailable"
	}
	switch {
	case !revoked:
		return 0, ""
	case bound.Equal(resultf61.Decommissioned):
		return fiber.StatusConflict, "vantage is decommissioned; issue a credential for a new vantage"
	case now.Truncate(time.Second).Before(bound):
		return fiber.StatusConflict, "a revocation of this vantage takes effect at " + bound.Format(time.RFC3339) + "; retry after it"
	}
	return 0, ""
}

// validCheckLocations bounds a synthetic check's locations list.
func validCheckLocations(locations []string) bool {
	if len(locations) == 0 || len(locations) > 32 {
		return false
	}
	for _, l := range locations {
		if l == "" || len(l) > 128 || strings.TrimSpace(l) != l {
			return false
		}
		for _, r := range l {
			if !unicode.IsPrint(r) {
				return false
			}
		}
	}
	return true
}
