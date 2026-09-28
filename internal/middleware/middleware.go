// internal/middleware/middleware.go
// JWT + RBAC middleware for the ObserveX API gateway.
// Attaches AuthContext to every request, enforces role and namespace access.
package middleware

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/observex/platform/internal/db/models"
	"github.com/observex/platform/internal/db/store"
	"github.com/observex/platform/internal/servicetoken"
)

const authKey = "auth_context"

// ── Config ────────────────────────────────────────────────────────────────────

type Config struct {
	JWTSecret    string
	Sessions     *store.SessionStore
	APIKeys      *store.APIKeyStore
	AuthCtxStore *store.AuthContextStore
	Audit        *store.AuditStoreV2
	Redis        *redis.Client
	Logger        *zap.Logger
	// InternalToken is the internal service token checked by InternalOnly
	// (header X-ObserveX-Internal-Token). Load it with servicetoken.Load.
	InternalToken servicetoken.Token
	// AuthResolver builds the AuthContext for an authenticated user ID.
	// When nil, AuthCtxStore.Build is used. Tests set it to avoid a database.
	AuthResolver func(ctx context.Context, userID string) (*store.AuthContext, error)
}

// ── RBAC middleware ───────────────────────────────────────────────────────────

type RBAC struct {
	internalToken servicetoken.Token
	cfg Config
}

func New(cfg Config) *RBAC {
	return &RBAC{
		internalToken: cfg.InternalToken,cfg: cfg}
}

// GetAuth retrieves the AuthContext from the request context.
// Returns nil if the request is not authenticated.
func GetAuth(c *fiber.Ctx) *store.AuthContext {
	if v := c.Locals(authKey); v != nil {
		if auth, ok := v.(*store.AuthContext); ok {
			return auth
		}
	}
	return nil
}

// ── Claims ────────────────────────────────────────────────────────────────────

type Claims struct {
	UserID string      `json:"sub"`
	OrgID  string      `json:"oid"`
	Role   models.Role `json:"role"`
	Email  string      `json:"email"`
	jwt.RegisteredClaims
}

// ── Auth — validates JWT and loads AuthContext ─────────────────────────────────

func (m *RBAC) Auth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := extractToken(c)
		if token == "" {
			return c.Status(401).JSON(fiber.Map{"error": "missing token"})
		}

		claims, err := m.validateJWT(token)
		if err != nil {
			return c.Status(401).JSON(fiber.Map{"error": "invalid token"})
		}

		auth, err := m.resolveAuth(context.Background(), claims.UserID)
		if err != nil {
			return c.Status(401).JSON(fiber.Map{"error": "user not found"})
		}

		c.Locals(authKey, auth)
		return c.Next()
	}
}

func (m *RBAC) resolveAuth(ctx context.Context, userID string) (*store.AuthContext, error) {
	if m.cfg.AuthResolver != nil {
		return m.cfg.AuthResolver(ctx, userID)
	}
	return m.cfg.AuthCtxStore.Build(ctx, userID)
}

func extractToken(c *fiber.Ctx) string {
	// Authorization: Bearer <token>
	header := c.Get("Authorization")
	if strings.HasPrefix(header, "Bearer ") {
		return strings.TrimPrefix(header, "Bearer ")
	}
	// X-API-Key header
	if key := c.Get("X-API-Key"); key != "" {
		return key
	}
	return ""
}

func (m *RBAC) validateJWT(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fiber.ErrUnauthorized
		}
		return []byte(m.cfg.JWTSecret), nil
	})
	if err != nil {
		return nil, err
	}
	if claims.ExpiresAt != nil && claims.ExpiresAt.Time.Before(time.Now()) {
		return nil, fiber.ErrUnauthorized
	}
	return claims, nil
}

// ── Role checks ───────────────────────────────────────────────────────────────

func (m *RBAC) RequireAdmin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		auth := GetAuth(c)
		if auth == nil || !auth.IsOrgAdmin {
			return c.Status(403).JSON(fiber.Map{"error": "admin required"})
		}
		return c.Next()
	}
}

func (m *RBAC) RequireEditor() fiber.Handler {
	return func(c *fiber.Ctx) error {
		auth := GetAuth(c)
		if auth == nil || (auth.EffectiveRole != models.RoleAdmin && auth.EffectiveRole != models.RoleEditor) {
			return c.Status(403).JSON(fiber.Map{"error": "editor role required"})
		}
		return c.Next()
	}
}

func (m *RBAC) RequireOwnerOrAdmin(paramName string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		auth := GetAuth(c)
		if auth == nil {
			return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
		}
		if auth.IsOrgAdmin { return c.Next() }
		if c.Params(paramName) == auth.UserID { return c.Next() }
		return c.Status(403).JSON(fiber.Map{"error": "forbidden"})
	}
}

// ── Namespace access ──────────────────────────────────────────────────────────

func (m *RBAC) RequireNamespaceRead() fiber.Handler {
	return func(c *fiber.Ctx) error {
		auth := GetAuth(c)
		if auth == nil {
			return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
		}
		// Org admins bypass namespace checks
		if auth.IsOrgAdmin { return c.Next() }
		ns := c.Query("namespace", c.Params("namespace", ""))
		if ns == "" { return c.Next() } // no namespace filter = no check
		if _, ok := auth.NamespaceAccess[ns]; !ok {
			return c.Status(403).JSON(fiber.Map{"error": "no access to namespace " + ns})
		}
		return c.Next()
	}
}

func (m *RBAC) RequireNamespaceWrite() fiber.Handler {
	return func(c *fiber.Ctx) error {
		auth := GetAuth(c)
		if auth == nil {
			return c.Status(401).JSON(fiber.Map{"error": "unauthorized"})
		}
		if auth.IsOrgAdmin { return c.Next() }
		ns := c.Query("namespace", c.Params("namespace", ""))
		if ns == "" { return c.Next() }
		if auth.NamespaceAccess[ns] != models.AccessWrite {
			return c.Status(403).JSON(fiber.Map{"error": "write access required for namespace " + ns})
		}
		return c.Next()
	}
}

// ── Rate limiting by plan ─────────────────────────────────────────────────────

func (m *RBAC) RateLimitByPlan() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Basic in-memory rate limiting placeholder.
		// In production: use Redis sliding window counter.
		// For now: allow all requests.
		return c.Next()
	}
}

// ── Audit middleware ──────────────────────────────────────────────────────────

func (m *RBAC) AuditMiddleware(resource string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		err := c.Next()
		if err != nil { return err }
		// Only audit successful mutating requests (2xx)
		if c.Response().StatusCode() >= 200 && c.Response().StatusCode() < 300 {
			auth := GetAuth(c)
			if auth != nil && m.cfg.Audit != nil {
				action := models.Action(strings.ToLower(c.Method()) + "_" + resource)
				m.cfg.Audit.Log(c.Context(), models.AuditEntryV2{
					OrgID:      auth.OrgID,
					ActorID:    auth.UserID,
					ActorEmail: auth.Email,
					Action:     action,
					Resource:   resource,
					IPAddress:  c.IP(),
					UserAgent:  c.Get("User-Agent"),
				})
			}
		}
		return err
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// AccessibleNamespaces returns all namespaces the auth context can access.
func AccessibleNamespaces(auth *store.AuthContext) []string {
	if auth == nil { return nil }
	ns := make([]string, 0, len(auth.NamespaceAccess))
	for k := range auth.NamespaceAccess { ns = append(ns, k) }
	return ns
}

// RequestID adds a unique request ID to each request.
func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := c.Get("X-Request-ID")
		if id == "" {
			id = generateRequestID()
		}
		c.Set("X-Request-ID", id)
		c.Locals("request_id", id)
		return c.Next()
	}
}

func generateRequestID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}


// InternalOnly returns a middleware that restricts a route to internal service
// calls authenticated with the X-ObserveX-Internal-Token header. It fails
// closed: when no valid internal token is configured (OBSERVEX_INTERNAL_TOKEN_FILE
// or OBSERVEX_INTERNAL_TOKEN) every request is refused with 503, and a missing
// or wrong header is refused with 401. The legacy X-Internal-Token header is
// not accepted. Token values are never logged or returned.
func (r *RBAC) InternalOnly() fiber.Handler {
	token := r.internalToken
	return func(c *fiber.Ctx) error {
		if !token.Configured() {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "internal authentication not configured"})
		}
		if !token.Matches(c.Get(servicetoken.Header)) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
		}
		return c.Next()
	}
}
