package probetoken

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// TokenPrefix starts every probe credential. It differs from the agent
	// install token's "oxat_" so the two are never confused by people, secret
	// scanners or parsers.
	TokenPrefix = "oxpt_"
	// TokenType is the typ claim of a probe credential.
	TokenType = "observex_synthetic_probe"
	// Scope is the dedicated F6.1-INTAKE-1(b) scope. It is the only scope a
	// probe credential carries and it is not in the ingestor's scope table.
	Scope = "synthetic:report"

	// DefaultTTL is used when an issue request names no lifetime.
	DefaultTTL = 168 * time.Hour
	// MinTTL and MaxTTL bound a requested lifetime. Expiry is the only
	// revocation mechanism, so the upper bound is part of the security model.
	MinTTL = time.Hour
	MaxTTL = 720 * time.Hour
	// ClockSkew is the leeway allowed between issuer and verifier clocks.
	ClockSkew = 30 * time.Second

	// MaxTokenLength bounds what Verify will parse.
	MaxTokenLength = 4096
	// MaxOrgIDLength bounds an org_id claim.
	MaxOrgIDLength = 256
	// MaxDeclarationLength bounds each declared attribute.
	MaxDeclarationLength = 128
)

// Errors. None of them ever contains any part of a presented credential.
var (
	ErrKeyNotConfigured = errors.New("probetoken: key is not configured")
	ErrNoClock          = errors.New("probetoken: a time is required")
	ErrMalformed        = errors.New("probetoken: credential is malformed")
	ErrSignature        = errors.New("probetoken: credential signature or algorithm is invalid")
	ErrExpired          = errors.New("probetoken: credential has expired")
	ErrNotYetValid      = errors.New("probetoken: credential is not yet valid")
	ErrWrongType        = errors.New("probetoken: credential is not a synthetic probe credential")
	ErrScope            = errors.New("probetoken: credential does not carry exactly the synthetic probe scope")
	ErrOrg              = errors.New("probetoken: org_id is missing or invalid")
	ErrVantageID        = errors.New("probetoken: vantage_id is missing, malformed or not bound to org_id")
	ErrDeclaration      = errors.New("probetoken: vantage declaration is invalid")
	ErrLifetime         = errors.New("probetoken: credential lifetime is outside the permitted range")
	ErrRandom           = errors.New("probetoken: could not generate a vantage id")
)

// Declaration is what the administrator declares about a vantage at issuance
// (F6.1-VANTAGE-1(b), the agent vocabulary). Every field is optional and is
// recorded exactly as given: nothing is defaulted, trimmed or verified.
type Declaration struct {
	NetworkZone string
	ClusterName string
	Environment string
	HostGroup   string
}

// Validate rejects a declaration that could not be stored or displayed
// safely: invalid UTF-8, control characters, or a field longer than
// MaxDeclarationLength bytes.
func (d Declaration) Validate() error {
	for _, v := range []string{d.NetworkZone, d.ClusterName, d.Environment, d.HostGroup} {
		if !validText(v, MaxDeclarationLength) {
			return ErrDeclaration
		}
	}
	return nil
}

// Credential is a probe credential value. It never prints its value; the
// only way to read it is Reveal, which exists so the issuer can hand it to the
// administrator exactly once and so a probe can place it in an Authorization
// header. Callers must never log what Reveal returns.
type Credential struct{ v *string }

// ErrCredentialFormat: a value that cannot be a probe credential.
var ErrCredentialFormat = errors.New("probetoken: value is not a probe credential")

// ParseCredential wraps a credential value held by a probe, after trimming
// surrounding whitespace (such as a trailing newline in a Secret file). It
// checks the shape only: the prefix, the length bound and the absence of
// inner whitespace. It does not verify the signature, which only the
// processor can do.
func ParseCredential(value string) (Credential, error) {
	v := strings.TrimSpace(value)
	if len(v) <= len(TokenPrefix) || len(v) > MaxTokenLength || !strings.HasPrefix(v, TokenPrefix) {
		return Credential{}, ErrCredentialFormat
	}
	if strings.IndexFunc(v, unicode.IsSpace) >= 0 || strings.Count(v, ".") != 2 {
		return Credential{}, ErrCredentialFormat
	}
	return Credential{v: &v}, nil
}

// Reveal returns the credential value.
func (c Credential) Reveal() string {
	if c.v == nil {
		return ""
	}
	return *c.v
}

func (c Credential) String() string               { return redacted }
func (c Credential) GoString() string             { return redacted }
func (c Credential) MarshalText() ([]byte, error) { return []byte(redacted), nil }
func (c Credential) MarshalJSON() ([]byte, error) { return []byte(`"` + redacted + `"`), nil }
func (c Credential) Format(f fmt.State, _ rune)   { _, _ = f.Write([]byte(redacted)) }

// IssueRequest describes one credential to issue.
type IssueRequest struct {
	// OrgID is the issuing administrator's organization. The issuer takes it
	// from its own authenticated context, never from the request.
	OrgID string
	// VantageID is empty to mint a new vantage, or an existing vantage ID to
	// re-issue (rotate) the credential for that vantage. A supplied ID must
	// have been minted by this key for OrgID.
	VantageID string
	// Declared is the operator declaration recorded in the credential.
	Declared Declaration
	// TTL is the credential lifetime; zero means DefaultTTL.
	TTL time.Duration
}

// Issued is the result of Issue.
type Issued struct {
	Credential Credential
	OrgID      string
	VantageID  string
	Declared   Declaration
	// Rotated is true when an existing VantageID was re-issued.
	Rotated   bool
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// Principal is what a verified credential establishes.
type Principal struct {
	OrgID     string
	VantageID string
	Declared  Declaration
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// claims is the credential payload. RegisteredClaims supplies iat and exp;
// its other fields are never set by Issue.
type claims struct {
	Type        string   `json:"typ"`
	OrgID       string   `json:"org_id"`
	VantageID   string   `json:"vantage_id"`
	Scopes      []string `json:"scopes"`
	NetworkZone string   `json:"network_zone,omitempty"`
	ClusterName string   `json:"cluster_name,omitempty"`
	Environment string   `json:"environment,omitempty"`
	HostGroup   string   `json:"host_group,omitempty"`
	jwt.RegisteredClaims
}

// Issue signs a credential for req at time now. random supplies the vantage
// nonce when a new vantage is minted.
func (k Key) Issue(req IssueRequest, now time.Time, random io.Reader) (Issued, error) {
	if !k.Configured() {
		return Issued{}, ErrKeyNotConfigured
	}
	if now.IsZero() {
		return Issued{}, ErrNoClock
	}
	if !validOrgID(req.OrgID) {
		return Issued{}, ErrOrg
	}
	if err := req.Declared.Validate(); err != nil {
		return Issued{}, err
	}
	ttl := req.TTL
	if ttl == 0 {
		ttl = DefaultTTL
	}
	if ttl < MinTTL || ttl > MaxTTL {
		return Issued{}, ErrLifetime
	}

	vantageID := req.VantageID
	rotated := vantageID != ""
	if rotated {
		if !k.VantageIDBoundTo(vantageID, req.OrgID) {
			return Issued{}, ErrVantageID
		}
	} else {
		id, err := k.NewVantageID(req.OrgID, random)
		if err != nil {
			return Issued{}, err
		}
		vantageID = id
	}

	issuedAt := now.UTC().Truncate(time.Second)
	expiresAt := issuedAt.Add(ttl)
	c := claims{
		Type:        TokenType,
		OrgID:       req.OrgID,
		VantageID:   vantageID,
		Scopes:      []string{Scope},
		NetworkZone: req.Declared.NetworkZone,
		ClusterName: req.Declared.ClusterName,
		Environment: req.Declared.Environment,
		HostGroup:   req.Declared.HostGroup,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(k.m.sign)
	if err != nil {
		return Issued{}, ErrMalformed
	}
	value := TokenPrefix + signed
	return Issued{
		Credential: Credential{v: &value},
		OrgID:      req.OrgID,
		VantageID:  vantageID,
		Declared:   req.Declared,
		Rotated:    rotated,
		IssuedAt:   issuedAt,
		ExpiresAt:  expiresAt,
	}, nil
}

// Verify checks a presented credential at time now and returns what it
// establishes. Every failure returns the zero Principal and a sentinel error.
func (k Key) Verify(presented string, now time.Time) (Principal, error) {
	if !k.Configured() {
		return Principal{}, ErrKeyNotConfigured
	}
	if now.IsZero() {
		return Principal{}, ErrNoClock
	}
	if len(presented) <= len(TokenPrefix) || len(presented) > MaxTokenLength ||
		!strings.HasPrefix(presented, TokenPrefix) {
		return Principal{}, ErrMalformed
	}

	var c claims
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(ClockSkew),
		jwt.WithTimeFunc(func() time.Time { return now }),
		jwt.WithStrictDecoding(),
	)
	tok, err := parser.ParseWithClaims(presented[len(TokenPrefix):], &c, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, ErrSignature
		}
		return k.m.sign, nil
	})
	if err != nil {
		return Principal{}, mapParseError(err)
	}
	if tok == nil || !tok.Valid {
		return Principal{}, ErrSignature
	}

	if c.Type != TokenType {
		return Principal{}, ErrWrongType
	}
	if len(c.Scopes) != 1 || c.Scopes[0] != Scope {
		return Principal{}, ErrScope
	}
	if !validOrgID(c.OrgID) {
		return Principal{}, ErrOrg
	}
	if !k.VantageIDBoundTo(c.VantageID, c.OrgID) {
		return Principal{}, ErrVantageID
	}
	if c.IssuedAt == nil || c.ExpiresAt == nil {
		return Principal{}, ErrLifetime
	}
	life := c.ExpiresAt.Time.Sub(c.IssuedAt.Time)
	if life <= 0 || life > MaxTTL {
		return Principal{}, ErrLifetime
	}
	declared := Declaration{
		NetworkZone: c.NetworkZone,
		ClusterName: c.ClusterName,
		Environment: c.Environment,
		HostGroup:   c.HostGroup,
	}
	if err := declared.Validate(); err != nil {
		return Principal{}, err
	}
	return Principal{
		OrgID:     c.OrgID,
		VantageID: c.VantageID,
		Declared:  declared,
		IssuedAt:  c.IssuedAt.Time.UTC(),
		ExpiresAt: c.ExpiresAt.Time.UTC(),
	}, nil
}

// mapParseError reduces a JWT library error to a sentinel, so that nothing
// the library might quote from the presented value reaches a caller or a log.
func mapParseError(err error) error {
	switch {
	case errors.Is(err, jwt.ErrTokenSignatureInvalid), errors.Is(err, jwt.ErrTokenUnverifiable):
		return ErrSignature
	case errors.Is(err, jwt.ErrTokenExpired):
		return ErrExpired
	case errors.Is(err, jwt.ErrTokenUsedBeforeIssued), errors.Is(err, jwt.ErrTokenNotValidYet):
		return ErrNotYetValid
	case errors.Is(err, jwt.ErrTokenRequiredClaimMissing):
		return ErrLifetime
	default:
		return ErrMalformed
	}
}

// validOrgID accepts a non-blank org_id of bounded length with no control
// characters. It is compared byte for byte elsewhere and never rewritten.
func validOrgID(s string) bool {
	return strings.TrimSpace(s) != "" && validText(s, MaxOrgIDLength)
}

func validText(s string, max int) bool {
	if len(s) > max || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
