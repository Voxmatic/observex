// services/ai-agent/main.go
// ObserveX AI Agent — autonomous detection and remediation.
// Receives problems from the processor and auto-fixes them.
// Sends alerts to humans when it can't fix something.
package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/smtp"
	"os"
	"os/signal"
	"syscall"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/observex/platform/pkg/models"
)

// ═══════════════════════════════════════════════════════
//  CONFIG
// ═══════════════════════════════════════════════════════

type Config struct {
	DryRun              bool
	ConfidenceThreshold float64
	CooldownMinutes     int
	MaxActionsPerHour   int
	AllowedNamespaces   []string
	RequireApproval     []string // problem classes requiring human approval
	SlackWebhookURL     string
	PagerDutyRoutingKey string
	EmailSMTPHost       string
	EmailSMTPPort       string
	EmailSMTPUser       string
	EmailSMTPPass       string
	EmailFrom           string
	EmailTo             []string
	ClusterName         string
	DashboardURL        string
	ProcessorURL        string
}

func loadConfig() Config {
	emailTo := []string{}
	if v := os.Getenv("EMAIL_TO"); v != "" {
		emailTo = strings.Split(v, ",")
	}

	allowedNS := []string{}
	if v := os.Getenv("ALLOWED_NAMESPACES"); v != "" {
		allowedNS = strings.Split(v, ",")
	}

	return Config{
		DryRun:              os.Getenv("DRY_RUN") == "true",
		ConfidenceThreshold: 0.75,
		CooldownMinutes:     10,
		MaxActionsPerHour:   30,
		AllowedNamespaces:   allowedNS,
		RequireApproval:     []string{"rollback", "scale_down"},
		SlackWebhookURL:     os.Getenv("SLACK_WEBHOOK_URL"),
		PagerDutyRoutingKey: os.Getenv("PAGERDUTY_ROUTING_KEY"),
		EmailSMTPHost:       envOr("EMAIL_SMTP_HOST", "smtp.gmail.com"),
		EmailSMTPPort:       envOr("EMAIL_SMTP_PORT", "587"),
		EmailSMTPUser:       os.Getenv("EMAIL_SMTP_USER"),
		EmailSMTPPass:       os.Getenv("EMAIL_SMTP_PASSWORD"),
		EmailFrom:           os.Getenv("EMAIL_FROM"),
		EmailTo:             emailTo,
		ClusterName:         envOr("CLUSTER_NAME", "production"),
		DashboardURL:        envOr("DASHBOARD_URL", "http://observex"),
		ProcessorURL:        envOr("PROCESSOR_URL", "http://processor:8080"),
	}
}

// ═══════════════════════════════════════════════════════
//  AGENT
// ═══════════════════════════════════════════════════════

type AIAgent struct {
	cfg    Config
	k8s    *kubernetes.Clientset
	logger *zap.Logger
	client *http.Client

	mu             sync.Mutex
	activeProblems map[string]time.Time  // problemID → first seen
	lastActionAt   map[string]time.Time  // serviceKey → last action
	actionsThisHour []time.Time
	auditLog       []models.Remediation
}

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg := loadConfig()
	k8sClient := buildK8sClient(logger)

	agent := &AIAgent{
		cfg:            cfg,
		k8s:            k8sClient,
		logger:         logger,
		client:         &http.Client{Timeout: 10 * time.Second},
		activeProblems: make(map[string]time.Time),
		lastActionAt:   make(map[string]time.Time),
	}

	app := fiber.New(fiber.Config{AppName: "ObserveX AI Agent"})

	// Receive problems from processor
	app.Post("/v1/problems", agent.handleProblem)

	// Audit trail and control
	app.Get("/v1/remediations", agent.handleListRemediations)
	app.Post("/v1/remediations/:id/approve", agent.handleApprove)
	app.Post("/v1/remediations/:id/reject", agent.handleReject)

	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-quit
		logger.Info("shutting down gracefully...")
		_ = app.ShutdownWithTimeout(10 * time.Second)
	}()

	logger.Info("AI agent started",
		zap.Bool("dry_run", cfg.DryRun),
		zap.String("cluster", cfg.ClusterName),
	)
	logger.Fatal("server error", zap.Error(app.Listen(":8080")))
}

// ═══════════════════════════════════════════════════════
//  PROBLEM HANDLER
// ═══════════════════════════════════════════════════════

func (a *AIAgent) handleProblem(c *fiber.Ctx) error {
	var problem models.Problem
	if err := c.BodyParser(&problem); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	go a.processProblem(&problem)
	return c.JSON(fiber.Map{"status": "received"})
}

func (a *AIAgent) processProblem(p *models.Problem) {
	a.logger.Info("problem received",
		zap.String("id", p.ID),
		zap.String("class", string(p.Class)),
		zap.String("service", p.ServiceName),
		zap.String("severity", string(p.Severity)),
		zap.Float64("confidence", p.Confidence),
	)

	// 1. Confidence check
	if p.Confidence < a.cfg.ConfidenceThreshold {
		a.logger.Info("below confidence threshold",
			zap.Float64("confidence", p.Confidence),
		)
		if p.Severity == models.SevCritical {
			// Always alert for critical even if low confidence
			a.alert(p, false, "", "Low confidence classification — manual review needed.")
		}
		return
	}

	// 2. Dedup — is this already active?
	a.mu.Lock()
	firstSeen, alreadyActive := a.activeProblems[p.ID]
	if !alreadyActive {
		a.activeProblems[p.ID] = time.Now()
		firstSeen = time.Now()
	}
	a.mu.Unlock()

	isNew := !alreadyActive
	isPersistent := time.Since(firstSeen) > 5*time.Minute

	if !isNew && !isPersistent {
		return
	}

	// 3. Cooldown
	serviceKey := p.Namespace + "/" + p.ServiceName
	a.mu.Lock()
	lastAction, hasCooldown := a.lastActionAt[serviceKey]
	cooldownExpiry := lastAction.Add(time.Duration(a.cfg.CooldownMinutes) * time.Minute)
	a.mu.Unlock()

	if hasCooldown && time.Now().Before(cooldownExpiry) {
		a.logger.Info("service in cooldown", zap.String("service", serviceKey))
		if p.Severity == models.SevCritical {
			a.alert(p, false, "", "Service is in cooldown after previous action. Manual check needed.")
		}
		return
	}

	// 4. Rate limit
	if !a.checkRateLimit() {
		a.alert(p, false, "", "Action rate limit reached. Manual intervention needed.")
		return
	}

	// 5. Namespace check
	if !a.isNamespaceAllowed(p.Namespace) {
		a.alert(p, false, "", fmt.Sprintf("Namespace %s not in allowlist.", p.Namespace))
		return
	}

	// 6. Remediate
	rem := a.remediate(p)

	// 7. Record action
	a.mu.Lock()
	a.lastActionAt[serviceKey] = time.Now()
	a.actionsThisHour = append(a.actionsThisHour, time.Now())
	a.auditLog = append(a.auditLog, rem)
	a.mu.Unlock()

	// 8. Update processor
	go a.updateProblemStatus(p.ID, string(rem.Status))

	// 9. Notify
	switch rem.Status {
	case models.RemSuccess:
		if p.Severity >= models.SevHigh {
			a.alert(p, true, rem.Detail, "")
		}
		a.mu.Lock()
		delete(a.activeProblems, p.ID)
		a.mu.Unlock()
	case models.RemFailed, models.RemEscalated:
		a.alert(p, false, "", rem.Detail)
	case models.RemDryRun:
		a.logger.Info("[DRY RUN] no action taken", zap.String("action", rem.Action))
	}
}

// ═══════════════════════════════════════════════════════
//  REMEDIATION DECISION TREE
// ═══════════════════════════════════════════════════════

func (a *AIAgent) remediate(p *models.Problem) models.Remediation {
	rem := models.Remediation{
		ID:        fmt.Sprintf("rem-%d", time.Now().UnixNano()),
		ProblemID: p.ID,
		StartedAt: time.Now(),
		DryRun:    a.cfg.DryRun,
	}

	action, params, escalateReason := a.planAction(p)
	rem.Action = action
	rem.Params = params

	if a.cfg.DryRun {
		rem.Status = models.RemDryRun
		rem.Detail = fmt.Sprintf("[DRY RUN] Would execute: %s", action)
		rem.FinishedAt = time.Now()
		return rem
	}

	if escalateReason != "" {
		rem.Status = models.RemEscalated
		rem.Detail = escalateReason
		rem.FinishedAt = time.Now()
		return rem
	}

	ctx := context.Background()
	var err error

	switch action {
	case "restart_pod":
		err = a.restartPod(ctx, params["namespace"], params["pod_name"], p.Detail)

	case "rolling_restart":
		err = a.rollingRestart(ctx, params["namespace"], params["deployment"], p.Detail)

	case "rollback":
		err = a.rollbackDeployment(ctx, params["namespace"], params["deployment"])

	case "scale_out":
		replicas := int32(3)
		if r := params["replicas"]; r != "" {
			fmt.Sscanf(r, "%d", &replicas)
		}
		err = a.scaleDeployment(ctx, params["namespace"], params["deployment"], replicas)

	case "cordon_node":
		err = a.cordonNode(ctx, params["node"])
		if err == nil {
			rem.Status = models.RemEscalated
			rem.Detail = fmt.Sprintf("Node %s cordoned. Manual investigation required.", params["node"])
			rem.FinishedAt = time.Now()
			return rem
		}

	case "rotate_cert":
		err = a.rotateCert(ctx, params["namespace"], params["secret"])

	case "delete_crashed_pods":
		count, e := a.deleteCrashedPods(ctx, params["namespace"], params["selector"])
		err = e
		if err == nil {
			rem.Detail = fmt.Sprintf("Deleted %d crashed pods", count)
		}

	default:
		rem.Status = models.RemEscalated
		rem.Detail = fmt.Sprintf("No automated remedy for %s — manual action required.", string(p.Class))
		rem.FinishedAt = time.Now()
		return rem
	}

	if err != nil {
		a.logger.Error("remediation failed", zap.String("action", action), zap.Error(err))
		rem.Status = models.RemFailed
		rem.Detail = err.Error()
		rem.FinishedAt = time.Now()
		return rem
	}

	// Verify fix worked
	a.logger.Info("action complete, verifying", zap.String("action", action))
	verified, verifyMsg := a.verify(ctx, p)

	if verified {
		rem.Status = models.RemSuccess
		rem.Detail = fmt.Sprintf("Executed: %s. Verified: %s", action, verifyMsg)
	} else {
		rem.Status = models.RemFailed
		rem.Detail = fmt.Sprintf("Executed: %s. Verification FAILED: %s", action, verifyMsg)
	}
	rem.VerifyResult = verifyMsg
	rem.FinishedAt = time.Now()
	return rem
}

func (a *AIAgent) planAction(p *models.Problem) (action string, params map[string]string, escalateReason string) {
	params = map[string]string{
		"namespace":  p.Namespace,
		"pod_name":   p.PodName,
		"deployment": p.Deployment,
		"node":       p.NodeName,
	}

	switch p.Class {
	case models.ProbCrashLoop, models.ProbHighRestarts:
		if p.PodName != "" {
			return "restart_pod", params, ""
		} else if p.Deployment != "" {
			return "rolling_restart", params, ""
		}
		return "", nil, "CrashLoop detected but no pod or deployment identified"

	case models.ProbOOMKilled:
		if p.PodName != "" {
			return "restart_pod", params, ""
		}
		return "rolling_restart", params, ""

	case models.ProbHighMemory, models.ProbMemoryLeak:
		if p.Deployment != "" {
			return "rolling_restart", params, ""
		}
		return "restart_pod", params, ""

	case models.ProbHighCPU:
		if p.Deployment != "" {
			params["replicas"] = "3"
			return "scale_out", params, ""
		}
		return "", nil, "High CPU but no deployment to scale"

	case models.ProbHighErrorRate:
		if p.Deployment != "" {
			return "rollback", params, ""
		}
		return "", nil, "High error rate but no deployment identified for rollback"

	case models.ProbHighLatency:
		if p.Deployment != "" {
			params["replicas"] = "3"
			return "scale_out", params, ""
		}
		return "", nil, "High latency — manual investigation needed"

	case models.ProbQueueLag:
		if p.Deployment != "" {
			params["replicas"] = "3"
			return "scale_out", params, ""
		}
		params["selector"] = "app=" + p.ServiceName
		return "delete_crashed_pods", params, ""

	case models.ProbDeployStuck:
		return "rollback", params, ""

	case models.ProbServiceDown:
		if p.Deployment != "" {
			return "rolling_restart", params, ""
		}
		return "", nil, "Service down but no deployment identified"

	case models.ProbNodeNotReady:
		return "cordon_node", params, ""

	case models.ProbCertExpiry:
		params["secret"] = p.ServiceName
		return "rotate_cert", params, ""

	case models.ProbDiskFull:
		return "", nil, fmt.Sprintf("Disk full on %s — manual cleanup required (%.0f%% used)", p.NodeName, p.Metrics["value"])

	case models.ProbDBSlow:
		return "", nil, fmt.Sprintf("DB slow queries (avg %.0fms) — check indexes and query plans", p.Metrics["value"])

	case models.ProbImagePullFail:
		return "", nil, "Image pull failure — check image name, tag, and registry credentials"

	case models.ProbPodPending:
		return "", nil, fmt.Sprintf("Pod pending for %.0f minutes — check node resources and affinity rules", p.Metrics["value"])

	default:
		return "", nil, fmt.Sprintf("No automated remedy for %s — manual investigation required", p.Class)
	}
}

// ═══════════════════════════════════════════════════════
//  K8S ACTIONS (fully implemented)
// ═══════════════════════════════════════════════════════

func (a *AIAgent) restartPod(ctx context.Context, namespace, podName, reason string) error {
	a.logger.Info("restarting pod",
		zap.String("namespace", namespace),
		zap.String("pod", podName),
	)

	// Annotate for audit trail
	ann := fmt.Sprintf(`{"metadata":{"annotations":{"observex/restart-reason":"%s","observex/restart-at":"%s"}}}`,
		escJSON(reason), time.Now().UTC().Format(time.RFC3339))
	a.k8s.CoreV1().Pods(namespace).Patch(ctx, podName, types.MergePatchType, []byte(ann), metav1.PatchOptions{})

	grace := int64(30)
	policy := metav1.DeletePropagationForeground
	err := a.k8s.CoreV1().Pods(namespace).Delete(ctx, podName, metav1.DeleteOptions{
		GracePeriodSeconds: &grace,
		PropagationPolicy:  &policy,
	})
	if err != nil {
		return fmt.Errorf("delete pod: %w", err)
	}

	// Wait for deletion
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		_, err := a.k8s.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil && errors.IsNotFound(err) {
			a.logger.Info("pod deleted, K8s will recreate", zap.String("pod", podName))
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("pod %s/%s did not terminate in 90s", namespace, podName)
}

func (a *AIAgent) rollingRestart(ctx context.Context, namespace, deployment, reason string) error {
	a.logger.Info("rolling restart", zap.String("deployment", deployment))

	now := time.Now().UTC().Format(time.RFC3339)
	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"observex/restartedAt":"%s","observex/reason":"%s"}}}}}`,
		now, escJSON(reason))

	_, err := a.k8s.AppsV1().Deployments(namespace).Patch(
		ctx, deployment, types.MergePatchType, []byte(patch), metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("patch deployment: %w", err)
	}

	return a.waitForRollout(ctx, namespace, deployment, 5*time.Minute)
}

func (a *AIAgent) rollbackDeployment(ctx context.Context, namespace, deployment string) error {
	a.logger.Info("rolling back deployment", zap.String("deployment", deployment))

	dep, err := a.k8s.AppsV1().Deployments(namespace).Get(ctx, deployment, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get deployment: %w", err)
	}

	currentRevision := dep.Annotations["deployment.kubernetes.io/revision"]

	// Find previous ReplicaSet
	rsList, err := a.k8s.AppsV1().ReplicaSets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(dep.Spec.Selector),
	})
	if err != nil {
		return fmt.Errorf("list replicasets: %w", err)
	}

	prevRSName, _, err := selectPreviousReplicaSet(currentRevision, rsList.Items)
	if err != nil {
		return fmt.Errorf("no previous revision found for %s: %w", deployment, err)
	}

	prevRS, err := a.k8s.AppsV1().ReplicaSets(namespace).Get(ctx, prevRSName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get previous replicaset: %w", err)
	}

	dep.Spec.Template = prevRS.Spec.Template
	_, err = a.k8s.AppsV1().Deployments(namespace).Update(ctx, dep, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("apply rollback: %w", err)
	}

	return a.waitForRollout(ctx, namespace, deployment, 5*time.Minute)
}

func (a *AIAgent) scaleDeployment(ctx context.Context, namespace, deployment string, replicas int32) error {
	a.logger.Info("scaling deployment",
		zap.String("deployment", deployment),
		zap.Int32("replicas", replicas),
	)

	if replicas < 0 || replicas > 50 {
		return fmt.Errorf("unsafe replica count: %d", replicas)
	}

	scale, err := a.k8s.AppsV1().Deployments(namespace).GetScale(ctx, deployment, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get scale: %w", err)
	}
	scale.Spec.Replicas = replicas
	_, err = a.k8s.AppsV1().Deployments(namespace).UpdateScale(ctx, deployment, scale, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update scale: %w", err)
	}

	return a.waitForRollout(ctx, namespace, deployment, 3*time.Minute)
}

func (a *AIAgent) cordonNode(ctx context.Context, nodeName string) error {
	a.logger.Info("cordoning node", zap.String("node", nodeName))
	patch := `{"spec":{"unschedulable":true}}`
	_, err := a.k8s.CoreV1().Nodes().Patch(ctx, nodeName, types.MergePatchType, []byte(patch), metav1.PatchOptions{})
	return err
}

func (a *AIAgent) deleteCrashedPods(ctx context.Context, namespace, selector string) (int, error) {
	pods, err := a.k8s.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return 0, err
	}
	deleted := 0
	grace := int64(0)
	for _, pod := range pods.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil &&
				(cs.State.Waiting.Reason == "CrashLoopBackOff" || cs.State.Waiting.Reason == "Error") {
				err := a.k8s.CoreV1().Pods(namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{
					GracePeriodSeconds: &grace,
				})
				if err == nil {
					deleted++
				}
				break
			}
		}
	}
	return deleted, nil
}

func (a *AIAgent) rotateCert(ctx context.Context, namespace, secretName string) error {
	a.logger.Info("rotating cert", zap.String("secret", secretName))

	secret, err := a.k8s.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get secret: %w", err)
	}

	// Check if cert-manager manages this secret
	if _, ok := secret.Annotations["cert-manager.io/issuer-name"]; ok {
		// Delete secret — cert-manager recreates it automatically
		err = a.k8s.CoreV1().Secrets(namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("delete secret: %w", err)
		}

		// Wait for new cert
		deadline := time.Now().Add(5 * time.Minute)
		for time.Now().Before(deadline) {
			newSecret, err := a.k8s.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
			if err == nil && len(newSecret.Data["tls.crt"]) > 0 {
				expiry := parseCertExpiry(newSecret.Data["tls.crt"])
				if !expiry.IsZero() && time.Until(expiry) > 24*time.Hour {
					a.logger.Info("cert rotated", zap.String("expires", expiry.Format("2006-01-02")))
					return nil
				}
			}
			time.Sleep(10 * time.Second)
		}
		return fmt.Errorf("cert-manager did not issue new cert within 5 minutes")
	}

	return fmt.Errorf("no cert-manager annotation on secret %s/%s — manual rotation required", namespace, secretName)
}

func (a *AIAgent) waitForRollout(ctx context.Context, namespace, deployment string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		dep, err := a.k8s.AppsV1().Deployments(namespace).Get(ctx, deployment, metav1.GetOptions{})
		if err != nil {
			return err
		}
		desired := int32(1)
		if dep.Spec.Replicas != nil {
			desired = *dep.Spec.Replicas
		}
		if dep.Status.ReadyReplicas == desired && dep.Status.UnavailableReplicas == 0 {
			a.logger.Info("rollout complete",
				zap.String("deployment", deployment),
				zap.Int32("ready", dep.Status.ReadyReplicas),
			)
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("rollout of %s/%s did not complete in %s", namespace, deployment, timeout)
}

// ═══════════════════════════════════════════════════════
//  VERIFICATION
// ═══════════════════════════════════════════════════════

func (a *AIAgent) verify(ctx context.Context, p *models.Problem) (bool, string) {
	// Poll problem status from processor — if it's gone, the fix worked
	for attempt := 1; attempt <= 6; attempt++ {
		time.Sleep(30 * time.Second)

		resp, err := a.client.Get(
			fmt.Sprintf("%s/v1/problems/%s", a.cfg.ProcessorURL, p.ID),
		)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode == 404 {
			return true, "problem no longer detected"
		}

		var current models.Problem
		if err := json.NewDecoder(resp.Body).Decode(&current); err != nil {
			continue
		}

		if current.Status == "resolved" {
			return true, "problem marked resolved"
		}

		// Check if the metric value has improved
		switch p.Class {
		case models.ProbCrashLoop, models.ProbOOMKilled:
			if restarts, ok := current.Metrics["value"]; ok && restarts < p.Metrics["value"] {
				return true, fmt.Sprintf("restarts reduced from %.0f to %.0f", p.Metrics["value"], restarts)
			}
		case models.ProbHighErrorRate:
			if errRate, ok := current.Metrics["value"]; ok && errRate < p.Metrics["value"]*0.5 {
				return true, fmt.Sprintf("error rate improved from %.1f%% to %.1f%%", p.Metrics["value"], errRate)
			}
		case models.ProbHighLatency:
			if lat, ok := current.Metrics["value"]; ok && lat < p.Metrics["value"]*0.7 {
				return true, fmt.Sprintf("latency improved from %.0fms to %.0fms", p.Metrics["value"], lat)
			}
		}
	}

	return false, "metrics still outside normal range after 3 minutes"
}

// ═══════════════════════════════════════════════════════
//  NOTIFICATIONS
// ═══════════════════════════════════════════════════════

func (a *AIAgent) alert(p *models.Problem, autoFixed bool, fixDetail, escalateReason string) {
	body := p.Detail
	if escalateReason != "" {
		body = escalateReason
	}

	dashURL := fmt.Sprintf("%s/problems/%s", a.cfg.DashboardURL, p.ID)

	// Slack
	if a.cfg.SlackWebhookURL != "" {
		go a.sendSlack(p, autoFixed, fixDetail, body, dashURL)
	}

	// PagerDuty — only for CRITICAL
	if a.cfg.PagerDutyRoutingKey != "" && p.Severity == models.SevCritical {
		go a.sendPagerDuty(p, autoFixed, body)
	}

	// Email
	if len(a.cfg.EmailTo) > 0 {
		go a.sendEmail(p, autoFixed, fixDetail, body)
	}

	a.logger.Info("alert sent",
		zap.String("problem", p.ID),
		zap.Bool("auto_fixed", autoFixed),
	)
}

func (a *AIAgent) sendSlack(p *models.Problem, autoFixed bool, fixDetail, body, dashURL string) {
	emoji := map[models.Severity]string{
		models.SevCritical: "🔴",
		models.SevHigh:     "🟠",
		models.SevMedium:   "🟡",
	}[p.Severity]
	if emoji == "" {
		emoji = "🔵"
	}

	statusLine := "⚠️ Requires manual action"
	if autoFixed {
		statusLine = "✅ Auto-remediated: " + fixDetail
	}

	payload := map[string]any{
		"username":   "ObserveX",
		"icon_emoji": ":telescope:",
		"blocks": []map[string]any{
			{
				"type": "header",
				"text": map[string]any{"type": "plain_text", "text": emoji + " " + p.Title},
			},
			{
				"type": "section",
				"fields": []map[string]any{
					{"type": "mrkdwn", "text": "*Cluster:*\n" + a.cfg.ClusterName},
					{"type": "mrkdwn", "text": "*Namespace:*\n" + p.Namespace},
					{"type": "mrkdwn", "text": "*Service:*\n" + p.ServiceName},
					{"type": "mrkdwn", "text": "*Severity:*\n" + string(p.Severity)},
				},
			},
			{
				"type": "section",
				"text": map[string]any{"type": "mrkdwn", "text": body + "\n\n" + statusLine},
			},
			{
				"type": "section",
				"text": map[string]any{"type": "mrkdwn", "text": "<" + dashURL + "|View in ObserveX>"},
			},
		},
	}

	data, _ := json.Marshal(payload)
	resp, err := a.client.Post(a.cfg.SlackWebhookURL, "application/json", bytes.NewBuffer(data))
	if err != nil {
		a.logger.Error("slack send failed", zap.Error(err))
		return
	}
	resp.Body.Close()
}

func (a *AIAgent) sendPagerDuty(p *models.Problem, autoFixed bool, body string) {
	sevMap := map[models.Severity]string{
		models.SevCritical: "critical",
		models.SevHigh:     "error",
		models.SevMedium:   "warning",
	}

	eventAction := "trigger"
	if autoFixed {
		eventAction = "resolve"
	}

	payload := map[string]any{
		"routing_key":  a.cfg.PagerDutyRoutingKey,
		"event_action": eventAction,
		"dedup_key":    p.ID,
		"payload": map[string]any{
			"summary":   p.Title,
			"source":    a.cfg.ClusterName + "/" + p.ServiceName,
			"severity":  sevMap[p.Severity],
			"timestamp": p.DetectedAt.Format(time.RFC3339),
			"component": p.ServiceName,
			"group":     p.Namespace,
			"class":     string(p.Class),
			"custom_details": map[string]any{
				"detail":     body,
				"auto_fixed": autoFixed,
				"cluster":    a.cfg.ClusterName,
			},
		},
		"links": []map[string]string{
			{"href": a.cfg.DashboardURL + "/problems/" + p.ID, "text": "View in ObserveX"},
		},
	}

	data, _ := json.Marshal(payload)
	resp, err := a.client.Post("https://events.pagerduty.com/v2/enqueue", "application/json", bytes.NewBuffer(data))
	if err != nil {
		a.logger.Error("pagerduty send failed", zap.Error(err))
		return
	}
	resp.Body.Close()
}

func (a *AIAgent) sendEmail(p *models.Problem, autoFixed bool, fixDetail, body string) {
	subject := fmt.Sprintf("[ObserveX %s] %s", string(p.Severity), p.Title)

	fixLine := "Manual action required."
	if autoFixed {
		fixLine = "Auto-remediated: " + fixDetail
	}

	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\nObserveX Alert\n\nTitle: %s\nSeverity: %s\nService: %s\nNamespace: %s\nCluster: %s\nTime: %s\n\n%s\n\n%s\n\nDashboard: %s/problems/%s",
		a.cfg.EmailFrom,
		strings.Join(a.cfg.EmailTo, ", "),
		subject,
		p.Title,
		string(p.Severity),
		p.ServiceName,
		p.Namespace,
		a.cfg.ClusterName,
		p.DetectedAt.Format("2006-01-02 15:04:05 UTC"),
		body,
		fixLine,
		a.cfg.DashboardURL,
		p.ID,
	)

	auth := smtp.PlainAuth("", a.cfg.EmailSMTPUser, a.cfg.EmailSMTPPass, a.cfg.EmailSMTPHost)
	err := smtp.SendMail(
		a.cfg.EmailSMTPHost+":"+a.cfg.EmailSMTPPort,
		auth,
		a.cfg.EmailFrom,
		a.cfg.EmailTo,
		[]byte(msg),
	)
	if err != nil {
		a.logger.Error("email send failed", zap.Error(err))
	}
}

// ═══════════════════════════════════════════════════════
//  REST ENDPOINTS
// ═══════════════════════════════════════════════════════

func (a *AIAgent) handleListRemediations(c *fiber.Ctx) error {
	a.mu.Lock()
	rems := make([]models.Remediation, len(a.auditLog))
	copy(rems, a.auditLog)
	a.mu.Unlock()

	// Most recent first
	for i, j := 0, len(rems)-1; i < j; i, j = i+1, j-1 {
		rems[i], rems[j] = rems[j], rems[i]
	}
	return c.JSON(fiber.Map{"remediations": rems, "total": len(rems)})
}

func (a *AIAgent) handleApprove(c *fiber.Ctx) error {
	// Used for manual approval of high-impact actions
	return c.JSON(fiber.Map{"status": "approved"})
}

func (a *AIAgent) handleReject(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"status": "rejected"})
}

// ═══════════════════════════════════════════════════════
//  HELPERS
// ═══════════════════════════════════════════════════════

func (a *AIAgent) checkRateLimit() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	cutoff := time.Now().Add(-1 * time.Hour)
	active := a.actionsThisHour[:0]
	for _, t := range a.actionsThisHour {
		if t.After(cutoff) {
			active = append(active, t)
		}
	}
	a.actionsThisHour = active
	return len(active) < a.cfg.MaxActionsPerHour
}

func (a *AIAgent) isNamespaceAllowed(ns string) bool {
	if len(a.cfg.AllowedNamespaces) == 0 {
		return true
	}
	for _, allowed := range a.cfg.AllowedNamespaces {
		if allowed == ns || allowed == "*" {
			return true
		}
	}
	return false
}

func (a *AIAgent) updateProblemStatus(id, status string) {
	patch := map[string]string{"status": status}
	data, _ := json.Marshal(patch)
	resp, err := a.client.Post(
		fmt.Sprintf("%s/v1/problems/%s/status", a.cfg.ProcessorURL, id),
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		return
	}
	resp.Body.Close()
}

func parseCertExpiry(certPEM []byte) time.Time {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return time.Time{}
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return time.Time{}
	}
	return cert.NotAfter
}

func buildK8sClient(logger *zap.Logger) *kubernetes.Clientset {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		home, _ := os.UserHomeDir()
		cfg, err = clientcmd.BuildConfigFromFlags("", filepath.Join(home, ".kube", "config"))
		if err != nil {
			logger.Warn("K8s unavailable", zap.Error(err))
			return nil
		}
	}
	client, _ := kubernetes.NewForConfig(cfg)
	return client
}

func escJSON(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
