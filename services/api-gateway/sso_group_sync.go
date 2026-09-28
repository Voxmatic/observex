// services/api-gateway/sso_group_sync.go
//
// SSO Group Sync — IdP group membership → ObserveX RBAC roles.
//
// Both SAML and OIDC carry group claims; this file normalises them and
// maps them to ObserveX roles (viewer / editor / admin) and teams.
//
// SAML: groups arrive as a multi-value attribute, e.g.:
//   <saml:Attribute Name="groups">
//     <saml:AttributeValue>observex-admins</saml:AttributeValue>
//     <saml:AttributeValue>platform-engineers</saml:AttributeValue>
//   </saml:Attribute>
//
// OIDC: groups arrive in the id_token claims, e.g.:
//   {"groups": ["observex-admins", "platform-engineers"]}
//
// Mapping is configured per org in sso_configs.group_mappings (JSONB):
//   [
//     {"idp_group":"observex-admins",   "role":"admin"},
//     {"idp_group":"platform-engineers","role":"editor","team":"platform"},
//     {"idp_group":"read-only",         "role":"viewer"}
//   ]
//
// Precedence: highest role wins. If user matches multiple groups,
// admin > editor > viewer. Team assignment is additive.
//
// The sync runs on every SSO login (not just first provision), so
// group changes in the IdP take effect on the user's next login.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	dbmodels "github.com/observex/platform/internal/db/models"
	"github.com/observex/platform/internal/db/store"
	"github.com/observex/platform/internal/middleware"
	"go.uber.org/zap"
)

// ── Types ─────────────────────────────────────────────────────────────────────

// GroupMapping is one entry in the sso_configs.group_mappings JSON column.
type GroupMapping struct {
	IDPGroup string `json:"idp_group"` // exact or glob match against IdP group name
	Role     string `json:"role"`      // viewer | editor | admin
	Team     string `json:"team,omitempty"` // optional team slug to join
}

// SyncResult captures what changed during a group sync.
type SyncResult struct {
	UserID       string
	Email        string
	PreviousRole string
	NewRole      string
	TeamsAdded   []string
	TeamsRemoved []string
	Changed      bool
}

// ── Loader ────────────────────────────────────────────────────────────────────

// loadGroupMappings fetches the group mapping config for an org + SSO type.
func (gw *Gateway) loadGroupMappings(ctx context.Context, orgID, ssoType string) ([]GroupMapping, error) {
	var raw []byte
	err := gw.db.Pool.QueryRow(ctx,
		`SELECT COALESCE(group_mappings, '[]'::jsonb)
		 FROM sso_configs WHERE org_id=$1 AND type=$2`,
		orgID, ssoType).Scan(&raw)
	if err != nil {
		// Column may not exist yet — return empty set gracefully
		return nil, nil
	}
	var mappings []GroupMapping
	if err := json.Unmarshal(raw, &mappings); err != nil {
		return nil, fmt.Errorf("parse group_mappings: %w", err)
	}
	return mappings, nil
}

// ── Role resolution ───────────────────────────────────────────────────────────

var rolePriority = map[string]int{
	"admin":  3,
	"editor": 2,
	"viewer": 1,
	"":       0,
}

// resolveRoleFromGroups returns the highest-priority role matching any of
// the user's IdP groups, plus the list of teams they should belong to.
func resolveRoleFromGroups(idpGroups []string, mappings []GroupMapping) (role string, teams []string) {
	bestPriority := 0
	teamSet := map[string]bool{}

	for _, idpGroup := range idpGroups {
		for _, m := range mappings {
			if groupMatches(idpGroup, m.IDPGroup) {
				if rolePriority[m.Role] > bestPriority {
					bestPriority = rolePriority[m.Role]
					role = m.Role
				}
				if m.Team != "" {
					teamSet[m.Team] = true
				}
			}
		}
	}

	for t := range teamSet {
		teams = append(teams, t)
	}
	return role, teams
}

// groupMatches supports exact match and simple glob (* suffix).
func groupMatches(idpGroup, pattern string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(idpGroup, strings.TrimSuffix(pattern, "*"))
	}
	return strings.EqualFold(idpGroup, pattern)
}

// ── Team lookup ────────────────────────────────────────────────────────────────

// findOrCreateTeam returns an existing team by slug or creates it.
func (gw *Gateway) findOrCreateTeam(ctx context.Context, orgID, slug string) (string, error) {
	// Try to find by name first
	teams, err := gw.teams.List(ctx, orgID)
	if err != nil {
		return "", err
	}
	for _, t := range teams {
		if strings.EqualFold(t.Name, slug) || strings.EqualFold(t.ID, slug) {
			return t.ID, nil
		}
	}
	// Create new team
	t, err := gw.teams.Create(ctx, orgID, slug)
	if err != nil {
		return "", fmt.Errorf("create team %q: %w", slug, err)
	}
	return t.ID, nil
}

// ── Main sync ─────────────────────────────────────────────────────────────────

// syncGroupMembership runs on every SSO login. It:
//  1. Loads the org's group→role mappings
//  2. Resolves the highest role from idpGroups
//  3. Updates the user's role in Postgres if it changed
//  4. Adds the user to mapped teams (but never removes from non-mapped teams)
func (gw *Gateway) syncGroupMembership(
	ctx context.Context,
	user *store.User,
	orgID string,
	ssoType string,
	idpGroups []string,
	defaultRole string,
) SyncResult {
	result := SyncResult{UserID: user.ID, Email: user.Email, PreviousRole: string(user.Role)}

	if len(idpGroups) == 0 {
		// No group claims — use default_role from sso_configs
		result.NewRole = defaultRole
		if result.NewRole == "" {
			result.NewRole = "viewer"
		}
	} else {
		mappings, err := gw.loadGroupMappings(ctx, orgID, ssoType)
		if err != nil {
			gw.log.Warn("group mappings load failed, using default role",
				zap.String("org_id", orgID), zap.Error(err))
		}

		if len(mappings) == 0 {
			// No mappings configured — fall through to default role
			result.NewRole = defaultRole
			if result.NewRole == "" {
				result.NewRole = "viewer"
			}
		} else {
			role, teams := resolveRoleFromGroups(idpGroups, mappings)
			if role == "" {
				// User has groups but none matched — deny or use default
				result.NewRole = defaultRole
				if result.NewRole == "" {
					result.NewRole = "viewer"
				}
			} else {
				result.NewRole = role
			}

			// Process team assignments
			for _, teamSlug := range teams {
				teamID, err := gw.findOrCreateTeam(ctx, orgID, teamSlug)
				if err != nil {
					gw.log.Warn("team lookup failed", zap.String("slug", teamSlug), zap.Error(err))
					continue
				}
				// Add user to team if not already a member
				members, _ := gw.teamMembers.ListByTeam(ctx, teamID)
				alreadyMember := false
				for _, m := range members {
					if m.UserID == user.ID {
						alreadyMember = true
						break
					}
				}
				if !alreadyMember {
					_, err := gw.teamMembers.Add(ctx, teamID, user.ID, dbmodels.Role(result.NewRole))
					if err != nil {
						gw.log.Warn("failed to add user to team",
							zap.String("user", user.Email),
							zap.String("team", teamSlug), zap.Error(err))
					} else {
						result.TeamsAdded = append(result.TeamsAdded, teamSlug)
					}
				}
			}
		}
	}

	// Apply role change if needed
	if result.NewRole != "" && result.NewRole != string(user.Role) {
		_, err := gw.users.Update(ctx, user.ID, dbmodels.UpdateUserRequest{
			Name: user.Name,
			Role: dbmodels.Role(result.NewRole),
		})
		if err != nil {
			gw.log.Error("failed to update user role from group sync",
				zap.String("user", user.Email),
				zap.String("new_role", result.NewRole),
				zap.Error(err))
		} else {
			result.Changed = true
			gw.log.Info("SSO group sync: role updated",
				zap.String("user", user.Email),
				zap.String("from", string(user.Role)),
				zap.String("to", result.NewRole),
			zap.Strings("idp_groups", idpGroups))
		}
	} else {
		result.NewRole = string(user.Role)
	}

	return result
}

// ── SAML group extraction ─────────────────────────────────────────────────────

// extractSAMLGroups pulls all values from the configured groups attribute.
// The attribute name is read from sso_configs (defaults to "groups").
func (gw *Gateway) extractSAMLGroups(ctx context.Context, xmlStr, orgID string) []string {
	// Read configured attribute name
	var groupAttr string
	gw.db.Pool.QueryRow(ctx,
		`SELECT COALESCE(group_attr, 'groups') FROM sso_configs WHERE org_id=$1 AND type='saml'`,
		orgID).Scan(&groupAttr)
	if groupAttr == "" {
		groupAttr = "groups"
	}

	// Extract all AttributeValue children for this attribute
	// Handles both saml:Attribute and Attribute namespaced tags
	var groups []string
	for _, prefix := range []string{"", "saml:"} {
		needle := fmt.Sprintf(`Name="%s"`, groupAttr)
		idx := strings.Index(xmlStr, needle)
		if idx == -1 {
			continue
		}
		// Scan forward for all AttributeValue elements
		sub := xmlStr[idx:]
		endAttr := strings.Index(sub, "</"+prefix+"Attribute>")
		if endAttr > 0 {
			sub = sub[:endAttr]
		}
		// Extract each value
		for {
			openTag  := "<" + prefix + "AttributeValue>"
			closeTag := "</" + prefix + "AttributeValue>"
			start := strings.Index(sub, openTag)
			if start == -1 {
				break
			}
			start += len(openTag)
			end := strings.Index(sub[start:], closeTag)
			if end == -1 {
				break
			}
			val := strings.TrimSpace(sub[start : start+end])
			if val != "" {
				groups = append(groups, val)
			}
			sub = sub[start+end+len(closeTag):]
		}
		if len(groups) > 0 {
			break
		}
	}
	return groups
}

// ── OIDC group extraction ─────────────────────────────────────────────────────

// extractOIDCGroups pulls the groups claim from raw id_token claims JSON.
// The claim name is configurable (defaults to "groups").
func extractOIDCGroups(claims map[string]any, claimName string) []string {
	if claimName == "" {
		claimName = "groups"
	}
	raw, ok := claims[claimName]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []any:
		groups := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				groups = append(groups, s)
			}
		}
		return groups
	case string:
		// Some IdPs send comma-separated string
		if v == "" {
			return nil
		}
		parts := strings.Split(v, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts
	}
	return nil
}

// ── Group sync management endpoints ──────────────────────────────────────────

// handleGetGroupMappings returns the current group→role mappings for an org.
// GET /api/v1/sso/:type/group-mappings
func (gw *Gateway) handleGetGroupMappings(c *fiber.Ctx) error {
	auth    := middleware.GetAuth(c)
	ssoType := c.Params("type")
	if ssoType == "" {
		ssoType = c.Query("type", "saml")
	}

	mappings, err := gw.loadGroupMappings(c.Context(), auth.OrgID, ssoType)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if mappings == nil {
		mappings = []GroupMapping{}
	}
	return c.JSON(fiber.Map{
		"org_id":   auth.OrgID,
		"sso_type": ssoType,
		"mappings": mappings,
		"total":    len(mappings),
	})
}

// handleSetGroupMappings replaces the group→role mappings for an org.
// PUT /api/v1/sso/:type/group-mappings
func (gw *Gateway) handleSetGroupMappings(c *fiber.Ctx) error {
	auth    := middleware.GetAuth(c)
	ssoType := c.Params("type")
	if ssoType == "" {
		ssoType = c.Query("type", "saml")
	}

	var body struct {
		Mappings []GroupMapping `json:"mappings"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	// Validate roles
	validRoles := map[string]bool{"viewer": true, "editor": true, "admin": true}
	for _, m := range body.Mappings {
		if m.IDPGroup == "" {
			return c.Status(400).JSON(fiber.Map{"error": "idp_group required in each mapping"})
		}
		if !validRoles[m.Role] {
			return c.Status(400).JSON(fiber.Map{
				"error": fmt.Sprintf("invalid role %q: must be viewer|editor|admin", m.Role),
			})
		}
	}

	mappingsJSON, _ := json.Marshal(body.Mappings)
	_, err := gw.db.Pool.Exec(c.Context(),
		`UPDATE sso_configs SET group_mappings=$1::jsonb, updated_at=NOW()
		 WHERE org_id=$2 AND type=$3`,
		string(mappingsJSON), auth.OrgID, ssoType)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	gw.auditV2.Log(c.Context(), dbmodels.AuditEntryV2{
		OrgID: auth.OrgID, ActorID: auth.UserID, ActorEmail: auth.Email,
		Action:     dbmodels.Action("sso_group_sync_update"),
		Resource:   "sso_group_mappings",
		ResourceID: ssoType,
		Details:    fmt.Sprintf("%d mappings configured", len(body.Mappings)),
		IPAddress:  c.IP(),
	})

	return c.JSON(fiber.Map{
		"updated":  true,
		"mappings": body.Mappings,
		"total":    len(body.Mappings),
	})
}

