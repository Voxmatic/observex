// internal/db/store/stores.go
// All store implementations: CRUD for every entity.
package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"crypto/aes"
	"crypto/cipher"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	m "github.com/observex/platform/internal/db/models"
)


var (
	aesNewCipher = aes.NewCipher
	newGCM       = cipher.NewGCM
)

func newID() string { return uuid.New().String() }

func newToken(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ════════════════════════════════════════════════════════════════════════════
//  UserStore
// ════════════════════════════════════════════════════════════════════════════

type UserStore struct { db *DB; log *zap.Logger }
func NewUserStore(db *DB, log *zap.Logger) *UserStore { return &UserStore{db, log} }

func (s *UserStore) Create(ctx context.Context, req m.CreateUserRequest) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil { return nil, err }
	u := &User{ID: newID(), Email: req.Email, Name: req.Name, Role: req.Role,
		PasswordHash: string(hash), CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_, err = s.db.Pool.Exec(ctx,
		`INSERT INTO users(id,email,name,password_hash,role,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7)`,
		u.ID, u.Email, u.Name, u.PasswordHash, u.Role, u.CreatedAt, u.UpdatedAt)
	return u, err
}

func (s *UserStore) GetByID(ctx context.Context, id string) (*User, error) {
	u := &User{}
	err := s.db.Pool.QueryRow(ctx, `SELECT id,org_id,email,name,password_hash,role,created_at,updated_at FROM users WHERE id=$1`, id).
		Scan(&u.ID, &u.OrgID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func (s *UserStore) GetByEmail(ctx context.Context, email string) (*User, error) {
	u := &User{}
	err := s.db.Pool.QueryRow(ctx, `SELECT id,org_id,email,name,password_hash,role,created_at,updated_at FROM users WHERE email=$1`, email).
		Scan(&u.ID, &u.OrgID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func (s *UserStore) Authenticate(ctx context.Context, email, password string) (*User, error) {
	u, err := s.GetByEmail(ctx, email)
	if err != nil { return nil, fmt.Errorf("invalid credentials") }
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}
	return u, nil
}

func (s *UserStore) List(ctx context.Context, orgID string, limit, offset int) ([]*User, error) {
	rows, err := s.db.Pool.Query(ctx, `SELECT id,org_id,email,name,role,created_at,updated_at FROM users WHERE org_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`, orgID, limit, offset)
	if err != nil { return nil, err }
	defer rows.Close()
	var users []*User
	for rows.Next() {
		u := &User{}
		rows.Scan(&u.ID, &u.OrgID, &u.Email, &u.Name, &u.Role, &u.CreatedAt, &u.UpdatedAt)
		users = append(users, u)
	}
	return users, nil
}

func (s *UserStore) Update(ctx context.Context, id string, req m.UpdateUserRequest) (*User, error) {
	_, err := s.db.Pool.Exec(ctx, `UPDATE users SET name=COALESCE(NULLIF($1,''),name), email=COALESCE(NULLIF($2,''),email), role=COALESCE(NULLIF($3::text,'')::user_role,role), updated_at=NOW() WHERE id=$4`, req.Name, req.Email, req.Role, id)
	if err != nil { return nil, err }
	return s.GetByID(ctx, id)
}

func (s *UserStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.Pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, id)
	return err
}

func (s *UserStore) ChangePassword(ctx context.Context, id, oldPwd, newPwd string) error {
	u, err := s.GetByID(ctx, id); if err != nil { return err }
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(oldPwd)); err != nil { return fmt.Errorf("wrong password") }
	hash, _ := bcrypt.GenerateFromPassword([]byte(newPwd), bcrypt.DefaultCost)
	_, err = s.db.Pool.Exec(ctx, `UPDATE users SET password_hash=$1, updated_at=NOW() WHERE id=$2`, string(hash), id)
	return err
}

func (s *UserStore) SetPasswordDirect(ctx context.Context, id, newPwd string) error {
	hash, _ := bcrypt.GenerateFromPassword([]byte(newPwd), bcrypt.DefaultCost)
	_, err := s.db.Pool.Exec(ctx, `UPDATE users SET password_hash=$1, updated_at=NOW() WHERE id=$2`, string(hash), id)
	return err
}

// ════════════════════════════════════════════════════════════════════════════
//  SessionStore
// ════════════════════════════════════════════════════════════════════════════

type SessionStore struct { db *DB; log *zap.Logger }
func NewSessionStore(db *DB, log *zap.Logger) *SessionStore { return &SessionStore{db, log} }

func (s *SessionStore) Create(ctx context.Context, userID, orgID, ip, ua string, ttl time.Duration) (*Session, error) {
	sess := &Session{ID: newID(), UserID: userID, OrgID: orgID, Token: newToken(32),
		IPAddress: ip, UserAgent: ua, ExpiresAt: time.Now().Add(ttl), CreatedAt: time.Now()}
	_, err := s.db.Pool.Exec(ctx,
		`INSERT INTO sessions(id,user_id,org_id,token,ip_address,user_agent,expires_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		sess.ID, sess.UserID, sess.OrgID, sess.Token, sess.IPAddress, sess.UserAgent, sess.ExpiresAt, sess.CreatedAt)
	return sess, err
}

func (s *SessionStore) Revoke(ctx context.Context, token string) error {
	_, err := s.db.Pool.Exec(ctx, `DELETE FROM sessions WHERE token=$1`, token)
	return err
}

func (s *SessionStore) RevokeAllForUser(ctx context.Context, userID string) error {
	_, err := s.db.Pool.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, userID)
	return err
}

func (s *SessionStore) PurgeExpired(ctx context.Context) error {
	_, err := s.db.Pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < NOW()`)
	return err
}

// ════════════════════════════════════════════════════════════════════════════
//  OrgStore
// ════════════════════════════════════════════════════════════════════════════

type OrgStore struct { db *DB; log *zap.Logger }
func NewOrgStore(db *DB, log *zap.Logger) *OrgStore { return &OrgStore{db, log} }

func (s *OrgStore) GetByID(ctx context.Context, id string) (*Org, error) {
	o := &Org{}
	err := s.db.Pool.QueryRow(ctx, `SELECT id,name,plan,created_at FROM orgs WHERE id=$1`, id).
		Scan(&o.ID, &o.Name, &o.Plan, &o.CreatedAt)
	return o, err
}

func (s *OrgStore) GetByUserID(ctx context.Context, userID string) (*Org, error) {
	o := &Org{}
	err := s.db.Pool.QueryRow(ctx, `SELECT o.id,o.name,o.plan,o.created_at FROM orgs o JOIN users u ON u.org_id=o.id WHERE u.id=$1`, userID).
		Scan(&o.ID, &o.Name, &o.Plan, &o.CreatedAt)
	return o, err
}

func (s *OrgStore) UpdateSettings(ctx context.Context, id, settings string) error {
	_, err := s.db.Pool.Exec(ctx, `UPDATE orgs SET settings=$1 WHERE id=$2`, settings, id)
	return err
}

func (s *OrgStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.Pool.Exec(ctx, `DELETE FROM orgs WHERE id=$1`, id)
	return err
}

// ════════════════════════════════════════════════════════════════════════════
//  TeamStore
// ════════════════════════════════════════════════════════════════════════════

type TeamStore struct { db *DB; log *zap.Logger }
func NewTeamStore(db *DB, log *zap.Logger) *TeamStore { return &TeamStore{db, log} }

func (s *TeamStore) Create(ctx context.Context, orgID, name string) (*Team, error) {
	t := &Team{ID: newID(), OrgID: orgID, Name: name, CreatedAt: time.Now()}
	_, err := s.db.Pool.Exec(ctx, `INSERT INTO teams(id,org_id,name,created_at) VALUES($1,$2,$3,$4)`, t.ID, t.OrgID, t.Name, t.CreatedAt)
	return t, err
}

func (s *TeamStore) GetByID(ctx context.Context, id string) (*Team, error) {
	t := &Team{}
	err := s.db.Pool.QueryRow(ctx, `SELECT id,org_id,name,created_at FROM teams WHERE id=$1`, id).
		Scan(&t.ID, &t.OrgID, &t.Name, &t.CreatedAt)
	return t, err
}

func (s *TeamStore) List(ctx context.Context, orgID string) ([]*Team, error) {
	rows, err := s.db.Pool.Query(ctx, `SELECT id,org_id,name,created_at FROM teams WHERE org_id=$1 ORDER BY name`, orgID)
	if err != nil { return nil, err }
	defer rows.Close()
	var teams []*Team
	for rows.Next() { t := &Team{}; rows.Scan(&t.ID, &t.OrgID, &t.Name, &t.CreatedAt); teams = append(teams, t) }
	return teams, nil
}

func (s *TeamStore) Update(ctx context.Context, id, name string) error {
	_, err := s.db.Pool.Exec(ctx, `UPDATE teams SET name=$1 WHERE id=$2`, name, id)
	return err
}

func (s *TeamStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.Pool.Exec(ctx, `DELETE FROM teams WHERE id=$1`, id)
	return err
}

// ════════════════════════════════════════════════════════════════════════════
//  TeamMemberStore
// ════════════════════════════════════════════════════════════════════════════

type TeamMemberStore struct { db *DB; log *zap.Logger }
func NewTeamMemberStore(db *DB, log *zap.Logger) *TeamMemberStore { return &TeamMemberStore{db, log} }

func (s *TeamMemberStore) Add(ctx context.Context, teamID, userID string, role m.Role) (*TeamMember, error) {
	tm := &TeamMember{TeamID: teamID, UserID: userID, Role: role}
	_, err := s.db.Pool.Exec(ctx, `INSERT INTO team_members(team_id,user_id,role) VALUES($1,$2,$3) ON CONFLICT(team_id,user_id) DO UPDATE SET role=$3`, teamID, userID, role)
	return tm, err
}

func (s *TeamMemberStore) ListByTeam(ctx context.Context, teamID string) ([]*TeamMember, error) {
	rows, err := s.db.Pool.Query(ctx, `SELECT team_id,user_id,role FROM team_members WHERE team_id=$1`, teamID)
	if err != nil { return nil, err }
	defer rows.Close()
	var members []*TeamMember
	for rows.Next() { tm := &TeamMember{}; rows.Scan(&tm.TeamID, &tm.UserID, &tm.Role); members = append(members, tm) }
	return members, nil
}

func (s *TeamMemberStore) ListByUser(ctx context.Context, userID string) ([]*TeamMember, error) {
	rows, err := s.db.Pool.Query(ctx, `SELECT team_id,user_id,role FROM team_members WHERE user_id=$1`, userID)
	if err != nil { return nil, err }
	defer rows.Close()
	var members []*TeamMember
	for rows.Next() { tm := &TeamMember{}; rows.Scan(&tm.TeamID, &tm.UserID, &tm.Role); members = append(members, tm) }
	return members, nil
}

func (s *TeamMemberStore) UpdateRole(ctx context.Context, teamID, userID string, role m.Role) error {
	_, err := s.db.Pool.Exec(ctx, `UPDATE team_members SET role=$1 WHERE team_id=$2 AND user_id=$3`, role, teamID, userID)
	return err
}

func (s *TeamMemberStore) Remove(ctx context.Context, teamID, userID string) error {
	_, err := s.db.Pool.Exec(ctx, `DELETE FROM team_members WHERE team_id=$1 AND user_id=$2`, teamID, userID)
	return err
}

// ════════════════════════════════════════════════════════════════════════════
//  APIKeyStore
// ════════════════════════════════════════════════════════════════════════════

type APIKeyStore struct { db *DB; log *zap.Logger }
func NewAPIKeyStore(db *DB, log *zap.Logger) *APIKeyStore { return &APIKeyStore{db, log} }

func (s *APIKeyStore) Create(ctx context.Context, userID, orgID string, req m.CreateAPIKeyRequest) (*m.CreateAPIKeyResponse, error) {
	token := "oxk_" + newToken(32)
	hash, _ := bcrypt.GenerateFromPassword([]byte(token), bcrypt.DefaultCost)
	id := newID(); now := time.Now()
	_, err := s.db.Pool.Exec(ctx, `INSERT INTO api_keys(id,user_id,org_id,name,key_hash,expires_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)`,
		id, userID, orgID, req.Name, string(hash), req.ExpiresAt, now)
	return &m.CreateAPIKeyResponse{ID: id, Name: req.Name, Token: token, CreatedAt: now, ExpiresAt: req.ExpiresAt}, err
}

func (s *APIKeyStore) ListForUser(ctx context.Context, userID string) ([]*APIKey, error) {
	rows, err := s.db.Pool.Query(ctx, `SELECT id,user_id,org_id,name,expires_at,created_at FROM api_keys WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil { return nil, err }
	defer rows.Close()
	var keys []*APIKey
	for rows.Next() { k := &APIKey{}; rows.Scan(&k.ID, &k.UserID, &k.OrgID, &k.Name, &k.ExpiresAt, &k.CreatedAt); keys = append(keys, k) }
	return keys, nil
}

func (s *APIKeyStore) Revoke(ctx context.Context, id, userID string) error {
	_, err := s.db.Pool.Exec(ctx, `DELETE FROM api_keys WHERE id=$1 AND user_id=$2`, id, userID)
	return err
}

// ════════════════════════════════════════════════════════════════════════════
//  DashboardStore
// ════════════════════════════════════════════════════════════════════════════

type DashboardStore struct { db *DB; log *zap.Logger }
func NewDashboardStore(db *DB, log *zap.Logger) *DashboardStore { return &DashboardStore{db, log} }

func (s *DashboardStore) Create(ctx context.Context, orgID string, d m.Dashboard) (*m.Dashboard, error) {
	d.ID = newID(); d.OrgID = orgID; d.CreatedAt = time.Now(); d.UpdatedAt = time.Now()
	_, err := s.db.Pool.Exec(ctx, `INSERT INTO dashboards(id,org_id,name,description,is_template,created_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		d.ID, d.OrgID, d.Name, d.Description, d.IsTemplate, d.CreatedBy, d.CreatedAt, d.UpdatedAt)
	return &d, err
}

func (s *DashboardStore) GetByID(ctx context.Context, id, orgID string) (*m.Dashboard, error) {
	d := &m.Dashboard{}
	err := s.db.Pool.QueryRow(ctx, `SELECT id,org_id,name,description,is_template,created_by,created_at,updated_at FROM dashboards WHERE id=$1 AND org_id=$2`, id, orgID).
		Scan(&d.ID, &d.OrgID, &d.Name, &d.Description, &d.IsTemplate, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

func (s *DashboardStore) List(ctx context.Context, orgID string) ([]*m.Dashboard, error) {
	rows, err := s.db.Pool.Query(ctx, `SELECT id,org_id,name,description,is_template,created_by,created_at,updated_at FROM dashboards WHERE org_id=$1 ORDER BY updated_at DESC`, orgID)
	if err != nil { return nil, err }
	defer rows.Close()
	var ds []*m.Dashboard
	for rows.Next() { d := &m.Dashboard{}; rows.Scan(&d.ID, &d.OrgID, &d.Name, &d.Description, &d.IsTemplate, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt); ds = append(ds, d) }
	return ds, nil
}

func (s *DashboardStore) Update(ctx context.Context, id, orgID string, d m.Dashboard) (*m.Dashboard, error) {
	_, err := s.db.Pool.Exec(ctx, `UPDATE dashboards SET name=$1,description=$2,updated_at=NOW() WHERE id=$3 AND org_id=$4`, d.Name, d.Description, id, orgID)
	if err != nil { return nil, err }
	return s.GetByID(ctx, id, orgID)
}

func (s *DashboardStore) Delete(ctx context.Context, id, orgID string) error {
	_, err := s.db.Pool.Exec(ctx, `DELETE FROM dashboards WHERE id=$1 AND org_id=$2`, id, orgID)
	return err
}

func (s *DashboardStore) Clone(ctx context.Context, id, orgID, newName, createdBy string) (*m.Dashboard, error) {
	orig, err := s.GetByID(ctx, id, orgID); if err != nil { return nil, err }
	clone := *orig; clone.ID = newID(); clone.Name = newName; clone.IsTemplate = false; clone.CreatedBy = createdBy; clone.CreatedAt = time.Now(); clone.UpdatedAt = time.Now()
	_, err = s.db.Pool.Exec(ctx, `INSERT INTO dashboards(id,org_id,name,description,is_template,created_by,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		clone.ID, clone.OrgID, clone.Name, clone.Description, clone.IsTemplate, clone.CreatedBy, clone.CreatedAt, clone.UpdatedAt)
	return &clone, err
}

// ════════════════════════════════════════════════════════════════════════════
//  SLOStore
// ════════════════════════════════════════════════════════════════════════════

type SLOStore struct { db *DB; log *zap.Logger }
func NewSLOStore(db *DB, log *zap.Logger) *SLOStore { return &SLOStore{db, log} }

func (s *SLOStore) Create(ctx context.Context, orgID string, slo m.SLO) (*m.SLO, error) {
	slo.ID = newID(); slo.OrgID = orgID; slo.CreatedAt = time.Now(); slo.UpdatedAt = time.Now()
	_, err := s.db.Pool.Exec(ctx, `INSERT INTO slos(id,org_id,name,service_id,metric_name,target_pct,window_days,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		slo.ID, slo.OrgID, slo.Name, slo.ServiceID, slo.MetricName, slo.TargetPct, slo.WindowDays, slo.CreatedAt, slo.UpdatedAt)
	return &slo, err
}

func (s *SLOStore) GetByID(ctx context.Context, id, orgID string) (*m.SLO, error) {
	slo := &m.SLO{}
	err := s.db.Pool.QueryRow(ctx, `SELECT id,org_id,name,service_id,metric_name,target_pct,window_days,created_at,updated_at FROM slos WHERE id=$1 AND org_id=$2`, id, orgID).
		Scan(&slo.ID, &slo.OrgID, &slo.Name, &slo.ServiceID, &slo.MetricName, &slo.TargetPct, &slo.WindowDays, &slo.CreatedAt, &slo.UpdatedAt)
	return slo, err
}

func (s *SLOStore) List(ctx context.Context, orgID string, limit, offset int) ([]*m.SLO, error) {
	rows, err := s.db.Pool.Query(ctx, `SELECT id,org_id,name,service_id,metric_name,target_pct,window_days,created_at,updated_at FROM slos WHERE org_id=$1 ORDER BY name LIMIT $2 OFFSET $3`, orgID, limit, offset)
	if err != nil { return nil, err }
	defer rows.Close()
	var slos []*m.SLO
	for rows.Next() { slo := &m.SLO{}; rows.Scan(&slo.ID, &slo.OrgID, &slo.Name, &slo.ServiceID, &slo.MetricName, &slo.TargetPct, &slo.WindowDays, &slo.CreatedAt, &slo.UpdatedAt); slos = append(slos, slo) }
	return slos, nil
}

func (s *SLOStore) Update(ctx context.Context, id, orgID string, slo m.SLO) (*m.SLO, error) {
	_, err := s.db.Pool.Exec(ctx, `UPDATE slos SET name=$1,target_pct=$2,window_days=$3,updated_at=NOW() WHERE id=$4 AND org_id=$5`, slo.Name, slo.TargetPct, slo.WindowDays, id, orgID)
	if err != nil { return nil, err }
	return s.GetByID(ctx, id, orgID)
}

func (s *SLOStore) Delete(ctx context.Context, id, orgID string) error {
	_, err := s.db.Pool.Exec(ctx, `DELETE FROM slos WHERE id=$1 AND org_id=$2`, id, orgID)
	return err
}

// ════════════════════════════════════════════════════════════════════════════
//  AlertRuleStore
// ════════════════════════════════════════════════════════════════════════════

type AlertRuleStore struct { db *DB; log *zap.Logger }
func NewAlertRuleStore(db *DB, log *zap.Logger) *AlertRuleStore { return &AlertRuleStore{db, log} }

func (s *AlertRuleStore) Create(ctx context.Context, orgID string, rule m.AlertRule) (*m.AlertRule, error) {
	rule.ID = newID(); rule.OrgID = orgID; rule.CreatedAt = time.Now(); rule.UpdatedAt = time.Now()
	_, err := s.db.Pool.Exec(ctx, `INSERT INTO alert_rules(id,org_id,name,namespace,expr,for_duration,severity,enabled,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		rule.ID, rule.OrgID, rule.Name, rule.Namespace, rule.Expr, rule.ForDuration, rule.Severity, rule.Enabled, rule.CreatedAt, rule.UpdatedAt)
	return &rule, err
}

func (s *AlertRuleStore) GetByID(ctx context.Context, id, orgID string) (*m.AlertRule, error) {
	r := &m.AlertRule{}
	err := s.db.Pool.QueryRow(ctx, `SELECT id,org_id,name,namespace,expr,for_duration,severity,enabled,created_at,updated_at FROM alert_rules WHERE id=$1 AND org_id=$2`, id, orgID).
		Scan(&r.ID, &r.OrgID, &r.Name, &r.Namespace, &r.Expr, &r.ForDuration, &r.Severity, &r.Enabled, &r.CreatedAt, &r.UpdatedAt)
	return r, err
}

func (s *AlertRuleStore) List(ctx context.Context, orgID, namespace string, limit, offset int) ([]*m.AlertRule, error) {
	q := `SELECT id,org_id,name,namespace,expr,for_duration,severity,enabled,created_at,updated_at FROM alert_rules WHERE org_id=$1`
	args := []any{orgID}
	if namespace != "" { q += " AND namespace=$2"; args = append(args, namespace) }
	q += fmt.Sprintf(" ORDER BY name LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, limit, offset)
	rows, err := s.db.Pool.Query(ctx, q, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	var rules []*m.AlertRule
	for rows.Next() { r := &m.AlertRule{}; rows.Scan(&r.ID, &r.OrgID, &r.Name, &r.Namespace, &r.Expr, &r.ForDuration, &r.Severity, &r.Enabled, &r.CreatedAt, &r.UpdatedAt); rules = append(rules, r) }
	return rules, nil
}

func (s *AlertRuleStore) Update(ctx context.Context, id, orgID string, rule m.AlertRule) (*m.AlertRule, error) {
	_, err := s.db.Pool.Exec(ctx, `UPDATE alert_rules SET name=$1,expr=$2,for_duration=$3,severity=$4,enabled=$5,updated_at=NOW() WHERE id=$6 AND org_id=$7`, rule.Name, rule.Expr, rule.ForDuration, rule.Severity, rule.Enabled, id, orgID)
	if err != nil { return nil, err }
	return s.GetByID(ctx, id, orgID)
}

func (s *AlertRuleStore) Delete(ctx context.Context, id, orgID string) error {
	_, err := s.db.Pool.Exec(ctx, `DELETE FROM alert_rules WHERE id=$1 AND org_id=$2`, id, orgID)
	return err
}

func (s *AlertRuleStore) SetSilenced(ctx context.Context, id, orgID string, silenced bool) error {
	_, err := s.db.Pool.Exec(ctx, `UPDATE alert_rules SET silenced=$1,updated_at=NOW() WHERE id=$2 AND org_id=$3`, silenced, id, orgID)
	return err
}

// ════════════════════════════════════════════════════════════════════════════
//  NamespacePermStore
// ════════════════════════════════════════════════════════════════════════════

type NamespacePermStore struct { db *DB; log *zap.Logger }
func NewNamespacePermStore(db *DB, log *zap.Logger) *NamespacePermStore { return &NamespacePermStore{db, log} }

func (s *NamespacePermStore) Grant(ctx context.Context, orgID, teamID, namespace string, access m.Access) error {
	_, err := s.db.Pool.Exec(ctx, `INSERT INTO namespace_perms(team_id,org_id,namespace,access) VALUES($1,$2,$3,$4) ON CONFLICT(team_id,namespace) DO UPDATE SET access=$4`, teamID, orgID, namespace, access)
	return err
}

func (s *NamespacePermStore) Revoke(ctx context.Context, teamID, namespace string) error {
	_, err := s.db.Pool.Exec(ctx, `DELETE FROM namespace_perms WHERE team_id=$1 AND namespace=$2`, teamID, namespace)
	return err
}

func (s *NamespacePermStore) ListByOrg(ctx context.Context, orgID string) ([]*NamespacePerm, error) {
	rows, err := s.db.Pool.Query(ctx, `SELECT team_id,org_id,namespace,access FROM namespace_perms WHERE org_id=$1`, orgID)
	if err != nil { return nil, err }
	defer rows.Close()
	var perms []*NamespacePerm
	for rows.Next() { p := &NamespacePerm{}; rows.Scan(&p.TeamID, &p.OrgID, &p.Namespace, &p.Access); perms = append(perms, p) }
	return perms, nil
}

func (s *NamespacePermStore) ListForTeams(ctx context.Context, teamIDs []string) ([]*NamespacePerm, error) {
	if len(teamIDs) == 0 { return nil, nil }
	rows, err := s.db.Pool.Query(ctx, `SELECT team_id,org_id,namespace,access FROM namespace_perms WHERE team_id=ANY($1)`, teamIDs)
	if err != nil { return nil, err }
	defer rows.Close()
	var perms []*NamespacePerm
	for rows.Next() { p := &NamespacePerm{}; rows.Scan(&p.TeamID, &p.OrgID, &p.Namespace, &p.Access); perms = append(perms, p) }
	return perms, nil
}

// ════════════════════════════════════════════════════════════════════════════
//  InvitationStore
// ════════════════════════════════════════════════════════════════════════════

type InvitationStore struct { db *DB; log *zap.Logger }
func NewInvitationStore(db *DB, log *zap.Logger) *InvitationStore { return &InvitationStore{db, log} }

func (s *InvitationStore) Create(ctx context.Context, orgID string, req m.SendInvitationRequest, teamID *string) (*Invitation, error) {
	inv := &Invitation{ID: newID(), OrgID: orgID, Email: req.Email, Role: req.Role, TeamID: teamID,
		Token: newToken(32), ExpiresAt: time.Now().Add(72 * time.Hour), CreatedAt: time.Now()}
	_, err := s.db.Pool.Exec(ctx, `INSERT INTO invitations(id,org_id,email,role,team_id,token,expires_at,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		inv.ID, inv.OrgID, inv.Email, inv.Role, inv.TeamID, inv.Token, inv.ExpiresAt, inv.CreatedAt)
	return inv, err
}

func (s *InvitationStore) ValidateToken(ctx context.Context, token string) (*Invitation, error) {
	inv := &Invitation{}
	err := s.db.Pool.QueryRow(ctx, `SELECT id,org_id,email,role,team_id,token,expires_at,created_at FROM invitations WHERE token=$1 AND accepted_at IS NULL AND expires_at>NOW()`, token).
		Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.Role, &inv.TeamID, &inv.Token, &inv.ExpiresAt, &inv.CreatedAt)
	return inv, err
}

func (s *InvitationStore) Accept(ctx context.Context, token string) error {
	_, err := s.db.Pool.Exec(ctx, `UPDATE invitations SET accepted_at=NOW() WHERE token=$1`, token)
	return err
}

func (s *InvitationStore) ListByOrg(ctx context.Context, orgID string) ([]*Invitation, error) {
	rows, err := s.db.Pool.Query(ctx, `SELECT id,org_id,email,role,team_id,token,expires_at,accepted_at,created_at FROM invitations WHERE org_id=$1 ORDER BY created_at DESC`, orgID)
	if err != nil { return nil, err }
	defer rows.Close()
	var invs []*Invitation
	for rows.Next() { inv := &Invitation{}; rows.Scan(&inv.ID, &inv.OrgID, &inv.Email, &inv.Role, &inv.TeamID, &inv.Token, &inv.ExpiresAt, &inv.AcceptedAt, &inv.CreatedAt); invs = append(invs, inv) }
	return invs, nil
}

func (s *InvitationStore) DeleteExpired(ctx context.Context) error {
	_, err := s.db.Pool.Exec(ctx, `DELETE FROM invitations WHERE expires_at<NOW() AND accepted_at IS NULL`)
	return err
}

// ════════════════════════════════════════════════════════════════════════════
//  PasswordResetStore
// ════════════════════════════════════════════════════════════════════════════

type PasswordResetStore struct { db *DB; log *zap.Logger }
func NewPasswordResetStore(db *DB, log *zap.Logger) *PasswordResetStore { return &PasswordResetStore{db, log} }

func (s *PasswordResetStore) Create(ctx context.Context, userID string) (*PasswordReset, error) {
	pr := &PasswordReset{ID: newID(), UserID: userID, Token: newToken(32), ExpiresAt: time.Now().Add(1 * time.Hour)}
	_, err := s.db.Pool.Exec(ctx, `INSERT INTO password_resets(id,user_id,token,expires_at) VALUES($1,$2,$3,$4)`, pr.ID, pr.UserID, pr.Token, pr.ExpiresAt)
	return pr, err
}

func (s *PasswordResetStore) Validate(ctx context.Context, token string) (*PasswordReset, error) {
	pr := &PasswordReset{}
	err := s.db.Pool.QueryRow(ctx, `SELECT id,user_id,token,expires_at FROM password_resets WHERE token=$1 AND used_at IS NULL AND expires_at>NOW()`, token).
		Scan(&pr.ID, &pr.UserID, &pr.Token, &pr.ExpiresAt)
	return pr, err
}

func (s *PasswordResetStore) Use(ctx context.Context, token string) error {
	_, err := s.db.Pool.Exec(ctx, `UPDATE password_resets SET used_at=NOW() WHERE token=$1`, token)
	return err
}

// ════════════════════════════════════════════════════════════════════════════
//  IncidentCommentStore
// ════════════════════════════════════════════════════════════════════════════

type IncidentCommentStore struct { db *DB; log *zap.Logger }
func NewIncidentCommentStore(db *DB, log *zap.Logger) *IncidentCommentStore { return &IncidentCommentStore{db, log} }

func (s *IncidentCommentStore) Create(ctx context.Context, c m.IncidentComment) (*m.IncidentComment, error) {
	c.ID = newID(); c.CreatedAt = time.Now()
	_, err := s.db.Pool.Exec(ctx, `INSERT INTO incident_comments(id,incident_id,author_id,body,created_at) VALUES($1,$2,$3,$4,$5)`, c.ID, c.IncidentID, c.AuthorID, c.Body, c.CreatedAt)
	return &c, err
}

func (s *IncidentCommentStore) List(ctx context.Context, incidentID string) ([]*m.IncidentComment, error) {
	rows, err := s.db.Pool.Query(ctx, `SELECT id,incident_id,author_id,body,created_at FROM incident_comments WHERE incident_id=$1 ORDER BY created_at`, incidentID)
	if err != nil { return nil, err }
	defer rows.Close()
	var cs []*m.IncidentComment
	for rows.Next() { c := &m.IncidentComment{}; rows.Scan(&c.ID, &c.IncidentID, &c.AuthorID, &c.Body, &c.CreatedAt); cs = append(cs, c) }
	return cs, nil
}

func (s *IncidentCommentStore) Update(ctx context.Context, id, authorID, body string) error {
	_, err := s.db.Pool.Exec(ctx, `UPDATE incident_comments SET body=$1 WHERE id=$2 AND author_id=$3`, body, id, authorID)
	return err
}

// ErrCommentNotFound is returned by IncidentCommentStore.Delete when no comment
// matched both the ID and the author.
var ErrCommentNotFound = errors.New("comment not found")

// Delete removes a comment written by authorID. It returns ErrCommentNotFound
// when nothing was deleted (unknown ID, or a comment by another author).
func (s *IncidentCommentStore) Delete(ctx context.Context, id, authorID string) error {
	tag, err := s.db.Pool.Exec(ctx, `DELETE FROM incident_comments WHERE id=$1 AND author_id=$2`, id, authorID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrCommentNotFound
	}
	return nil
}

// ════════════════════════════════════════════════════════════════════════════
//  AuditStoreV2
// ════════════════════════════════════════════════════════════════════════════

type AuditStoreV2 struct { db *DB }
func NewAuditStoreV2(db *DB) *AuditStoreV2 { return &AuditStoreV2{db} }

func (s *AuditStoreV2) Log(ctx context.Context, entry m.AuditEntryV2) {
	entry.CreatedAt = time.Now()
	s.db.Pool.Exec(ctx, `INSERT INTO audit_log(org_id,actor_id,actor_email,action,resource,resource_id,details,ip_address,user_agent,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		entry.OrgID, entry.ActorID, entry.ActorEmail, entry.Action, entry.Resource, entry.ResourceID, entry.Details, entry.IPAddress, entry.UserAgent, entry.CreatedAt)
}

func (s *AuditStoreV2) List(ctx context.Context, orgID string, limit, offset int) ([]m.AuditEntryV2, error) {
	rows, err := s.db.Pool.Query(ctx, `SELECT org_id,actor_id,actor_email,action,resource,resource_id,details,ip_address,user_agent,created_at FROM audit_log WHERE org_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`, orgID, limit, offset)
	if err != nil { return nil, err }
	defer rows.Close()
	var entries []m.AuditEntryV2
	for rows.Next() { e := m.AuditEntryV2{}; rows.Scan(&e.OrgID, &e.ActorID, &e.ActorEmail, &e.Action, &e.Resource, &e.ResourceID, &e.Details, &e.IPAddress, &e.UserAgent, &e.CreatedAt); entries = append(entries, e) }
	return entries, nil
}

// ════════════════════════════════════════════════════════════════════════════
//  AuthContextStore  — builds full AuthContext for a user from JWT claims
// ════════════════════════════════════════════════════════════════════════════

type AuthContextStore struct {
	db      *DB
	nsPerms *NamespacePermStore
	members *TeamMemberStore
	log     *zap.Logger
}

func NewAuthContextStore(db *DB, nsPerms *NamespacePermStore, members *TeamMemberStore, log *zap.Logger) *AuthContextStore {
	return &AuthContextStore{db, nsPerms, members, log}
}

func (s *AuthContextStore) Build(ctx context.Context, userID string) (*AuthContext, error) {
	u, err := (&UserStore{db: s.db}).GetByID(ctx, userID)
	if err != nil { return nil, err }

	// Get team memberships
	teamMemberships, _ := s.members.ListByUser(ctx, userID)
	teamIDs := make([]string, 0, len(teamMemberships))
	for _, tm := range teamMemberships { teamIDs = append(teamIDs, tm.TeamID) }

	// Get namespace permissions for all teams
	perms, _ := s.nsPerms.ListForTeams(ctx, teamIDs)
	nsAccess := make(map[string]m.Access)
	for _, p := range perms {
		if existing, ok := nsAccess[p.Namespace]; !ok || (p.Access == m.AccessWrite && existing == m.AccessRead) {
			nsAccess[p.Namespace] = p.Access
		}
	}

	isAdmin := u.Role == m.RoleAdmin
	return &AuthContext{
		UserID: u.ID, UserEmail: u.Email, Email: u.Email, Name: u.Name,
		OrgID: u.OrgID, Role: u.Role, EffectiveRole: u.Role,
		TeamIDs: teamIDs, NamespaceAccess: nsAccess, IsOrgAdmin: isAdmin,
	}, nil
}

// ════════════════════════════════════════════════════════════════════════════
//  OnCallStore  (oncall_schedules + oncall_rotations)
// ════════════════════════════════════════════════════════════════════════════

type OnCallSchedule struct {
	ID          string    `json:"id" db:"id"`
	OrgID       string    `json:"org_id" db:"org_id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	Timezone    string    `json:"timezone" db:"timezone"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type OnCallRotation struct {
	ID         string    `json:"id" db:"id"`
	ScheduleID string    `json:"schedule_id" db:"schedule_id"`
	UserID     string    `json:"user_id" db:"user_id"`
	StartTime  time.Time `json:"start_time" db:"start_time"`
	EndTime    time.Time `json:"end_time" db:"end_time"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
}

type EscalationPolicy struct {
	ID          string    `json:"id" db:"id"`
	OrgID       string    `json:"org_id" db:"org_id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	Steps       string    `json:"steps" db:"steps"` // JSONB stored as string
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

type OnCallStore struct{ db *DB; log *zap.Logger }
func NewOnCallStore(db *DB, log *zap.Logger) *OnCallStore { return &OnCallStore{db, log} }

func (s *OnCallStore) CreateSchedule(ctx context.Context, orgID, name, description, timezone string) (*OnCallSchedule, error) {
	sc := &OnCallSchedule{
		ID: newID(), OrgID: orgID, Name: name,
		Description: description, Timezone: timezone,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	_, err := s.db.Pool.Exec(ctx,
		`INSERT INTO oncall_schedules(id,org_id,name,description,timezone,created_at,updated_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7)`,
		sc.ID, sc.OrgID, sc.Name, sc.Description, sc.Timezone, sc.CreatedAt, sc.UpdatedAt)
	return sc, err
}

func (s *OnCallStore) ListSchedules(ctx context.Context, orgID string) ([]*OnCallSchedule, error) {
	rows, err := s.db.Pool.Query(ctx,
		`SELECT id,org_id,name,description,timezone,created_at,updated_at
		 FROM oncall_schedules WHERE org_id=$1 ORDER BY name`, orgID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*OnCallSchedule
	for rows.Next() {
		sc := &OnCallSchedule{}
		if err := rows.Scan(&sc.ID,&sc.OrgID,&sc.Name,&sc.Description,&sc.Timezone,&sc.CreatedAt,&sc.UpdatedAt); err != nil { return nil, err }
		out = append(out, sc)
	}
	return out, rows.Err()
}

func (s *OnCallStore) GetSchedule(ctx context.Context, id, orgID string) (*OnCallSchedule, error) {
	sc := &OnCallSchedule{}
	err := s.db.Pool.QueryRow(ctx,
		`SELECT id,org_id,name,description,timezone,created_at,updated_at
		 FROM oncall_schedules WHERE id=$1 AND org_id=$2`, id, orgID).
		Scan(&sc.ID,&sc.OrgID,&sc.Name,&sc.Description,&sc.Timezone,&sc.CreatedAt,&sc.UpdatedAt)
	return sc, err
}

func (s *OnCallStore) DeleteSchedule(ctx context.Context, id, orgID string) error {
	_, err := s.db.Pool.Exec(ctx, `DELETE FROM oncall_schedules WHERE id=$1 AND org_id=$2`, id, orgID)
	return err
}

func (s *OnCallStore) ListRotations(ctx context.Context, scheduleID string) ([]*OnCallRotation, error) {
	rows, err := s.db.Pool.Query(ctx,
		`SELECT id,schedule_id,user_id,start_time,end_time,created_at
		 FROM oncall_rotations WHERE schedule_id=$1 ORDER BY start_time`, scheduleID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*OnCallRotation
	for rows.Next() {
		r := &OnCallRotation{}
		if err := rows.Scan(&r.ID,&r.ScheduleID,&r.UserID,&r.StartTime,&r.EndTime,&r.CreatedAt); err != nil { return nil, err }
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *OnCallStore) CreateRotation(ctx context.Context, scheduleID, userID string, start, end time.Time) (*OnCallRotation, error) {
	r := &OnCallRotation{ID: newID(), ScheduleID: scheduleID, UserID: userID, StartTime: start, EndTime: end, CreatedAt: time.Now()}
	_, err := s.db.Pool.Exec(ctx,
		`INSERT INTO oncall_rotations(id,schedule_id,user_id,start_time,end_time,created_at) VALUES($1,$2,$3,$4,$5,$6)`,
		r.ID, r.ScheduleID, r.UserID, r.StartTime, r.EndTime, r.CreatedAt)
	return r, err
}

func (s *OnCallStore) WhoIsOnCall(ctx context.Context, scheduleID string, at time.Time) (*OnCallRotation, error) {
	r := &OnCallRotation{}
	err := s.db.Pool.QueryRow(ctx,
		`SELECT id,schedule_id,user_id,start_time,end_time,created_at
		 FROM oncall_rotations WHERE schedule_id=$1 AND start_time<=$2 AND end_time>$2
		 ORDER BY start_time DESC LIMIT 1`, scheduleID, at).
		Scan(&r.ID,&r.ScheduleID,&r.UserID,&r.StartTime,&r.EndTime,&r.CreatedAt)
	return r, err
}

// EscalationPolicy CRUD
func (s *OnCallStore) CreatePolicy(ctx context.Context, orgID, name, description, stepsJSON string) (*EscalationPolicy, error) {
	p := &EscalationPolicy{ID: newID(), OrgID: orgID, Name: name, Description: description, Steps: stepsJSON, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	_, err := s.db.Pool.Exec(ctx,
		`INSERT INTO escalation_policies(id,org_id,name,description,steps,created_at,updated_at) VALUES($1,$2,$3,$4,$5::jsonb,$6,$7)`,
		p.ID, p.OrgID, p.Name, p.Description, p.Steps, p.CreatedAt, p.UpdatedAt)
	return p, err
}

func (s *OnCallStore) ListPolicies(ctx context.Context, orgID string) ([]*EscalationPolicy, error) {
	rows, err := s.db.Pool.Query(ctx,
		`SELECT id,org_id,name,description,steps::text,created_at,updated_at FROM escalation_policies WHERE org_id=$1 ORDER BY name`, orgID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*EscalationPolicy
	for rows.Next() {
		p := &EscalationPolicy{}
		if err := rows.Scan(&p.ID,&p.OrgID,&p.Name,&p.Description,&p.Steps,&p.CreatedAt,&p.UpdatedAt); err != nil { return nil, err }
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *OnCallStore) DeletePolicy(ctx context.Context, id, orgID string) error {
	_, err := s.db.Pool.Exec(ctx, `DELETE FROM escalation_policies WHERE id=$1 AND org_id=$2`, id, orgID)
	return err
}

// ════════════════════════════════════════════════════════════════════════════
//  SyntheticStore
// ════════════════════════════════════════════════════════════════════════════

type SyntheticCheck struct {
	ID                 string    `json:"id"`
	OrgID              string    `json:"org_id"`
	Name               string    `json:"name"`
	Type               string    `json:"type"`
	Target             string    `json:"target"`
	IntervalSec        int       `json:"interval_sec"`
	TimeoutSec         int       `json:"timeout_sec"`
	Locations          []string  `json:"locations"`
	Enabled            bool      `json:"enabled"`
	ExpectStatus       int       `json:"expect_status"`
	ExpectBodyContains string    `json:"expect_body_contains"`
	Headers            string    `json:"headers"`
	Namespace          string    `json:"namespace"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type SyntheticStore struct{ db *DB; log *zap.Logger }
func NewSyntheticStore(db *DB, log *zap.Logger) *SyntheticStore { return &SyntheticStore{db, log} }

func (s *SyntheticStore) Create(ctx context.Context, orgID string, c SyntheticCheck) (*SyntheticCheck, error) {
	c.ID = newID(); c.OrgID = orgID; c.CreatedAt = time.Now(); c.UpdatedAt = time.Now()
	if c.IntervalSec == 0 { c.IntervalSec = 60 }
	if c.TimeoutSec == 0  { c.TimeoutSec = 10  }
	if c.ExpectStatus == 0 { c.ExpectStatus = 200 }
	if len(c.Locations) == 0 { c.Locations = []string{"local"} }
	if c.Headers == "" { c.Headers = "{}" }
	if c.Namespace == "" { c.Namespace = "default" }
	_, err := s.db.Pool.Exec(ctx,
		`INSERT INTO synthetic_checks(id,org_id,name,type,target,interval_sec,timeout_sec,locations,enabled,
		  expect_status,expect_body_contains,headers,namespace,created_at,updated_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,$13,$14,$15)`,
		c.ID,c.OrgID,c.Name,c.Type,c.Target,c.IntervalSec,c.TimeoutSec,c.Locations,c.Enabled,
		c.ExpectStatus,c.ExpectBodyContains,c.Headers,c.Namespace,c.CreatedAt,c.UpdatedAt)
	return &c, err
}

func (s *SyntheticStore) List(ctx context.Context, orgID, namespace string) ([]*SyntheticCheck, error) {
	q := `SELECT id,org_id,name,type,target,interval_sec,timeout_sec,locations,enabled,expect_status,expect_body_contains,headers::text,namespace,created_at,updated_at
		  FROM synthetic_checks WHERE org_id=$1`
	args := []any{orgID}
	if namespace != "" && namespace != "all" { q += ` AND namespace=$2`; args = append(args, namespace) }
	q += ` ORDER BY name`
	rows, err := s.db.Pool.Query(ctx, q, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*SyntheticCheck
	for rows.Next() {
		c := &SyntheticCheck{}
		if err := rows.Scan(&c.ID,&c.OrgID,&c.Name,&c.Type,&c.Target,&c.IntervalSec,&c.TimeoutSec,&c.Locations,&c.Enabled,&c.ExpectStatus,&c.ExpectBodyContains,&c.Headers,&c.Namespace,&c.CreatedAt,&c.UpdatedAt); err != nil { return nil, err }
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *SyntheticStore) GetByID(ctx context.Context, id, orgID string) (*SyntheticCheck, error) {
	c := &SyntheticCheck{}
	err := s.db.Pool.QueryRow(ctx,
		`SELECT id,org_id,name,type,target,interval_sec,timeout_sec,locations,enabled,expect_status,expect_body_contains,headers::text,namespace,created_at,updated_at
		 FROM synthetic_checks WHERE id=$1 AND org_id=$2`, id, orgID).
		Scan(&c.ID,&c.OrgID,&c.Name,&c.Type,&c.Target,&c.IntervalSec,&c.TimeoutSec,&c.Locations,&c.Enabled,&c.ExpectStatus,&c.ExpectBodyContains,&c.Headers,&c.Namespace,&c.CreatedAt,&c.UpdatedAt)
	return c, err
}

func (s *SyntheticStore) SetEnabled(ctx context.Context, id, orgID string, enabled bool) error {
	_, err := s.db.Pool.Exec(ctx, `UPDATE synthetic_checks SET enabled=$1,updated_at=NOW() WHERE id=$2 AND org_id=$3`, enabled, id, orgID)
	return err
}

func (s *SyntheticStore) Delete(ctx context.Context, id, orgID string) error {
	_, err := s.db.Pool.Exec(ctx, `DELETE FROM synthetic_checks WHERE id=$1 AND org_id=$2`, id, orgID)
	return err
}

// ════════════════════════════════════════════════════════════════════════════
//  IntegrationStore
// ════════════════════════════════════════════════════════════════════════════

type Integration struct {
	ID           string     `json:"id"`
	OrgID        string     `json:"org_id"`
	Type         string     `json:"type"`
	IsActive     bool       `json:"is_active"`
	LastTestedAt *time.Time `json:"last_tested_at,omitempty"`
	LastTestOK   *bool      `json:"last_test_ok,omitempty"`
	LastTestMsg  string     `json:"last_test_msg"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type IntegrationStore struct{ db *DB; log *zap.Logger; encKey []byte }
func NewIntegrationStore(db *DB, log *zap.Logger, encKey []byte) *IntegrationStore {
	return &IntegrationStore{db, log, encKey}
}

func (s *IntegrationStore) Upsert(ctx context.Context, orgID, intType string, configJSON []byte) (*Integration, error) {
	// Encrypt config before storage
	encrypted, iv, err := encryptAESGCM(s.encKey, configJSON)
	if err != nil { return nil, fmt.Errorf("encrypt config: %w", err) }
	id := newID()
	now := time.Now()
	_, err = s.db.Pool.Exec(ctx,
		`INSERT INTO integrations(id,org_id,type,config_enc,config_iv,is_active,created_at,updated_at)
		 VALUES($1,$2,$3,$4,$5,true,$6,$7)
		 ON CONFLICT(org_id,type) DO UPDATE SET config_enc=$4,config_iv=$5,is_active=true,updated_at=$7`,
		id, orgID, intType, encrypted, iv, now, now)
	return &Integration{ID: id, OrgID: orgID, Type: intType, IsActive: true, CreatedAt: now, UpdatedAt: now}, err
}

func (s *IntegrationStore) List(ctx context.Context, orgID string) ([]*Integration, error) {
	rows, err := s.db.Pool.Query(ctx,
		`SELECT id,org_id,type,is_active,last_tested_at,last_test_ok,last_test_msg,created_at,updated_at
		 FROM integrations WHERE org_id=$1 ORDER BY type`, orgID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*Integration
	for rows.Next() {
		ig := &Integration{}
		if err := rows.Scan(&ig.ID,&ig.OrgID,&ig.Type,&ig.IsActive,&ig.LastTestedAt,&ig.LastTestOK,&ig.LastTestMsg,&ig.CreatedAt,&ig.UpdatedAt); err != nil { return nil, err }
		out = append(out, ig)
	}
	return out, rows.Err()
}

func (s *IntegrationStore) GetConfig(ctx context.Context, id, orgID string) ([]byte, error) {
	var enc, iv []byte
	err := s.db.Pool.QueryRow(ctx, `SELECT config_enc,config_iv FROM integrations WHERE id=$1 AND org_id=$2`, id, orgID).Scan(&enc, &iv)
	if err != nil { return nil, err }
	return decryptAESGCM(s.encKey, enc, iv)
}

func (s *IntegrationStore) SetTestResult(ctx context.Context, id string, ok bool, msg string) error {
	_, err := s.db.Pool.Exec(ctx,
		`UPDATE integrations SET last_tested_at=NOW(),last_test_ok=$1,last_test_msg=$2,updated_at=NOW() WHERE id=$3`,
		ok, msg, id)
	return err
}

func (s *IntegrationStore) Delete(ctx context.Context, id, orgID string) error {
	_, err := s.db.Pool.Exec(ctx, `DELETE FROM integrations WHERE id=$1 AND org_id=$2`, id, orgID)
	return err
}

// AES-GCM encryption helpers

// ════════════════════════════════════════════════════════════════════════════
//  PostmortemStore
// ════════════════════════════════════════════════════════════════════════════

type Postmortem struct {
	ID          string     `json:"id"`
	OrgID       string     `json:"org_id"`
	IncidentID  string     `json:"incident_id"`
	Title       string     `json:"title"`
	Severity    string     `json:"severity"`
	Status      string     `json:"status"`
	DetectedAt  *time.Time `json:"detected_at,omitempty"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
	DurationMin int        `json:"duration_min"`
	Impact      string     `json:"impact"`
	Summary     string     `json:"summary"`
	RootCause   string     `json:"root_cause"`
	Timeline    string     `json:"timeline"`
	Whys        string     `json:"whys"`
	ActionItems string     `json:"action_items"`
	Learnings   string     `json:"learnings"`
	AuthorID    string     `json:"author_id"`
	ReviewerID  *string    `json:"reviewer_id,omitempty"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type PostmortemStore struct{ db *DB; log *zap.Logger }
func NewPostmortemStore(db *DB, log *zap.Logger) *PostmortemStore { return &PostmortemStore{db, log} }

func (s *PostmortemStore) Create(ctx context.Context, orgID string, p Postmortem) (*Postmortem, error) {
	p.ID = newID(); p.OrgID = orgID; p.CreatedAt = time.Now(); p.UpdatedAt = time.Now()
	if p.Status == "" { p.Status = "draft" }
	if p.Severity == "" { p.Severity = "HIGH" }
	if p.Timeline == "" { p.Timeline = "[]" }
	if p.Whys == "" { p.Whys = "[]" }
	if p.ActionItems == "" { p.ActionItems = "[]" }
	if p.Learnings == "" { p.Learnings = "[]" }
	_, err := s.db.Pool.Exec(ctx,
		`INSERT INTO postmortems(id,org_id,incident_id,title,severity,status,detected_at,resolved_at,
		  duration_min,impact,summary,root_cause,timeline,whys,action_items,learnings,author_id,reviewer_id,created_at,updated_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb,$14::jsonb,$15::jsonb,$16::jsonb,$17,$18,$19,$20)`,
		p.ID,p.OrgID,p.IncidentID,p.Title,p.Severity,p.Status,p.DetectedAt,p.ResolvedAt,
		p.DurationMin,p.Impact,p.Summary,p.RootCause,p.Timeline,p.Whys,p.ActionItems,p.Learnings,
		p.AuthorID,p.ReviewerID,p.CreatedAt,p.UpdatedAt)
	return &p, err
}

func (s *PostmortemStore) List(ctx context.Context, orgID string, limit, offset int) ([]*Postmortem, error) {
	if limit == 0 { limit = 20 }
	rows, err := s.db.Pool.Query(ctx,
		`SELECT id,org_id,incident_id,title,severity,status,detected_at,resolved_at,duration_min,
		  impact,summary,root_cause,timeline::text,whys::text,action_items::text,learnings,
		  author_id,reviewer_id,published_at,created_at,updated_at
		 FROM postmortems WHERE org_id=$1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		orgID, limit, offset)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*Postmortem
	for rows.Next() {
		p := &Postmortem{}
		if err := rows.Scan(&p.ID,&p.OrgID,&p.IncidentID,&p.Title,&p.Severity,&p.Status,&p.DetectedAt,&p.ResolvedAt,&p.DurationMin,&p.Impact,&p.Summary,&p.RootCause,&p.Timeline,&p.Whys,&p.ActionItems,&p.Learnings,&p.AuthorID,&p.ReviewerID,&p.PublishedAt,&p.CreatedAt,&p.UpdatedAt); err != nil { return nil, err }
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PostmortemStore) GetByID(ctx context.Context, id, orgID string) (*Postmortem, error) {
	p := &Postmortem{}
	err := s.db.Pool.QueryRow(ctx,
		`SELECT id,org_id,incident_id,title,severity,status,detected_at,resolved_at,duration_min,
		  impact,summary,root_cause,timeline::text,whys::text,action_items::text,learnings,
		  author_id,reviewer_id,published_at,created_at,updated_at
		 FROM postmortems WHERE id=$1 AND org_id=$2`, id, orgID).
		Scan(&p.ID,&p.OrgID,&p.IncidentID,&p.Title,&p.Severity,&p.Status,&p.DetectedAt,&p.ResolvedAt,&p.DurationMin,&p.Impact,&p.Summary,&p.RootCause,&p.Timeline,&p.Whys,&p.ActionItems,&p.Learnings,&p.AuthorID,&p.ReviewerID,&p.PublishedAt,&p.CreatedAt,&p.UpdatedAt)
	return p, err
}

func (s *PostmortemStore) Update(ctx context.Context, id, orgID string, p Postmortem) error {
	_, err := s.db.Pool.Exec(ctx,
		`UPDATE postmortems SET title=$1,severity=$2,status=$3,impact=$4,summary=$5,root_cause=$6,
		  timeline=$7::jsonb,whys=$8::jsonb,action_items=$9::jsonb,learnings=$10,reviewer_id=$11,updated_at=NOW()
		 WHERE id=$12 AND org_id=$13`,
		p.Title,p.Severity,p.Status,p.Impact,p.Summary,p.RootCause,
		p.Timeline,p.Whys,p.ActionItems,p.Learnings,p.ReviewerID,id,orgID)
	return err
}

func (s *PostmortemStore) Publish(ctx context.Context, id, orgID string) error {
	_, err := s.db.Pool.Exec(ctx,
		`UPDATE postmortems SET status='published',published_at=NOW(),updated_at=NOW() WHERE id=$1 AND org_id=$2`,
		id, orgID)
	return err
}

func (s *PostmortemStore) Delete(ctx context.Context, id, orgID string) error {
	_, err := s.db.Pool.Exec(ctx, `DELETE FROM postmortems WHERE id=$1 AND org_id=$2 AND status='draft'`, id, orgID)
	return err
}

// ════════════════════════════════════════════════════════════════════════════
//  SSOConfigStore
// ════════════════════════════════════════════════════════════════════════════

type SSOConfig struct {
	ID            string    `json:"id"`
	OrgID         string    `json:"org_id"`
	Type          string    `json:"type"`
	Enabled       bool      `json:"enabled"`
	AutoProvision bool      `json:"auto_provision"`
	DefaultRole   string    `json:"default_role"`
	// SAML
	IDPEntityID   string    `json:"idp_entity_id"`
	IDPSSOUrl     string    `json:"idp_sso_url"`
	IDPCert       string    `json:"idp_cert"`
	SPEntityID    string    `json:"sp_entity_id"`
	// OIDC
	OIDCIssuer       string `json:"oidc_issuer"`
	OIDCClientID     string `json:"oidc_client_id"`
	OIDCClientSecret string `json:"oidc_client_secret,omitempty"` // masked on read
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type SSOConfigStore struct{ db *DB; log *zap.Logger }
func NewSSOConfigStore(db *DB, log *zap.Logger) *SSOConfigStore { return &SSOConfigStore{db, log} }

func (s *SSOConfigStore) Upsert(ctx context.Context, cfg SSOConfig) (*SSOConfig, error) {
	if cfg.ID == "" { cfg.ID = newID() }
	cfg.CreatedAt = time.Now(); cfg.UpdatedAt = time.Now()
	_, err := s.db.Pool.Exec(ctx,
		`INSERT INTO sso_configs(id,org_id,type,enabled,auto_provision,default_role,
		  idp_entity_id,idp_sso_url,idp_cert,sp_entity_id,oidc_issuer,oidc_client_id,oidc_client_secret,created_at,updated_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		 ON CONFLICT(org_id,type) DO UPDATE SET enabled=$4,auto_provision=$5,default_role=$6,
		  idp_entity_id=$7,idp_sso_url=$8,idp_cert=$9,sp_entity_id=$10,oidc_issuer=$11,oidc_client_id=$12,oidc_client_secret=$13,updated_at=$15`,
		cfg.ID,cfg.OrgID,cfg.Type,cfg.Enabled,cfg.AutoProvision,cfg.DefaultRole,
		cfg.IDPEntityID,cfg.IDPSSOUrl,cfg.IDPCert,cfg.SPEntityID,
		cfg.OIDCIssuer,cfg.OIDCClientID,cfg.OIDCClientSecret,cfg.CreatedAt,cfg.UpdatedAt)
	return &cfg, err
}

func (s *SSOConfigStore) GetByOrgAndType(ctx context.Context, orgID, ssoType string) (*SSOConfig, error) {
	cfg := &SSOConfig{}
	err := s.db.Pool.QueryRow(ctx,
		`SELECT id,org_id,type,enabled,auto_provision,default_role,idp_entity_id,idp_sso_url,idp_cert,sp_entity_id,oidc_issuer,oidc_client_id,oidc_client_secret,created_at,updated_at
		 FROM sso_configs WHERE org_id=$1 AND type=$2`, orgID, ssoType).
		Scan(&cfg.ID,&cfg.OrgID,&cfg.Type,&cfg.Enabled,&cfg.AutoProvision,&cfg.DefaultRole,
			&cfg.IDPEntityID,&cfg.IDPSSOUrl,&cfg.IDPCert,&cfg.SPEntityID,
			&cfg.OIDCIssuer,&cfg.OIDCClientID,&cfg.OIDCClientSecret,&cfg.CreatedAt,&cfg.UpdatedAt)
	return cfg, err
}

func (s *SSOConfigStore) ListByOrg(ctx context.Context, orgID string) ([]*SSOConfig, error) {
	rows, err := s.db.Pool.Query(ctx,
		`SELECT id,org_id,type,enabled,auto_provision,default_role,idp_entity_id,idp_sso_url,idp_cert,sp_entity_id,oidc_issuer,oidc_client_id,created_at,updated_at
		 FROM sso_configs WHERE org_id=$1 ORDER BY type`, orgID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*SSOConfig
	for rows.Next() {
		c := &SSOConfig{}
		// Note: OIDCClientSecret intentionally not scanned here (masked in list)
		if err := rows.Scan(&c.ID,&c.OrgID,&c.Type,&c.Enabled,&c.AutoProvision,&c.DefaultRole,&c.IDPEntityID,&c.IDPSSOUrl,&c.IDPCert,&c.SPEntityID,&c.OIDCIssuer,&c.OIDCClientID,&c.CreatedAt,&c.UpdatedAt); err != nil { return nil, err }
		out = append(out, c)
	}
	return out, rows.Err()
}

// ════════════════════════════════════════════════════════════════════════════
//  K8sClusterStore
// ════════════════════════════════════════════════════════════════════════════

type K8sCluster struct {
	ID            string     `json:"id"`
	OrgID         string     `json:"org_id"`
	Name          string     `json:"name"`
	DisplayName   string     `json:"display_name"`
	Region        string     `json:"region"`
	Provider      string     `json:"provider"`
	APIServerURL  string     `json:"api_server_url"`
	Status        string     `json:"status"`
	AgentVersion  string     `json:"agent_version"`
	NodeCount     int        `json:"node_count"`
	PodCount      int        `json:"pod_count"`
	LastSeenAt    *time.Time `json:"last_seen_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type K8sClusterStore struct{ db *DB; log *zap.Logger }
func NewK8sClusterStore(db *DB, log *zap.Logger) *K8sClusterStore { return &K8sClusterStore{db, log} }

func (s *K8sClusterStore) Register(ctx context.Context, orgID string, cl K8sCluster) (*K8sCluster, error) {
	cl.ID = newID(); cl.OrgID = orgID; cl.CreatedAt = time.Now(); cl.UpdatedAt = time.Now()
	if cl.Status == "" { cl.Status = "unknown" }
	if cl.Provider == "" { cl.Provider = "vanilla" }
	_, err := s.db.Pool.Exec(ctx,
		`INSERT INTO k8s_clusters(id,org_id,name,display_name,region,provider,api_server_url,status,agent_version,node_count,pod_count,created_at,updated_at)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		cl.ID,cl.OrgID,cl.Name,cl.DisplayName,cl.Region,cl.Provider,cl.APIServerURL,cl.Status,cl.AgentVersion,cl.NodeCount,cl.PodCount,cl.CreatedAt,cl.UpdatedAt)
	return &cl, err
}

func (s *K8sClusterStore) List(ctx context.Context, orgID string) ([]*K8sCluster, error) {
	rows, err := s.db.Pool.Query(ctx,
		`SELECT id,org_id,name,display_name,region,provider,api_server_url,status,agent_version,node_count,pod_count,last_seen_at,created_at,updated_at
		 FROM k8s_clusters WHERE org_id=$1 ORDER BY name`, orgID)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*K8sCluster
	for rows.Next() {
		c := &K8sCluster{}
		if err := rows.Scan(&c.ID,&c.OrgID,&c.Name,&c.DisplayName,&c.Region,&c.Provider,&c.APIServerURL,&c.Status,&c.AgentVersion,&c.NodeCount,&c.PodCount,&c.LastSeenAt,&c.CreatedAt,&c.UpdatedAt); err != nil { return nil, err }
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *K8sClusterStore) UpdateStatus(ctx context.Context, id string, status, agentVersion string, nodes, pods int) error {
	_, err := s.db.Pool.Exec(ctx,
		`UPDATE k8s_clusters SET status=$1,agent_version=$2,node_count=$3,pod_count=$4,last_seen_at=NOW(),updated_at=NOW() WHERE id=$5`,
		status, agentVersion, nodes, pods, id)
	return err
}

func (s *K8sClusterStore) GetByID(ctx context.Context, id, orgID string) (*K8sCluster, error) {
	c := &K8sCluster{}
	err := s.db.Pool.QueryRow(ctx,
		`SELECT id,org_id,name,display_name,region,provider,api_server_url,status,agent_version,node_count,pod_count,last_seen_at,created_at,updated_at
		 FROM k8s_clusters WHERE id=$1 AND org_id=$2`, id, orgID).
		Scan(&c.ID,&c.OrgID,&c.Name,&c.DisplayName,&c.Region,&c.Provider,&c.APIServerURL,&c.Status,&c.AgentVersion,&c.NodeCount,&c.PodCount,&c.LastSeenAt,&c.CreatedAt,&c.UpdatedAt)
	return c, err
}

func (s *K8sClusterStore) Delete(ctx context.Context, id, orgID string) error {
	_, err := s.db.Pool.Exec(ctx, `DELETE FROM k8s_clusters WHERE id=$1 AND org_id=$2`, id, orgID)
	return err
}

// ════════════════════════════════════════════════════════════════════════════
//  NetworkFlowStore
// ════════════════════════════════════════════════════════════════════════════

type NetworkFlow struct {
	SrcServiceID string  `json:"src_service_id"`
	DstServiceID string  `json:"dst_service_id"`
	Namespace    string  `json:"namespace"`
	Protocol     string  `json:"protocol"`
	BytesPerSec  float64 `json:"bytes_per_sec"`
	LatencyMs    float64 `json:"latency_ms"`
	ErrorRate    float64 `json:"error_rate"`
	Established  int     `json:"established"`
}

type NetworkFlowStore struct{ db *DB; log *zap.Logger }
func NewNetworkFlowStore(db *DB, log *zap.Logger) *NetworkFlowStore { return &NetworkFlowStore{db, log} }

func (s *NetworkFlowStore) Insert(ctx context.Context, f NetworkFlow) error {
	_, err := s.db.Pool.Exec(ctx,
		`INSERT INTO network_flows(src_service_id,dst_service_id,namespace,protocol,bytes_per_sec,latency_ms,error_rate,established)
		 VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,
		f.SrcServiceID,f.DstServiceID,f.Namespace,f.Protocol,f.BytesPerSec,f.LatencyMs,f.ErrorRate,f.Established)
	return err
}

func (s *NetworkFlowStore) ListRecent(ctx context.Context, namespace string, limit int) ([]*NetworkFlow, error) {
	if limit == 0 { limit = 100 }
	q := `SELECT src_service_id,dst_service_id,namespace,protocol,AVG(bytes_per_sec),AVG(latency_ms),AVG(error_rate),SUM(established)
		  FROM network_flows WHERE captured_at > NOW()-INTERVAL '5 minutes'`
	args := []any{}
	if namespace != "" && namespace != "all" { q += ` AND namespace=$1`; args = append(args, namespace) }
	q += ` GROUP BY src_service_id,dst_service_id,namespace,protocol ORDER BY AVG(bytes_per_sec) DESC LIMIT ` + fmt.Sprintf("%d", limit)
	rows, err := s.db.Pool.Query(ctx, q, args...)
	if err != nil { return nil, err }
	defer rows.Close()
	var out []*NetworkFlow
	for rows.Next() {
		f := &NetworkFlow{}
		if err := rows.Scan(&f.SrcServiceID,&f.DstServiceID,&f.Namespace,&f.Protocol,&f.BytesPerSec,&f.LatencyMs,&f.ErrorRate,&f.Established); err != nil { return nil, err }
		out = append(out, f)
	}
	return out, rows.Err()
}

// ════════════════════════════════════════════════════════════════════════════
//  AuditStore
// ════════════════════════════════════════════════════════════════════════════

// ── AES-256-GCM for encrypting integration configs ──────────────────────────

func encryptAESGCM(key, plaintext []byte) (ciphertext, nonce []byte, err error) {
	block, err2 := aesNewCipher(key)
	if err2 != nil { return nil, nil, err2 }
	gcm, err2 := newGCM(block)
	if err2 != nil { return nil, nil, err2 }
	nonce = make([]byte, gcm.NonceSize())
	if _, err2 = rand.Read(nonce); err2 != nil { return nil, nil, err2 }
	return gcm.Seal(nil, nonce, plaintext, nil), nonce, nil
}

func decryptAESGCM(key, ciphertext, nonce []byte) ([]byte, error) {
	block, err := aesNewCipher(key)
	if err != nil { return nil, err }
	gcm, err := newGCM(block)
	if err != nil { return nil, err }
	return gcm.Open(nil, nonce, ciphertext, nil)
}
