// internal/db/models/models.go
// Auth, RBAC, and user-management data models for the API gateway.
package models

import "time"

// ── Roles ─────────────────────────────────────────────────────────────────────

type Role string
const (
	RoleAdmin  Role = "admin"
	RoleEditor Role = "editor"
	RoleViewer Role = "viewer"
)

// ── Access levels ─────────────────────────────────────────────────────────────

type Access string
const (
	AccessRead  Access = "read"
	AccessWrite Access = "write"
)

// ── Audit actions ─────────────────────────────────────────────────────────────

type Action string
const (
	ActionLogin          Action = "login"
	ActionLoginFailed    Action = "login_failed"
	ActionLogout         Action = "logout"
	ActionPasswordChange Action = "password_change"
	ActionPasswordReset  Action = "password_reset"
	ActionAPIKeyCreate   Action = "apikey_create"
	ActionAPIKeyRevoke   Action = "apikey_revoke"
	ActionInviteSend     Action = "invite_send"
	ActionInviteAccept   Action = "invite_accept"
	ActionGrantNamespace Action = "grant_namespace"
	ActionRevokeNamespace Action = "revoke_namespace"
)

// ── Audit ─────────────────────────────────────────────────────────────────────

type AuditEntryV2 struct {
	OrgID       string    `json:"org_id"`
	ActorID     string    `json:"actor_id"`
	ActorEmail  string    `json:"actor_email"`
	Action      Action    `json:"action"`
	Resource    string    `json:"resource"`
	ResourceID  string    `json:"resource_id,omitempty"`
	Details     string    `json:"details,omitempty"`
	IPAddress   string    `json:"ip_address,omitempty"`
	UserAgent   string    `json:"user_agent,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// ── Auth ──────────────────────────────────────────────────────────────────────

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      UserPublic `json:"user"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

type ResetPasswordRequest struct {
	Token    string `json:"token"`
	Password string `json:"new_password"`
}

// ── Users ─────────────────────────────────────────────────────────────────────

type UserPublic struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Role      Role      `json:"role"`
	OrgID     string    `json:"org_id"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateUserRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
	Role     Role   `json:"role"`
}

type UpdateUserRequest struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
	Role  Role   `json:"role,omitempty"`
}

// ── Teams ─────────────────────────────────────────────────────────────────────

type AddTeamMemberRequest struct {
	UserID string `json:"user_id"`
	Role   Role   `json:"role"`
}

// ── API Keys ──────────────────────────────────────────────────────────────────

type CreateAPIKeyRequest struct {
	Name        string    `json:"name"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	Scopes      []string  `json:"scopes,omitempty"`
}

type CreateAPIKeyResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Token     string    `json:"token"` // shown only on creation
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// ── Invitations ───────────────────────────────────────────────────────────────

type SendInvitationRequest struct {
	Email string `json:"email"`
	Role  Role   `json:"role"`
}

type AcceptInvitationRequest struct {
	Token          string `json:"token"`
	Name           string `json:"name"`
	Password       string `json:"password"`
	ExistingUserID string `json:"existing_user_id,omitempty"` // set if joining an existing account
}

// ── Namespace permissions ─────────────────────────────────────────────────────

type GrantNamespaceRequest struct {
	Namespace string `json:"namespace"`
	Access    Access `json:"access"`
}

// ── Re-export commonly used models from pkg/models for convenience ────────────
// These aliases let callers use dbmodels.Dashboard etc without a separate import.

type SLO struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	ServiceID   string    `json:"service_id"`
	MetricName  string    `json:"metric_name"`
	TargetPct   float64   `json:"target_pct"`
	WindowDays  int       `json:"window_days"`
	SLI         float64   `json:"sli,omitempty"`
	BudgetLeft  float64   `json:"budget_left,omitempty"`
	BurnRate1h  float64   `json:"burn_rate_1h,omitempty"`
	Status      string    `json:"status,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AlertRule struct {
	ID                   string            `json:"id"`
	OrgID                string            `json:"org_id"`
	Name                 string            `json:"name"`
	Namespace            string            `json:"namespace"`
	Expr                 string            `json:"expr"`
	ForDuration          string            `json:"for"`
	Severity             string            `json:"severity"`
	Annotations          map[string]string `json:"annotations,omitempty"`
	Labels               map[string]string `json:"labels,omitempty"`
	Enabled              bool              `json:"enabled"`
	NotificationChannels string            `json:"notification_channels"` // comma-sep: slack:url,pagerduty:key
	RunbookURL           string            `json:"runbook_url,omitempty"`
	Message              string            `json:"message,omitempty"`
	CreatedAt            time.Time         `json:"created_at"`
	UpdatedAt            time.Time         `json:"updated_at"`
}

type Dashboard struct {
	ID          string                 `json:"id"`
	OrgID       string                 `json:"org_id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	Widgets     []map[string]any       `json:"widgets,omitempty"`
	IsTemplate  bool                   `json:"is_template"`
	CreatedBy   string                 `json:"created_by,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

type IncidentComment struct {
	ID         string    `json:"id"`
	IncidentID string    `json:"incident_id"`
	AuthorID   string    `json:"author_id"`
	AuthorName string    `json:"author_name,omitempty"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}
