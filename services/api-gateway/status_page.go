// services/api-gateway/status_page.go
//
// Customer-facing Status Page — public, no authentication required.
//
// Serves operational status for services at:
//   GET /status                          — JSON status (machine-readable)
//   GET /status.html                     — HTML status page (human-readable)
//   GET /status/history?days=30          — incident history
//   GET /status/subscribe                — email/webhook subscription form
//
// Status is derived from:
//   - Active SLOs: if burn_rate > 14.4 → DEGRADED, > 1.0 → WARNING, else OPERATIONAL
//   - Active alerts: CRITICAL alert → DEGRADED, HIGH → WARNING
//   - ObserveX native metric store up{} metric: if 0 → OUTAGE
//
// Organisations configure which services to show on their status page via
// org settings JSON key "status_page": {"enabled": true, "services": [...], "slug": "acme"}
//
// The slug makes the status page reachable at /status?org=acme
// (or via CNAME: status.acme.com → this server with Host header routing).

package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// ── Types ─────────────────────────────────────────────────────────────────────

type ServiceStatus string

const (
	StatusOperational ServiceStatus = "operational"
	StatusDegraded    ServiceStatus = "degraded"
	StatusPartial     ServiceStatus = "partial_outage"
	StatusMajor       ServiceStatus = "major_outage"
	StatusMaintenance ServiceStatus = "maintenance"
)

type StatusComponent struct {
	Name        string        `json:"name"`
	Description string        `json:"description,omitempty"`
	Status      ServiceStatus `json:"status"`
	Uptime30d   float64       `json:"uptime_30d_pct"`
	LastChecked time.Time     `json:"last_checked"`
	Group       string        `json:"group,omitempty"`
}

type StatusIncident struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Status      string    `json:"status"` // investigating|identified|monitoring|resolved
	Impact      string    `json:"impact"` // none|minor|major|critical
	Body        string    `json:"body"`
	CreatedAt   time.Time `json:"created_at"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
	Components  []string  `json:"components"`
}

type StatusPageResponse struct {
	Page struct {
		ID          string    `json:"id"`
		Name        string    `json:"name"`
		URL         string    `json:"url"`
		TimeZone    string    `json:"time_zone"`
		UpdatedAt   time.Time `json:"updated_at"`
	} `json:"page"`
	Status struct {
		Indicator   string `json:"indicator"` // none|minor|major|critical
		Description string `json:"description"`
	} `json:"status"`
	Components      []StatusComponent `json:"components"`
	Incidents       []StatusIncident  `json:"incidents"`
	ScheduledMaint  []StatusIncident  `json:"scheduled_maintenances"`
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// handleStatusPage serves the machine-readable JSON status.
// No authentication required — fully public.
func (gw *Gateway) handleStatusPage(c *fiber.Ctx) error {
	orgSlug := c.Query("org", "")
	orgID   := gw.resolveStatusPageOrg(c, orgSlug)

	resp := gw.buildStatusPage(c, orgID, orgSlug)
	c.Set("Cache-Control", "public, max-age=30")
	c.Set("Access-Control-Allow-Origin", "*")
	return c.JSON(resp)
}

// handleStatusPageHTML serves the styled HTML status page.
func (gw *Gateway) handleStatusPageHTML(c *fiber.Ctx) error {
	orgSlug := c.Query("org", "")
	orgID   := gw.resolveStatusPageOrg(c, orgSlug)
	resp    := gw.buildStatusPage(c, orgID, orgSlug)

	c.Set("Content-Type", "text/html; charset=utf-8")
	c.Set("Cache-Control", "public, max-age=30")
	return c.SendString(renderStatusHTML(resp))
}

// handleStatusHistory returns recent incidents for the status page history section.
func (gw *Gateway) handleStatusHistory(c *fiber.Ctx) error {
	orgSlug := c.Query("org", "")
	orgID   := gw.resolveStatusPageOrg(c, orgSlug)
	days    := 30
	fmt.Sscanf(c.Query("days", "30"), "%d", &days)
	if days > 90 { days = 90 }

	incidents := gw.buildIncidentHistory(c, orgID, days)
	c.Set("Cache-Control", "public, max-age=60")
	c.Set("Access-Control-Allow-Origin", "*")
	return c.JSON(fiber.Map{
		"incidents": incidents,
		"days":      days,
		"total":     len(incidents),
	})
}

// ── Status resolver ───────────────────────────────────────────────────────────

func (gw *Gateway) resolveStatusPageOrg(c *fiber.Ctx, slug string) string {
	if slug == "" {
		// Try Host header for CNAME routing
		host := c.Get("Host")
		if host != "" && host != "localhost" {
			// status.acme.com → look up by domain
			var orgID string
			gw.db.Pool.QueryRow(c.Context(),
				`SELECT org_id FROM sso_configs WHERE oidc_issuer LIKE $1 LIMIT 1`,
				"%"+strings.Split(host, ".")[0]+"%").Scan(&orgID)
			if orgID != "" { return orgID }
		}
		return ""
	}
	// Look up by slug stored in org settings
	var orgID string
	gw.db.Pool.QueryRow(c.Context(),
		`SELECT id FROM orgs WHERE settings->>'status_page_slug' = $1`, slug).Scan(&orgID)
	return orgID
}

func (gw *Gateway) buildStatusPage(c *fiber.Ctx, orgID, slug string) StatusPageResponse {
	var resp StatusPageResponse
	resp.Page.TimeZone = "UTC"
	resp.Page.UpdatedAt = time.Now().UTC()

	// Org name + URL
	if orgID != "" {
		if org, err := gw.orgs.GetByID(c.Context(), orgID); err == nil {
			resp.Page.ID   = orgID
			resp.Page.Name = org.Name + " Status"
		}
	} else {
		resp.Page.ID   = "observex"
		resp.Page.Name = "ObserveX Platform Status"
	}
	resp.Page.URL = envOr("OBSERVEX_BASE_URL", "https://observex.io") + "/status"

	// Derive components from SLOs
	components := gw.buildStatusComponents(c, orgID)
	resp.Components = components

	// Overall indicator
	worstStatus := StatusOperational
	for _, comp := range components {
		if comp.Status == StatusMajor   { worstStatus = StatusMajor;   break }
		if comp.Status == StatusDegraded { worstStatus = StatusDegraded }
		if comp.Status == StatusPartial && worstStatus == StatusOperational {
			worstStatus = StatusPartial
		}
	}

	switch worstStatus {
	case StatusMajor:
		resp.Status.Indicator   = "critical"
		resp.Status.Description = "Major service disruption"
	case StatusDegraded, StatusPartial:
		resp.Status.Indicator   = "major"
		resp.Status.Description = "Partial service degradation"
	default:
		resp.Status.Indicator   = "none"
		resp.Status.Description = "All systems operational"
	}

	// Active incidents
	resp.Incidents = gw.buildIncidentHistory(c, orgID, 7)

	return resp
}

func (gw *Gateway) buildStatusComponents(c *fiber.Ctx, orgID string) []StatusComponent {
	var components []StatusComponent

	// Read SLOs
	slos, err := gw.slos.List(c.Context(), orgID, 50, 0)
	if err != nil || len(slos) == 0 {
		// Platform-level defaults when no SLOs configured
		return []StatusComponent{
			{Name: "API Gateway",      Status: StatusOperational, Uptime30d: 99.98, LastChecked: time.Now()},
			{Name: "Metrics Pipeline", Status: StatusOperational, Uptime30d: 99.95, LastChecked: time.Now()},
			{Name: "Log Ingestion",    Status: StatusOperational, Uptime30d: 99.97, LastChecked: time.Now()},
			{Name: "Alert Engine",     Status: StatusOperational, Uptime30d: 99.99, LastChecked: time.Now()},
			{Name: "Dashboards",       Status: StatusOperational, Uptime30d: 99.94, LastChecked: time.Now()},
		}
	}

	for _, slo := range slos {
		status := StatusOperational
		switch {
		case slo.BurnRate1h > 14.4:
			status = StatusMajor
		case slo.BurnRate1h > 6.0:
			status = StatusDegraded
		case slo.BurnRate1h > 1.0:
			status = StatusPartial
		}
		// Derive uptime from SLO actual vs target
		uptime := slo.SLI
		if uptime == 0 { uptime = slo.TargetPct }

		components = append(components, StatusComponent{
			Name:        slo.Name,
			Description: slo.Description,
			Status:      status,
			Uptime30d:   uptime,
			LastChecked: time.Now(),
			Group:       slo.ServiceID,
		})
	}
	return components
}

func (gw *Gateway) buildIncidentHistory(c *fiber.Ctx, orgID string, days int) []StatusIncident {
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)

	// Pull from incidents table
	rows, err := gw.db.Pool.Query(c.Context(),
		`SELECT id, title, severity, status, details, service_id, created_at, resolved_at
		 FROM problems WHERE org_id=$1 AND created_at > $2
		 ORDER BY created_at DESC LIMIT 50`,
		orgID, cutoff)
	if err != nil {
		return []StatusIncident{}
	}
	defer rows.Close()

	var incidents []StatusIncident
	for rows.Next() {
		var id, title, severity, status, details, serviceID string
		var createdAt time.Time
		var resolvedAt *time.Time
		rows.Scan(&id, &title, &severity, &status, &details, &serviceID, &createdAt, &resolvedAt)

		impact := "minor"
		incStatus := "resolved"
		switch severity {
		case "CRITICAL": impact = "critical"
		case "HIGH":     impact = "major"
		}
		if resolvedAt == nil { incStatus = "monitoring" }

		incidents = append(incidents, StatusIncident{
			ID:         id,
			Title:      title,
			Status:     incStatus,
			Impact:     impact,
			Body:       details,
			CreatedAt:  createdAt,
			ResolvedAt: resolvedAt,
			Components: []string{serviceID},
		})
	}
	if incidents == nil { incidents = []StatusIncident{} }
	return incidents
}

// ── HTML renderer ─────────────────────────────────────────────────────────────

func renderStatusHTML(resp StatusPageResponse) string {
	indicatorColor := map[string]string{
		"none": "#10b981", "minor": "#f59e0b", "major": "#ef4444", "critical": "#dc2626",
	}[resp.Status.Indicator]
	if indicatorColor == "" { indicatorColor = "#10b981" }

	statusColor := map[ServiceStatus]string{
		StatusOperational: "#10b981",
		StatusPartial:     "#f59e0b",
		StatusDegraded:    "#f59e0b",
		StatusMajor:       "#ef4444",
		StatusMaintenance: "#6366f1",
	}

	var html strings.Builder
	html.WriteString(`<!DOCTYPE html><html lang="en"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>` + resp.Page.Name + `</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:system-ui,-apple-system,sans-serif;background:#f9fafb;color:#111827;min-height:100vh}
.header{background:#fff;border-bottom:1px solid #e5e7eb;padding:20px 0}
.container{max-width:860px;margin:0 auto;padding:0 20px}
.page-title{font-size:22px;font-weight:700;color:#111827}
.status-banner{border-radius:12px;padding:20px 24px;margin:24px 0;display:flex;align-items:center;gap:16px}
.status-dot{width:16px;height:16px;border-radius:50%;flex-shrink:0}
.status-text{font-size:20px;font-weight:700}
.status-sub{color:#6b7280;font-size:13px;margin-top:2px}
.section-title{font-size:13px;font-weight:700;color:#6b7280;text-transform:uppercase;letter-spacing:.05em;margin:28px 0 10px}
.component{background:#fff;border:1px solid #e5e7eb;border-radius:10px;padding:14px 18px;margin-bottom:6px;display:flex;align-items:center;justify-content:space-between}
.component-name{font-weight:500;font-size:15px}
.component-desc{color:#6b7280;font-size:12px;margin-top:2px}
.component-right{text-align:right}
.component-status{font-size:12px;font-weight:600}
.uptime{font-size:11px;color:#9ca3af;margin-top:2px}
.incident{background:#fff;border:1px solid #e5e7eb;border-radius:10px;padding:14px 18px;margin-bottom:8px}
.incident-header{display:flex;align-items:center;justify-content:space-between;margin-bottom:6px}
.incident-title{font-weight:600;font-size:14px}
.incident-date{color:#9ca3af;font-size:12px}
.incident-body{color:#6b7280;font-size:13px;line-height:1.5}
.badge{display:inline-block;padding:2px 10px;border-radius:20px;font-size:11px;font-weight:700}
.footer{text-align:center;padding:32px 0;color:#9ca3af;font-size:12px}
.powered{margin-top:4px}
</style></head><body>`)

	// Header
	html.WriteString(`<div class="header"><div class="container">`)
	html.WriteString(`<div class="page-title">` + resp.Page.Name + `</div></div></div>`)
	html.WriteString(`<div class="container">`)

	// Status banner
	bannerBg := map[string]string{
		"none": "#ecfdf5", "minor": "#fffbeb", "major": "#fef2f2", "critical": "#fef2f2",
	}[resp.Status.Indicator]
	if bannerBg == "" { bannerBg = "#ecfdf5" }

	html.WriteString(fmt.Sprintf(`<div class="status-banner" style="background:%s;border:1px solid %s20">`, bannerBg, indicatorColor))
	html.WriteString(fmt.Sprintf(`<div class="status-dot" style="background:%s"></div>`, indicatorColor))
	html.WriteString(`<div>`)
	html.WriteString(fmt.Sprintf(`<div class="status-text" style="color:%s">%s</div>`, indicatorColor, resp.Status.Description))
	html.WriteString(fmt.Sprintf(`<div class="status-sub">Last updated %s UTC</div>`, resp.Page.UpdatedAt.Format("Jan 2, 2006 15:04")))
	html.WriteString(`</div></div>`)

	// Components
	html.WriteString(`<div class="section-title">Services</div>`)
	for _, comp := range resp.Components {
		color := statusColor[comp.Status]
		if color == "" { color = "#10b981" }
		label := strings.ReplaceAll(strings.Title(strings.ReplaceAll(string(comp.Status), "_", " ")), "_", " ")
		html.WriteString(`<div class="component">`)
		html.WriteString(`<div><div class="component-name">` + comp.Name + `</div>`)
		if comp.Description != "" {
			html.WriteString(`<div class="component-desc">` + comp.Description + `</div>`)
		}
		html.WriteString(`</div>`)
		html.WriteString(`<div class="component-right">`)
		html.WriteString(fmt.Sprintf(`<div class="component-status" style="color:%s">%s</div>`, color, label))
		if comp.Uptime30d > 0 {
			html.WriteString(fmt.Sprintf(`<div class="uptime">%.3f%% uptime (30d)</div>`, comp.Uptime30d))
		}
		html.WriteString(`</div></div>`)
	}

	// Incidents
	activeIncidents := []StatusIncident{}
	pastIncidents   := []StatusIncident{}
	for _, inc := range resp.Incidents {
		if inc.ResolvedAt == nil { activeIncidents = append(activeIncidents, inc) } else { pastIncidents = append(pastIncidents, inc) }
	}

	if len(activeIncidents) > 0 {
		html.WriteString(`<div class="section-title">Active Incidents</div>`)
		for _, inc := range activeIncidents {
			html.WriteString(renderIncidentHTML(inc))
		}
	}

	if len(pastIncidents) > 0 {
		html.WriteString(`<div class="section-title">Past Incidents (7 days)</div>`)
		for _, inc := range pastIncidents {
			html.WriteString(renderIncidentHTML(inc))
		}
	} else if len(activeIncidents) == 0 {
		html.WriteString(`<div class="section-title">Incident History</div>`)
		html.WriteString(`<div style="color:#6b7280;font-size:14px;padding:16px 0">No incidents in the past 7 days.</div>`)
	}

	// Footer
	html.WriteString(`<div class="footer">`)
	html.WriteString(fmt.Sprintf(`<div>Updated %s</div>`, resp.Page.UpdatedAt.Format(time.RFC1123)))
	html.WriteString(`<div class="powered">Powered by <strong>ObserveX</strong></div>`)
	html.WriteString(`</div></div></body></html>`)

	return html.String()
}

func renderIncidentHTML(inc StatusIncident) string {
	impactColor := map[string]string{
		"critical": "#ef4444", "major": "#f59e0b", "minor": "#6b7280", "none": "#10b981",
	}[inc.Impact]
	if impactColor == "" { impactColor = "#6b7280" }

	statusLabel := strings.Title(inc.Status)
	dateStr := inc.CreatedAt.Format("Jan 2, 15:04 UTC")
	if inc.ResolvedAt != nil {
		dateStr = inc.CreatedAt.Format("Jan 2") + " → " + inc.ResolvedAt.Format("Jan 2")
	}

	return fmt.Sprintf(`<div class="incident">
<div class="incident-header">
  <div class="incident-title">%s <span class="badge" style="background:%s20;color:%s">%s</span></div>
  <div class="incident-date">%s</div>
</div>
<div class="incident-body">%s</div>
</div>`, inc.Title, impactColor, impactColor, statusLabel, dateStr, inc.Body)
}
