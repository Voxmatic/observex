// Package probetoken issues and verifies the credential that one dedicated
// F6.1 synthetic probe Deployment presents to the control plane.
//
// # Why a separate credential
//
// Under G-1 each vantage is its own Deployment, so each vantage can be handed
// its own credential by an administrator at deployment time. The credential,
// not the probe, is what establishes the vantage identity: the vantage ID is
// minted by the server at issuance, signed into the credential, and read back
// only from a verified signature. Nothing the probe sends (environment, header,
// request body, self-generated agent ID) can establish or override it.
//
// # Shape
//
// A credential is TokenPrefix followed by a compact JWT signed with HS256 by
// github.com/golang-jwt/jwt/v5, the library and algorithm the agent install
// token already uses. Its payload carries:
//
//	typ           TokenType ("observex_synthetic_probe"), never "observex_agent_install"
//	org_id        the issuing administrator's organization, from the gateway auth context
//	vantage_id    server-minted, MAC-bound to org_id (see NewVantageID)
//	scopes        exactly [Scope]; no telemetry-submission scope, no wildcard
//	network_zone, cluster_name, environment, host_group
//	              operator declarations recorded at issuance (F6.1-VANTAGE-1(b));
//	              optional, never defaulted, never verified against reality
//	iat, exp      bounded lifetime, at most MaxTTL
//
// It deliberately carries no "sub" and no "jti". The user-session validator in
// internal/middleware reads "sub" as a user ID, so leaving it out means that
// even a misconfigured deployment that reused the session secret could not
// turn a probe credential into a session. jti is not used for anything.
//
// # Key
//
// The key is loaded like the internal service token (internal/servicetoken):
// KeyFileEnvVar wins when set, even if the file is missing, unreadable or too
// short (there is then no fallback to KeyEnvVar); the value is trimmed with
// strings.TrimSpace and must be at least MinKeyLength bytes. The key is never
// printed: Key formats as "[REDACTED]" and has no accessor for its value.
//
// Two sub-keys are derived from it with HMAC-SHA256 under fixed labels, one to
// sign credentials and one to bind vantage IDs to their organization, so
// neither use can be substituted for the other and a token signed directly with
// the raw key does not verify.
//
// The key must not be the user-session JWT secret or the agent install token
// secret. Issuers are expected to refuse to issue when SameAs reports a match.
// A dedicated key means the ingestor (which holds the agent secret) rejects a
// probe credential on signature, and a service that verifies probe credentials
// is not thereby able to mint telemetry or session tokens.
//
// # Verification
//
// Verify pins the algorithm to HS256, requires exp and iat, rejects iat in the
// future beyond ClockSkew, and fails closed on any missing, malformed or extra
// value: wrong typ, anything other than exactly [Scope], a blank or invalid
// org_id, a vantage_id not bound to that org_id, an invalid declaration, or a
// lifetime longer than MaxTTL. Signature comparison is constant-time (hmac.Equal
// inside the JWT library) and so is the vantage binding check. Errors are fixed
// sentinels and never contain any part of the presented credential.
//
// # What this package does not do
//
// It does not revoke. A credential stays valid until it expires; rotation
// issues a new credential for the same vantage, but the old one is not
// invalidated. Per-credential revocation needs persisted state, which is
// outside F6.1-VANTAGE-1(c) and requires separate migration approval.
//
// It does not prove possession. A credential is a bearer token: two processes
// presenting the same credential are indistinguishable. Per-vantage identity is
// therefore exactly as strong as the custody of each Deployment's credential.
//
// It does not verify operator declarations. "External", zone and cluster are
// what the administrator declared at issuance, not measured properties.
//
// It holds no registry, persists nothing and opens no connection.
package probetoken
