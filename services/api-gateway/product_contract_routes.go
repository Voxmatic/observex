package main

import (
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/observex/platform/internal/middleware"
)

var productContracts = struct {
	sync.RWMutex
	runbooks map[string]fiber.Map
	workloads map[string]fiber.Map
	anomalyRules map[string]fiber.Map
	hubInstalled map[string]bool
}{
	runbooks: map[string]fiber.Map{
		"rb-001": {
			"id": "rb-001", "name": "DB Connection Pool Recovery", "category": "database",
			"severity": "CRITICAL", "execTime": "3-5m", "lastRun": "19:45:00",
			"runs": 47, "successRate": 94, "autoTrigger": true,
			"description": "Restores PostgreSQL connection pool when exhausted.",
			"steps": []fiber.Map{
				{"id": "s1", "title": "Identify blocked queries", "type": "check", "command": "SELECT pid, query, state FROM pg_stat_activity WHERE wait_event_type IS NOT NULL;", "expected": "List of blocking PIDs", "done": false},
				{"id": "s2", "title": "Kill long-running queries", "type": "action", "command": "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE state != 'active' AND query_start < NOW() - INTERVAL '5 min';", "expected": "Queries terminated", "done": false},
				{"id": "s3", "title": "Verify pool recovery", "type": "check", "command": "SELECT count(*) FROM pg_stat_activity;", "expected": "Count < 150", "done": false},
			},
		},
		"rb-002": {
			"id": "rb-002", "name": "K8s Pod CrashLoop Recovery", "category": "kubernetes",
			"severity": "HIGH", "execTime": "2-4m", "lastRun": "16:12:00",
			"runs": 23, "successRate": 87, "autoTrigger": true,
			"description": "Diagnoses and recovers pods in CrashLoopBackOff state.",
			"steps": []fiber.Map{
				{"id": "s1", "title": "Get crash logs", "type": "check", "command": "kubectl logs <pod-name> --previous -n production", "expected": "Error stack trace", "done": false},
				{"id": "s2", "title": "Restart pod", "type": "action", "command": "kubectl rollout restart deployment/<name> -n production", "expected": "Rollout triggered", "done": false},
			},
		},
	},
	workloads: map[string]fiber.Map{
		"wl-checkout": {
			"id": "wl-checkout", "name": "Checkout Team", "owner": "commerce@corp.io",
			"env": "production", "health": "degraded", "healthPct": 72,
			"entities": []fiber.Map{
				{"name": "checkout-service", "type": "SERVICE", "status": "warning", "p99": "892ms", "err": "3.2%"},
				{"name": "payment-service", "type": "SERVICE", "status": "healthy", "p99": "98ms", "err": "0.08%"},
				{"name": "postgres-primary", "type": "DATABASE", "status": "critical"},
			},
			"slos": []fiber.Map{
				{"name": "Checkout success rate", "target": 99.9, "current": 96.2, "ok": false},
				{"name": "Payment P99 < 500ms", "target": 99.5, "current": 99.9, "ok": true},
			},
			"errorCount": 247, "openProblems": 2,
		},
		"wl-platform": {
			"id": "wl-platform", "name": "Platform Team", "owner": "platform@corp.io",
			"env": "production", "health": "critical", "healthPct": 45,
			"entities": []fiber.Map{
				{"name": "api-gateway", "type": "SERVICE", "status": "warning", "p99": "284ms", "err": "1.84%"},
				{"name": "user-service", "type": "SERVICE", "status": "critical", "p99": "4200ms", "err": "8.4%"},
			},
			"slos": []fiber.Map{
				{"name": "API availability", "target": 99.9, "current": 91.2, "ok": false},
			},
			"errorCount": 1842, "openProblems": 1,
		},
	},
	anomalyRules: map[string]fiber.Map{
		"ad-001": {"id": "ad-001", "name": "Response Time Spike", "type": "BASELINE", "metric": "http.response_time.p99", "services": []string{"api-gateway", "checkout-service"}, "sensitivity": 2, "enabled": true, "triggered": 12, "lastFired": "19:24:11", "algorithm": "AUTO_ADAPTIVE", "window": "5m"},
		"ad-002": {"id": "ad-002", "name": "Error Rate Anomaly", "type": "BASELINE", "metric": "http.error_rate", "services": []string{"all"}, "sensitivity": 1, "enabled": true, "triggered": 8, "lastFired": "18:44:00", "algorithm": "AUTO_ADAPTIVE", "window": "3m"},
	},
	hubInstalled: map[string]bool{
		"aws-cloudwatch": true,
		"gcp": true,
		"postgres-ext": true,
		"redis-ext": true,
		"prom": true,
		"pagerduty": true,
		"slack-ext": true,
		"github-actions": true,
		"snyk": true,
		"otel": true,
	},
}

func (gw *Gateway) registerProductContractRoutes(api fiber.Router, mw *middleware.RBAC) {
	api.Get("/workloads", mw.RequireNamespaceRead(), gw.handleWorkloadList)
	api.Post("/workloads", mw.RequireEditor(), gw.handleWorkloadCreate)
	api.Get("/workloads/:id", mw.RequireNamespaceRead(), gw.handleWorkloadGet)
	api.Put("/workloads/:id", mw.RequireEditor(), gw.handleWorkloadUpdate)
	api.Delete("/workloads/:id", mw.RequireEditor(), gw.handleWorkloadDelete)

	api.Get("/mobile/apps", mw.RequireNamespaceRead(), gw.handleMobileApps)
	api.Get("/mobile/apps/:id/crashes", mw.RequireNamespaceRead(), gw.handleMobileCrashes)
	api.Get("/mobile/apps/:id/sessions", mw.RequireNamespaceRead(), gw.handleMobileSessions)
	api.Get("/mobile/apps/:id/metrics", mw.RequireNamespaceRead(), gw.handleMobileMetrics)
	api.Get("/mobile/apps/:id/network", mw.RequireNamespaceRead(), gw.handleMobileNetwork)

	api.Get("/hub/extensions", mw.RequireNamespaceRead(), gw.handleHubExtensions)
	api.Post("/hub/extensions/:id/install", mw.RequireEditor(), gw.handleHubInstall)
	api.Delete("/hub/extensions/:id", mw.RequireEditor(), gw.handleHubUninstall)
	api.Get("/hub/installed", mw.RequireNamespaceRead(), gw.handleHubInstalled)

	api.Get("/anomaly/rules", mw.RequireNamespaceRead(), gw.handleAnomalyRules)
	api.Post("/anomaly/rules", mw.RequireEditor(), gw.handleAnomalyCreate)
	api.Put("/anomaly/rules/:id", mw.RequireEditor(), gw.handleAnomalyUpdate)
	api.Delete("/anomaly/rules/:id", mw.RequireEditor(), gw.handleAnomalyDelete)
	api.Post("/anomaly/preview", mw.RequireNamespaceRead(), gw.handleAnomalyPreview)

	api.Get("/llm/calls", mw.RequireNamespaceRead(), gw.handleLLMCalls)
}

func contractOrgID(c *fiber.Ctx) string {
	if auth := middleware.GetAuth(c); auth != nil && auth.OrgID != "" {
		return auth.OrgID
	}
	return "org-default"
}

func toAnyMap(item fiber.Map) map[string]any {
	out := map[string]any{}
	for k, v := range item {
		out[k] = v
	}
	return out
}

func toFiberMap(item map[string]any) fiber.Map {
	out := fiber.Map{}
	for k, v := range item {
		out[k] = v
	}
	return out
}

func anyMapsToFiber(items []map[string]any) []fiber.Map {
	out := make([]fiber.Map, 0, len(items))
	for _, item := range items {
		out = append(out, toFiberMap(item))
	}
	return out
}

func fallbackSlice(defaults map[string]fiber.Map) []fiber.Map {
	productContracts.RLock()
	defer productContracts.RUnlock()
	return mapsToSlice(defaults)
}

func (gw *Gateway) seedFeatureDefaults(c *fiber.Ctx, orgID, kind string, defaults map[string]fiber.Map) ([]fiber.Map, error) {
	out := make([]fiber.Map, 0, len(defaults))
	for id, item := range defaults {
		saved, err := gw.featureState.Upsert(c.Context(), orgID, kind, id, toAnyMap(item))
		if err != nil {
			return nil, err
		}
		out = append(out, toFiberMap(saved))
	}
	return out, nil
}

func (gw *Gateway) listFeatureItems(c *fiber.Ctx, kind, responseKey string, defaults map[string]fiber.Map) error {
	orgID := contractOrgID(c)
	items, err := gw.featureState.List(c.Context(), orgID, kind)
	if err == nil {
		out := anyMapsToFiber(items)
		if len(out) == 0 && len(defaults) > 0 {
			if seeded, seedErr := gw.seedFeatureDefaults(c, orgID, kind, defaults); seedErr == nil {
				out = seeded
			}
		}
		return c.JSON(fiber.Map{responseKey: out, "total": len(out)})
	}
	out := fallbackSlice(defaults)
	return c.JSON(fiber.Map{responseKey: out, "total": len(out), "source": "fallback"})
}

func (gw *Gateway) getFeatureItem(c *fiber.Ctx, kind, label string, defaults map[string]fiber.Map) error {
	id := c.Params("id")
	item, err := gw.featureState.Get(c.Context(), contractOrgID(c), kind, id)
	if err == nil {
		return c.JSON(item)
	}
	productContracts.RLock()
	defer productContracts.RUnlock()
	if fallback, ok := defaults[id]; ok {
		return c.JSON(fallback)
	}
	return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": label + " not found"})
}

func (gw *Gateway) createFeatureItem(c *fiber.Ctx, kind, idPrefix string, defaults map[string]fiber.Map) error {
	item, err := parseMapBody(c)
	if err != nil {
		return err
	}
	id := stringValue(item["id"])
	if id == "" {
		id = idPrefix + "-" + uuid.NewString()
	}
	saved, err := gw.featureState.Upsert(c.Context(), contractOrgID(c), kind, id, toAnyMap(item))
	if err != nil {
		productContracts.Lock()
		item["id"] = id
		defaults[id] = item
		productContracts.Unlock()
		return c.Status(fiber.StatusCreated).JSON(item)
	}
	return c.Status(fiber.StatusCreated).JSON(saved)
}

func (gw *Gateway) updateFeatureItem(c *fiber.Ctx, kind, label string, defaults map[string]fiber.Map) error {
	body, err := parseMapBody(c)
	if err != nil {
		return err
	}
	id := c.Params("id")
	current, err := gw.featureState.Get(c.Context(), contractOrgID(c), kind, id)
	if err == nil {
		for k, v := range body {
			current[k] = v
		}
		saved, err := gw.featureState.Upsert(c.Context(), contractOrgID(c), kind, id, current)
		if err == nil {
			return c.JSON(saved)
		}
	}
	productContracts.Lock()
	defer productContracts.Unlock()
	item, ok := defaults[id]
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": label + " not found"})
	}
	for k, v := range body {
		item[k] = v
	}
	item["id"] = id
	return c.JSON(item)
}

func (gw *Gateway) deleteFeatureItem(c *fiber.Ctx, kind, label string, defaults map[string]fiber.Map) error {
	id := c.Params("id")
	if err := gw.featureState.Delete(c.Context(), contractOrgID(c), kind, id); err == nil {
		return c.SendStatus(fiber.StatusNoContent)
	}
	productContracts.Lock()
	defer productContracts.Unlock()
	if _, ok := defaults[id]; !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": label + " not found"})
	}
	delete(defaults, id)
	return c.SendStatus(fiber.StatusNoContent)
}

func fallbackHubInstalled() map[string]bool {
	productContracts.RLock()
	defer productContracts.RUnlock()
	installed := make(map[string]bool, len(productContracts.hubInstalled))
	for id, ok := range productContracts.hubInstalled {
		installed[id] = ok
	}
	return installed
}

func (gw *Gateway) hubInstalledLookup(c *fiber.Ctx) map[string]bool {
	orgID := contractOrgID(c)
	rows, err := gw.featureState.List(c.Context(), orgID, "hub_install")
	if err != nil {
		return fallbackHubInstalled()
	}
	if len(rows) == 0 {
		defaults := fallbackHubInstalled()
		for id, installed := range defaults {
			_, err := gw.featureState.Upsert(c.Context(), orgID, "hub_install", id, map[string]any{
				"id":        id,
				"installed": installed,
				"seeded":    true,
			})
			if err != nil {
				return defaults
			}
		}
		return defaults
	}
	installed := map[string]bool{}
	for _, row := range rows {
		id := stringValue(row["id"])
		if id != "" && boolDefault(row["installed"], false) {
			installed[id] = true
		}
	}
	return installed
}

func (gw *Gateway) handleRunbookList(c *fiber.Ctx) error {
	return gw.listFeatureItems(c, "runbook", "runbooks", productContracts.runbooks)
}

func (gw *Gateway) handleRunbookGet(c *fiber.Ctx) error {
	return gw.getFeatureItem(c, "runbook", "runbook", productContracts.runbooks)
}

func (gw *Gateway) handleRunbookCreate(c *fiber.Ctx) error {
	item, err := parseMapBody(c)
	if err != nil {
		return err
	}
	id := stringValue(item["id"])
	if id == "" {
		id = "rb-" + uuid.NewString()
	}
	item["id"] = id
	item["runs"] = numberValue(item["runs"], 0)
	item["successRate"] = numberValue(item["successRate"], 100)
	item["lastRun"] = stringDefault(item["lastRun"], "Never")
	saved, err := gw.featureState.Upsert(c.Context(), contractOrgID(c), "runbook", id, toAnyMap(item))
	if err != nil {
		productContracts.Lock()
		productContracts.runbooks[id] = item
		productContracts.Unlock()
		return c.Status(fiber.StatusCreated).JSON(item)
	}
	return c.Status(fiber.StatusCreated).JSON(saved)
}

func (gw *Gateway) handleRunbookUpdate(c *fiber.Ctx) error {
	return gw.updateFeatureItem(c, "runbook", "runbook", productContracts.runbooks)
}

func (gw *Gateway) handleRunbookDelete(c *fiber.Ctx) error {
	return gw.deleteFeatureItem(c, "runbook", "runbook", productContracts.runbooks)
}

func (gw *Gateway) handleRunbookExecute(c *fiber.Ctx) error {
	id := c.Params("id")
	orgID := contractOrgID(c)
	item, err := gw.featureState.Get(c.Context(), orgID, "runbook", id)
	if err != nil {
		productContracts.Lock()
		defer productContracts.Unlock()
		fallback, ok := productContracts.runbooks[id]
		if !ok {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "runbook not found"})
		}
		fallback["runs"] = int(numberValue(fallback["runs"], 0)) + 1
		fallback["lastRun"] = time.Now().Format("15:04:05")
		return c.JSON(fiber.Map{"id": id, "status": "completed", "started_at": time.Now().Add(-2 * time.Second), "finished_at": time.Now(), "steps": fallback["steps"]})
	}
	item["runs"] = int(numberValue(item["runs"], 0)) + 1
	item["lastRun"] = time.Now().Format("15:04:05")
	_, _ = gw.featureState.Upsert(c.Context(), orgID, "runbook", id, item)
	return c.JSON(fiber.Map{"id": id, "status": "completed", "started_at": time.Now().Add(-2 * time.Second), "finished_at": time.Now(), "steps": item["steps"]})
}

func (gw *Gateway) handleWorkloadList(c *fiber.Ctx) error {
	return gw.listFeatureItems(c, "workload", "workloads", productContracts.workloads)
}

func (gw *Gateway) handleWorkloadGet(c *fiber.Ctx) error {
	return gw.getFeatureItem(c, "workload", "workload", productContracts.workloads)
}

func (gw *Gateway) handleWorkloadCreate(c *fiber.Ctx) error {
	return gw.createFeatureItem(c, "workload", "wl", productContracts.workloads)
}

func (gw *Gateway) handleWorkloadUpdate(c *fiber.Ctx) error {
	return gw.updateFeatureItem(c, "workload", "workload", productContracts.workloads)
}

func (gw *Gateway) handleWorkloadDelete(c *fiber.Ctx) error {
	return gw.deleteFeatureItem(c, "workload", "workload", productContracts.workloads)
}

func (gw *Gateway) handleMobileApps(c *fiber.Ctx) error {
	apps := []fiber.Map{
		{"id": "ios-shop", "name": "ShopApp iOS", "platform": "iOS", "version": "4.2.1", "users": 142800, "crashes": 247, "crashRate": 0.173, "sessions": 312400, "sessionDuration": 4.2, "httpErrors": 1.84, "status": "warning", "icon": "iOS", "buildNum": "1842"},
		{"id": "android-shop", "name": "ShopApp Android", "platform": "Android", "version": "4.2.0", "users": 289400, "crashes": 814, "crashRate": 0.281, "sessions": 628100, "sessionDuration": 3.8, "httpErrors": 2.14, "status": "warning", "icon": "Android", "buildNum": "2041"},
		{"id": "ios-driver", "name": "Driver iOS", "platform": "iOS", "version": "2.1.4", "users": 18400, "crashes": 12, "crashRate": 0.065, "sessions": 84200, "sessionDuration": 22.4, "httpErrors": 0.42, "status": "healthy", "icon": "iOS", "buildNum": "412"},
	}
	return c.JSON(fiber.Map{"apps": apps, "total": len(apps)})
}

func (gw *Gateway) handleMobileCrashes(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"app_id": c.Params("id"), "crashes": []fiber.Map{
		{"id": "cr1", "title": "NullPointerException: UserProfileManager.fetchUser()", "count": 142, "users": 89, "version": "4.2.1", "os": "iOS 17.4", "firstSeen": "2h ago", "lastSeen": "4m ago", "status": "unresolved"},
		{"id": "cr2", "title": "NetworkOnMainThreadException: HttpClient.execute()", "count": 98, "users": 76, "version": "4.2.0", "os": "Android 14", "firstSeen": "6h ago", "lastSeen": "12m ago", "status": "unresolved"},
	}})
}

func (gw *Gateway) handleMobileSessions(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"app_id": c.Params("id"), "sessions": []fiber.Map{
		{"id": "ms-1", "duration_sec": 244, "country": "US", "status": "completed", "started_at": time.Now().Add(-12 * time.Minute)},
		{"id": "ms-2", "duration_sec": 96, "country": "IN", "status": "crashed", "started_at": time.Now().Add(-9 * time.Minute)},
	}})
}

func (gw *Gateway) handleMobileMetrics(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"app_id": c.Params("id"), "metrics": fiber.Map{"crash_free_rate": 99.72, "http_error_rate": 1.84, "avg_session_duration_min": 4.2, "p99_startup_ms": 1240}})
}

func (gw *Gateway) handleMobileNetwork(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"app_id": c.Params("id"), "requests": []fiber.Map{
		{"url": "/api/v2/products", "method": "GET", "avgTime": 284, "p99": 892, "errors": 3.2, "calls": 12840},
		{"url": "/api/v1/user/profile", "method": "GET", "avgTime": 4200, "p99": 8400, "errors": 8.4, "calls": 55800},
	}})
}

func (gw *Gateway) handleHubExtensions(c *fiber.Ctx) error {
	category := c.Query("category")
	search := strings.ToLower(c.Query("q"))
	extensions := hubCatalog()
	out := make([]fiber.Map, 0, len(extensions))
	installed := gw.hubInstalledLookup(c)
	for _, ext := range extensions {
		ext["installed"] = installed[stringValue(ext["id"])]
		if category != "" && category != "all" && stringValue(ext["category"]) != category {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(stringValue(ext["name"])+" "+stringValue(ext["vendor"])), search) {
			continue
		}
		out = append(out, ext)
	}
	return c.JSON(fiber.Map{"extensions": out, "total": len(out)})
}

func (gw *Gateway) handleHubInstall(c *fiber.Ctx) error {
	id := c.Params("id")
	_, err := gw.featureState.Upsert(c.Context(), contractOrgID(c), "hub_install", id, map[string]any{
		"id":           id,
		"installed":    true,
		"status":       "installed",
		"installed_at": time.Now(),
	})
	if err != nil {
		productContracts.Lock()
		productContracts.hubInstalled[id] = true
		productContracts.Unlock()
	}
	return c.JSON(fiber.Map{"id": id, "installed": true, "status": "installed"})
}

func (gw *Gateway) handleHubUninstall(c *fiber.Ctx) error {
	id := c.Params("id")
	_, err := gw.featureState.Upsert(c.Context(), contractOrgID(c), "hub_install", id, map[string]any{
		"id":             id,
		"installed":      false,
		"status":         "uninstalled",
		"uninstalled_at": time.Now(),
	})
	if err != nil {
		productContracts.Lock()
		delete(productContracts.hubInstalled, id)
		productContracts.Unlock()
	}
	return c.JSON(fiber.Map{"id": id, "installed": false, "status": "uninstalled"})
}

func (gw *Gateway) handleHubInstalled(c *fiber.Ctx) error {
	installedIDs := gw.hubInstalledLookup(c)
	installed := make([]fiber.Map, 0, len(installedIDs))
	for _, ext := range hubCatalog() {
		if installedIDs[stringValue(ext["id"])] {
			ext["installed"] = true
			installed = append(installed, ext)
		}
	}
	return c.JSON(fiber.Map{"extensions": installed, "total": len(installed)})
}

func (gw *Gateway) handleAnomalyRules(c *fiber.Ctx) error {
	return gw.listFeatureItems(c, "anomaly_rule", "rules", productContracts.anomalyRules)
}

func (gw *Gateway) handleAnomalyCreate(c *fiber.Ctx) error {
	item, err := parseMapBody(c)
	if err != nil {
		return err
	}
	id := stringValue(item["id"])
	if id == "" {
		id = "ad-" + uuid.NewString()
	}
	item["id"] = id
	item["enabled"] = boolDefault(item["enabled"], true)
	item["triggered"] = numberValue(item["triggered"], 0)
	item["lastFired"] = stringDefault(item["lastFired"], "Never")
	saved, err := gw.featureState.Upsert(c.Context(), contractOrgID(c), "anomaly_rule", id, toAnyMap(item))
	if err != nil {
		productContracts.Lock()
		productContracts.anomalyRules[id] = item
		productContracts.Unlock()
		return c.Status(fiber.StatusCreated).JSON(item)
	}
	return c.Status(fiber.StatusCreated).JSON(saved)
}

func (gw *Gateway) handleAnomalyUpdate(c *fiber.Ctx) error {
	return gw.updateFeatureItem(c, "anomaly_rule", "anomaly rule", productContracts.anomalyRules)
}

func (gw *Gateway) handleAnomalyDelete(c *fiber.Ctx) error {
	return gw.deleteFeatureItem(c, "anomaly_rule", "anomaly rule", productContracts.anomalyRules)
}

func (gw *Gateway) handleAnomalyPreview(c *fiber.Ctx) error {
	var body fiber.Map
	_ = c.BodyParser(&body)
	metric := stringDefault(body["metric"], "http.response_time.p99")
	return c.JSON(fiber.Map{
		"metric": metric,
		"window": stringDefault(body["window"], "5m"),
		"baseline": 214.5,
		"current": 892.0,
		"score": 3.8,
		"would_fire": true,
		"points": []fiber.Map{
			{"ts": time.Now().Add(-4 * time.Minute), "value": 220.1},
			{"ts": time.Now().Add(-3 * time.Minute), "value": 238.4},
			{"ts": time.Now().Add(-2 * time.Minute), "value": 510.8},
			{"ts": time.Now().Add(-1 * time.Minute), "value": 892.0},
		},
	})
}

func (gw *Gateway) handleLLMCalls(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"calls": []fiber.Map{
		{"id": "c1", "ts": "19:47:12", "model": "Claude Haiku 4.5", "prompt": "Analyze DB connection pool exhaustion and recommend actions", "tokens": 1840, "cost": "$0.00046", "latency": 380, "status": "success", "cached": false},
		{"id": "c2", "ts": "19:46:58", "model": "Llama3 (Local)", "prompt": "Root cause analysis: user-service error rate spike", "tokens": 2200, "cost": "$0.00", "latency": 820, "status": "success", "cached": false},
	}, "total": 2})
}

func parseMapBody(c *fiber.Ctx) (fiber.Map, error) {
	item := fiber.Map{}
	if err := c.BodyParser(&item); err != nil {
		return nil, c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	return item, nil
}

func getFromContractMap(c *fiber.Ctx, items map[string]fiber.Map, name string) error {
	productContracts.RLock()
	defer productContracts.RUnlock()
	item, ok := items[c.Params("id")]
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": name + " not found"})
	}
	return c.JSON(item)
}

func updateContractMap(c *fiber.Ctx, items map[string]fiber.Map, name string) error {
	body, err := parseMapBody(c)
	if err != nil {
		return err
	}
	id := c.Params("id")
	productContracts.Lock()
	defer productContracts.Unlock()
	item, ok := items[id]
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": name + " not found"})
	}
	for k, v := range body {
		item[k] = v
	}
	item["id"] = id
	return c.JSON(item)
}

func deleteFromContractMap(c *fiber.Ctx, items map[string]fiber.Map, name string) error {
	id := c.Params("id")
	productContracts.Lock()
	defer productContracts.Unlock()
	if _, ok := items[id]; !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": name + " not found"})
	}
	delete(items, id)
	return c.SendStatus(fiber.StatusNoContent)
}

func mapsToSlice(items map[string]fiber.Map) []fiber.Map {
	out := make([]fiber.Map, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	return out
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func stringDefault(v any, fallback string) string {
	if s := stringValue(v); s != "" {
		return s
	}
	return fallback
}

func numberValue(v any, fallback float64) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case float64:
		return n
	case float32:
		return float64(n)
	default:
		return fallback
	}
}

func boolDefault(v any, fallback bool) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return fallback
}

func hubCatalog() []fiber.Map {
	return []fiber.Map{
		{"id": "aws-cloudwatch", "name": "AWS CloudWatch", "category": "cloud", "vendor": "Amazon Web Services", "official": true, "stars": 4.8, "downloads": 284000, "version": "3.2.1", "description": "CloudWatch metrics for EC2, RDS, ECS, Lambda, SQS, and SNS.", "tags": []string{"aws", "metrics", "cloud"}, "icon": "cloud"},
		{"id": "postgres-ext", "name": "PostgreSQL Deep Dive", "category": "databases", "vendor": "ObserveX", "official": true, "stars": 4.9, "downloads": 312000, "version": "2.4.0", "description": "Query-level performance, wait events, lock analysis, explain plans, and autovacuum tracking.", "tags": []string{"postgres", "database", "sql"}, "icon": "postgres"},
		{"id": "observex-native", "name": "ObserveX Native Collector", "category": "monitoring", "vendor": "ObserveX", "official": true, "stars": 4.9, "downloads": 489000, "version": "5.0.0", "description": "Native host, process, container, database, trace, log, and topology collection through ObserveX Agent.", "tags": []string{"observex", "metrics", "logs", "traces"}, "icon": "observex"},
		{"id": "pagerduty", "name": "PagerDuty", "category": "alerting", "vendor": "PagerDuty", "official": true, "stars": 4.9, "downloads": 312000, "version": "4.2.0", "description": "Two-way incident creation, status sync, and escalation policy linking.", "tags": []string{"pagerduty", "oncall", "alerting"}, "icon": "pagerduty"},
		{"id": "github-actions", "name": "GitHub Actions", "category": "cicd", "vendor": "GitHub", "official": true, "stars": 4.8, "downloads": 284000, "version": "3.4.0", "description": "Deployment tracking, PR correlation, and DORA metrics from CI runs.", "tags": []string{"github", "cicd", "deployment"}, "icon": "github"},
		{"id": "otel", "name": "OpenTelemetry", "category": "apm", "vendor": "CNCF", "official": true, "stars": 5.0, "downloads": 512000, "version": "4.8.0", "description": "OTLP ingest for traces, metrics, and logs with auto-instrumentation guidance.", "tags": []string{"opentelemetry", "otel", "tracing"}, "icon": "otel"},
		{"id": "azure-monitor", "name": "Azure Monitor", "category": "cloud", "vendor": "Microsoft", "official": true, "stars": 4.6, "downloads": 176000, "version": "4.1.0", "description": "Azure Monitor metrics and logs for App Service, AKS, Cosmos DB, and Service Bus.", "tags": []string{"azure", "metrics", "cloud"}, "icon": "azure"},
		{"id": "snyk", "name": "Snyk Security", "category": "security", "vendor": "Snyk", "official": true, "stars": 4.8, "downloads": 198000, "version": "2.4.0", "description": "Vulnerability correlation with runtime context and ownership metadata.", "tags": []string{"snyk", "vulnerability", "security"}, "icon": "security"},
	}
}
