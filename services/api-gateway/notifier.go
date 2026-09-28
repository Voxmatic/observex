// services/api-gateway/notifier.go
// Alert notification delivery: Slack, PagerDuty, OpsGenie, email webhook.
// Called by the processor's alert evaluation loop and by handleTestIntegration.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// ── Notifier ──────────────────────────────────────────────────────────────────

type Notifier struct {
	client *http.Client
	log    *zap.Logger
	baseURL string // ObserveX public URL for deep links
}

func NewNotifier(client *http.Client, log *zap.Logger, baseURL string) *Notifier {
	return &Notifier{client: client, log: log, baseURL: baseURL}
}

// Send dispatches an alert to one or more channels based on route config.
// channelDSNs is a comma-separated list like "slack:https://hooks.slack.com/...,pagerduty:key123"
func (n *Notifier) Send(ctx context.Context, alert AlertPayload, channelDSNs string) {
	channels := strings.Split(channelDSNs, ",")
	for _, ch := range channels {
		ch = strings.TrimSpace(ch)
		if ch == "" { continue }

		parts := strings.SplitN(ch, ":", 2)
		if len(parts) < 2 { continue }
		provider, endpoint := parts[0], parts[1]

		var err error
		switch strings.ToLower(provider) {
		case "slack":
			err = n.sendSlack(ctx, alert, "https:"+endpoint)
		case "pagerduty":
			err = n.sendPagerDuty(ctx, alert, endpoint)
		case "opsgenie":
			err = n.sendOpsGenie(ctx, alert, endpoint)
		case "webhook":
			err = n.sendWebhook(ctx, alert, "https:"+endpoint)
		case "email":
			err = n.sendEmailWebhook(ctx, alert, endpoint)
		default:
			n.log.Warn("unknown notification provider", zap.String("provider", provider))
		}

		if err != nil {
			n.log.Error("notification delivery failed",
				zap.String("provider", provider),
				zap.String("alert", alert.AlertName),
				zap.Error(err))
		} else {
			n.log.Info("notification delivered",
				zap.String("provider", provider),
				zap.String("alert", alert.AlertName),
				zap.String("state", alert.State))
		}
	}
}

// ── Slack ─────────────────────────────────────────────────────────────────────

func (n *Notifier) sendSlack(ctx context.Context, alert AlertPayload, webhookURL string) error {
	emoji := map[string]string{
		"CRITICAL": "🔴", "HIGH": "🟠", "MEDIUM": "🟡", "LOW": "🔵",
	}[alert.Severity]
	if emoji == "" { emoji = "⚪" }

	stateEmoji := "🔥"
	if alert.State == "resolved" { stateEmoji = "✅" }

	color := map[string]string{
		"CRITICAL": "#E53E3E", "HIGH": "#DD6B20", "MEDIUM": "#D69E2E", "LOW": "#3182CE",
	}[alert.Severity]
	if color == "" { color = "#718096" }
	if alert.State == "resolved" { color = "#38A169" }

	text := fmt.Sprintf("%s %s *%s* — %s", stateEmoji, emoji, alert.AlertName, alert.State)
	fallback := fmt.Sprintf("[%s] %s: %s", alert.Severity, alert.AlertName, alert.State)

	fields := []map[string]any{
		{"title": "Service",   "value": alert.Service,   "short": true},
		{"title": "Severity",  "value": alert.Severity,  "short": true},
		{"title": "Namespace", "value": alert.Namespace,  "short": true},
		{"title": "Value",     "value": fmt.Sprintf("%.2f (threshold: %.2f)", alert.Value, alert.Threshold), "short": true},
	}
	if alert.RunbookURL != "" {
		fields = append(fields, map[string]any{"title": "Runbook", "value": fmt.Sprintf("<%s|View Runbook>", alert.RunbookURL), "short": false})
	}

	payload := map[string]any{
		"text": text,
		"attachments": []map[string]any{{
			"fallback":    fallback,
			"color":       color,
			"text":        fmt.Sprintf("*%s* · `%s` · %s", alert.Service, alert.Namespace, alert.StartsAt.Format("15:04:05 UTC")),
			"fields":      fields,
			"footer":      "ObserveX Alerting",
			"footer_icon": "https://observex.io/favicon.ico",
			"ts":          alert.StartsAt.Unix(),
		}},
	}

	return n.postJSON(ctx, webhookURL, "", payload)
}

// ── PagerDuty ─────────────────────────────────────────────────────────────────

func (n *Notifier) sendPagerDuty(ctx context.Context, alert AlertPayload, routingKey string) error {
	eventAction := "trigger"
	if alert.State == "resolved" { eventAction = "resolve" }

	dedupKey := fmt.Sprintf("observex-%s-%s-%s", alert.OrgID, alert.AlertName, alert.Service)

	severity := strings.ToLower(alert.Severity)
	if severity == "high" { severity = "error" }
	if severity == "medium" { severity = "warning" }
	if severity == "low" { severity = "info" }

	payload := map[string]any{
		"routing_key":  routingKey,
		"event_action": eventAction,
		"dedup_key":    dedupKey,
		"payload": map[string]any{
			"summary":        fmt.Sprintf("[%s] %s — %s/%s", alert.Severity, alert.AlertName, alert.Namespace, alert.Service),
			"source":         "ObserveX",
			"severity":       severity,
			"timestamp":      alert.StartsAt.Format(time.RFC3339),
			"component":      alert.Service,
			"group":          alert.Namespace,
			"class":          alert.AlertName,
			"custom_details": map[string]any{
				"value":     alert.Value,
				"threshold": alert.Threshold,
				"labels":    alert.Labels,
				"org_id":    alert.OrgID,
			},
		},
		"links": []map[string]string{
			{"href": n.baseURL + "/incidents", "text": "View in ObserveX"},
		},
	}
	if alert.RunbookURL != "" {
		payload["links"] = append(payload["links"].([]map[string]string),
			map[string]string{"href": alert.RunbookURL, "text": "Runbook"})
	}

	return n.postJSON(ctx, "https://events.pagerduty.com/v2/enqueue", "", payload)
}

// ── OpsGenie ─────────────────────────────────────────────────────────────────

func (n *Notifier) sendOpsGenie(ctx context.Context, alert AlertPayload, apiKey string) error {
	if alert.State == "resolved" {
		// Close the alert
		alias := fmt.Sprintf("observex-%s-%s", alert.AlertName, alert.Service)
		url := fmt.Sprintf("https://api.opsgenie.com/v2/alerts/%s/close?identifierType=alias", alias)
		payload := map[string]any{"source": "ObserveX", "note": "Alert resolved by ObserveX"}
		return n.postJSON(ctx, url, "GenieKey "+apiKey, payload)
	}

	priority := map[string]string{
		"CRITICAL": "P1", "HIGH": "P2", "MEDIUM": "P3", "LOW": "P4",
	}[alert.Severity]
	if priority == "" { priority = "P3" }

	payload := map[string]any{
		"message":     fmt.Sprintf("[%s] %s — %s", alert.Severity, alert.AlertName, alert.Service),
		"alias":       fmt.Sprintf("observex-%s-%s", alert.AlertName, alert.Service),
		"description": fmt.Sprintf("Alert %s firing for %s/%s. Value: %.2f (threshold: %.2f)", alert.AlertName, alert.Namespace, alert.Service, alert.Value, alert.Threshold),
		"source":      "ObserveX",
		"priority":    priority,
		"tags":        []string{"observex", alert.Severity, alert.Namespace},
		"details": map[string]string{
			"org_id":    alert.OrgID,
			"namespace": alert.Namespace,
			"service":   alert.Service,
		},
	}
	if alert.RunbookURL != "" {
		payload["details"].(map[string]string)["runbook"] = alert.RunbookURL
	}

	return n.postJSON(ctx, "https://api.opsgenie.com/v2/alerts", "GenieKey "+apiKey, payload)
}

// ── Generic Webhook ───────────────────────────────────────────────────────────

func (n *Notifier) sendWebhook(ctx context.Context, alert AlertPayload, url string) error {
	return n.postJSON(ctx, url, "", alert)
}

// ── Email (via webhook relay) ─────────────────────────────────────────────────

func (n *Notifier) sendEmailWebhook(ctx context.Context, alert AlertPayload, recipient string) error {
	// Sends to a configured email relay webhook (e.g. SendGrid, Mailgun webhook)
	emailRelay := envOr("EMAIL_WEBHOOK_URL", "")
	if emailRelay == "" {
		n.log.Warn("EMAIL_WEBHOOK_URL not configured, email notification skipped",
			zap.String("recipient", recipient))
		return nil
	}
	payload := map[string]any{
		"to":      recipient,
		"subject": fmt.Sprintf("[ObserveX %s] %s — %s", alert.Severity, alert.AlertName, alert.State),
		"body": fmt.Sprintf(
			"Alert: %s\nService: %s\nSeverity: %s\nState: %s\nValue: %.2f (threshold: %.2f)\nNamespace: %s\nTime: %s",
			alert.AlertName, alert.Service, alert.Severity, alert.State,
			alert.Value, alert.Threshold, alert.Namespace,
			alert.StartsAt.Format(time.RFC3339),
		),
	}
	return n.postJSON(ctx, emailRelay, "", payload)
}

// ── Integration test ──────────────────────────────────────────────────────────

// TestIntegration sends a test notification to verify connectivity.
// cfgJSON is the decrypted integration config JSON.
func (n *Notifier) TestIntegration(ctx context.Context, cfgJSON string) (bool, string) {
	var cfg map[string]string
	if err := json.Unmarshal([]byte(cfgJSON), &cfg); err != nil {
		return false, "invalid config JSON: " + err.Error()
	}

	testAlert := AlertPayload{
		AlertName:  "ObserveX Test Notification",
		Severity:   "LOW",
		State:      "firing",
		Service:    "observex-test",
		Namespace:  "default",
		Value:      1.0,
		Threshold:  1.0,
		StartsAt:   time.Now(),
		OrgID:      "test",
	}

	if url, ok := cfg["webhook_url"]; ok && url != "" {
		err := n.sendSlack(ctx, testAlert, url)
		if err != nil { return false, "Slack test failed: " + err.Error() }
		return true, "Test notification sent to Slack ✓"
	}
	if key, ok := cfg["routing_key"]; ok && key != "" {
		err := n.sendPagerDuty(ctx, testAlert, key)
		if err != nil { return false, "PagerDuty test failed: " + err.Error() }
		return true, "Test alert sent to PagerDuty ✓"
	}
	if key, ok := cfg["api_key"]; ok && key != "" {
		err := n.sendOpsGenie(ctx, testAlert, key)
		if err != nil { return false, "OpsGenie test failed: " + err.Error() }
		return true, "Test alert sent to OpsGenie ✓"
	}

	return false, "no recognized channel config (webhook_url / routing_key / api_key)"
}

// ── HTTP helper ───────────────────────────────────────────────────────────────

func (n *Notifier) postJSON(ctx context.Context, url, authHeader string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil { return fmt.Errorf("marshal: %w", err) }

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil { return fmt.Errorf("request: %w", err) }
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" { req.Header.Set("Authorization", authHeader) }

	resp, err := n.client.Do(req)
	if err != nil { return fmt.Errorf("send: %w", err) }
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("endpoint returned HTTP %d", resp.StatusCode)
	}
	return nil
}
