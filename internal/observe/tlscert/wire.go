package tlscert

// Observation wire codec.
//
// Under G-1 a probe runs in its own Deployment and the control plane runs in
// the processor, so an Observation taken in one process has to be rebuilt in
// another. Observation keeps its certificate chain unexported so that it cannot
// be altered after the fact; this file is the one sanctioned way across a
// process boundary, and it keeps that guarantee.
//
// # What travels, and what does not
//
// The payload carries exactly what the prober measured that cannot be derived
// from anything else: the DER of each presented certificate, in wire order,
// the observation time and duration, the outcome, the trust result and the
// detail text. Nothing derived from the DER travels. On decode every
// Certificate is rebuilt from its DER by describe, the same function Probe
// uses, so a payload cannot state a notAfter, subject, issuer, serial or SAN
// that its DER does not contain.
//
// Identity never travels as a claim to be believed:
//
//   - The vantage is not in the payload at all. The receiver supplies it in
//     Binding, from its own authenticated source (for F6.1, the verified probe
//     credential). A payload that carries a vantage field is rejected as
//     unknown.
//   - The check ID and endpoint are in the payload only so that they can be
//     compared: each must equal, byte for byte, the value the receiver supplies
//     in Binding from its authoritative source (the synthetic_checks row). A
//     payload for another check, or for an endpoint the row no longer names,
//     is refused with ErrWireBinding.
//   - The payload carries no organization, namespace or subject of any kind.
//
// # Consistency with the prober
//
// A decoded Observation must be one the prober could have produced:
//
//   - HostnameVerified is recomputed from the leaf DER and the endpoint host,
//     exactly as Probe computes it, and must equal the reported value.
//   - ChainVerified depends on the vantage's trust roots and clock, so it is
//     accepted as the vantage's evidence, subject to what is deterministic: a
//     verified chain requires the leaf to be inside its validity window at
//     ObservedAt, and a "not yet valid" or "expired" reason must agree with the
//     leaf's notBefore.
//   - Reason is drawn from the Reason* vocabulary in the order Probe writes it.
//   - Outcome is Observed exactly when both checks passed, ObservedUntrusted
//     otherwise; certificates are present only for those two outcomes, and the
//     trust result is empty for every other outcome.
//   - Detail must be one of the texts Probe writes for that outcome. The package
//     promise that peer-supplied text never reaches Detail therefore survives
//     transport.
//
// # Versioning
//
// The document carries "v". This package produces and accepts WireVersion and
// nothing else; any other value is ErrWireVersion, decided before the rest of
// the document is read, so a newer sender is told it is newer rather than that
// it is malformed. Decoding is strict: unknown fields, trailing data, oversized
// payloads and oversized chains are refused.
//
// # What this file does not do
//
// It does not decide whether an observation is fresh enough to use: ObservedAt
// is reported by the vantage and is not compared with any clock here. That is
// replay defence, an open decision. It does not verify the chain against the
// receiver's roots, authenticate the sender, or look anything up.

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// WireVersion is the only wire version this package produces or accepts.
const WireVersion = 1

// Limits enforced on decode. MaxWireChainBytes matches the 64 KiB ceiling
// crypto/tls places on a handshake message, so any chain Probe can observe
// fits; the other limits bound work on hostile input.
const (
	MaxWireBytes        = 256 << 10
	MaxWireCertificates = 64
	MaxWireChainBytes   = 64 << 10
	MaxWireDuration     = time.Hour
)

// Wire errors. Each is a sentinel; none quotes any part of the payload.
var (
	// ErrWireTooLarge: the payload or its chain exceeds a limit.
	ErrWireTooLarge = errors.New("tlscert: observation payload exceeds a size limit")
	// ErrWireMalformed: not a well-formed wire document (bad JSON, wrong
	// types, unknown fields, trailing data, missing version).
	ErrWireMalformed = errors.New("tlscert: observation payload is malformed")
	// ErrWireVersion: a well-formed document of a version this package does
	// not speak.
	ErrWireVersion = errors.New("tlscert: observation wire version is not supported")
	// ErrWireInvalid: well-formed, but not an observation the prober could
	// have produced (unparseable DER, inconsistent trust, unknown outcome,
	// unexpected detail).
	ErrWireInvalid = errors.New("tlscert: observation payload is not a consistent observation")
	// ErrWireBinding: the payload does not match the check it was received
	// for, or the receiver supplied an incomplete Binding.
	ErrWireBinding = errors.New("tlscert: observation does not match its binding")
)

// Binding is what the receiver already knows from authoritative sources. The
// decoded Observation takes CheckID, Endpoint and Vantage from here, never from
// the payload.
type Binding struct {
	// CheckID is the check the observation is being accepted for.
	CheckID string
	// Endpoint is the check's current target.
	Endpoint string
	// Vantage is the authenticated vantage the observation came from. Both
	// Kind and ID are required.
	Vantage Vantage
}

type wireTrust struct {
	ChainVerified    bool   `json:"chain_verified"`
	HostnameVerified bool   `json:"hostname_verified"`
	Reason           string `json:"reason"`
}

type wireObservation struct {
	Version      int       `json:"v"`
	CheckID      string    `json:"check_id"`
	Endpoint     string    `json:"endpoint"`
	ObservedAt   time.Time `json:"observed_at"`
	DurationNS   int64     `json:"duration_ns"`
	Outcome      Outcome   `json:"outcome"`
	Trust        wireTrust `json:"trust"`
	Detail       string    `json:"detail"`
	Certificates [][]byte  `json:"certificates,omitempty"`
}

// MarshalObservation encodes o for transport. The vantage is not encoded.
//
// It succeeds only if the result decodes: the encoded document is checked
// with UnmarshalObservation against o's own check ID and endpoint before it is
// returned, so a sender never transmits something a receiver of the same
// version would refuse.
func MarshalObservation(o Observation) ([]byte, error) {
	w := wireObservation{
		Version:    WireVersion,
		CheckID:    o.CheckID,
		Endpoint:   o.Endpoint,
		ObservedAt: o.ObservedAt.UTC(),
		DurationNS: int64(o.Duration),
		Outcome:    o.Outcome,
		Trust: wireTrust{
			ChainVerified:    o.Trust.ChainVerified,
			HostnameVerified: o.Trust.HostnameVerified,
			Reason:           o.Trust.Reason,
		},
		Detail: o.Detail,
	}
	for _, c := range o.certificates {
		w.Certificates = append(w.Certificates, c.DER())
	}
	data, err := json.Marshal(w)
	if err != nil {
		return nil, ErrWireInvalid
	}
	check := Binding{
		CheckID:  o.CheckID,
		Endpoint: o.Endpoint,
		Vantage:  Vantage{Kind: "wire-self-check", ID: "wire-self-check"},
	}
	if _, err := UnmarshalObservation(data, check); err != nil {
		return nil, err
	}
	return data, nil
}

// UnmarshalObservation decodes a payload produced by MarshalObservation and
// rebuilds the Observation, bound to b. On any failure it returns the zero
// Observation and exactly one of the Err* wire errors.
func UnmarshalObservation(data []byte, b Binding) (Observation, error) {
	if len(data) > MaxWireBytes {
		return Observation{}, ErrWireTooLarge
	}
	host, err := validBinding(b)
	if err != nil {
		return Observation{}, err
	}

	// The version is read first, leniently, so that a document of another
	// version is reported as such even if its other fields are unknown here.
	var head struct {
		Version *int `json:"v"`
	}
	if err := json.Unmarshal(data, &head); err != nil || head.Version == nil {
		return Observation{}, ErrWireMalformed
	}
	if *head.Version != WireVersion {
		return Observation{}, ErrWireVersion
	}

	var w wireObservation
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&w); err != nil {
		return Observation{}, ErrWireMalformed
	}
	if _, err := dec.Token(); err != io.EOF {
		return Observation{}, ErrWireMalformed
	}

	if w.CheckID != b.CheckID || w.Endpoint != b.Endpoint {
		return Observation{}, ErrWireBinding
	}
	if w.ObservedAt.IsZero() || w.DurationNS < 0 || time.Duration(w.DurationNS) > MaxWireDuration {
		return Observation{}, ErrWireInvalid
	}
	observedAt := w.ObservedAt.UTC()

	certs, err := parseChain(w.Certificates)
	if err != nil {
		return Observation{}, err
	}
	trust := TrustStatus{
		ChainVerified:    w.Trust.ChainVerified,
		HostnameVerified: w.Trust.HostnameVerified,
		Reason:           w.Trust.Reason,
	}

	switch w.Outcome {
	case Observed, ObservedUntrusted:
		if len(certs) == 0 {
			return Observation{}, ErrWireInvalid
		}
		if err := consistentTrust(trust, certs[0], host, observedAt); err != nil {
			return Observation{}, err
		}
		if (w.Outcome == Observed) != trust.Verified() {
			return Observation{}, ErrWireInvalid
		}
		if w.Detail != observedDetail(len(certs), trust) {
			return Observation{}, ErrWireInvalid
		}
	case NoCertificates:
		if len(certs) != 0 || trust != (TrustStatus{}) || w.Detail != noCertificatesDetail {
			return Observation{}, ErrWireInvalid
		}
	case TransportFailure, Timeout, Malformed:
		if len(certs) != 0 || trust != (TrustStatus{}) {
			return Observation{}, ErrWireInvalid
		}
		if w.Detail != string(w.Outcome)+" during dial" && w.Detail != string(w.Outcome)+" during handshake" {
			return Observation{}, ErrWireInvalid
		}
	default:
		return Observation{}, ErrWireInvalid
	}

	obs := Observation{
		CheckID:    b.CheckID,
		Endpoint:   b.Endpoint,
		ObservedAt: observedAt,
		Duration:   time.Duration(w.DurationNS),
		Outcome:    w.Outcome,
		Trust:      trust,
		Vantage:    b.Vantage,
		Detail:     w.Detail,
	}
	if len(certs) > 0 {
		obs.certificates = describe(certs)
	}
	return obs, nil
}

// noCertificatesDetail is the Detail Probe writes for NoCertificates.
const noCertificatesDetail = "handshake completed but the peer presented no certificate"

// observedDetail is the Detail Probe writes for Observed and ObservedUntrusted.
func observedDetail(n int, trust TrustStatus) string {
	if trust.Verified() {
		return fmt.Sprintf("observed %d certificate(s); verification passed", n)
	}
	return fmt.Sprintf("observed %d certificate(s); verification failed: %s", n, trust.Reason)
}

// validBinding refuses a Binding the receiver could not have filled from an
// authoritative source, and returns the endpoint host used for hostname
// verification.
func validBinding(b Binding) (string, error) {
	if strings.TrimSpace(b.CheckID) == "" || strings.TrimSpace(b.Vantage.Kind) == "" || strings.TrimSpace(b.Vantage.ID) == "" {
		return "", ErrWireBinding
	}
	host, _, err := splitEndpoint(b.Endpoint)
	if err != nil {
		return "", ErrWireBinding
	}
	return host, nil
}

// parseChain parses each DER exactly, within the chain limits.
func parseChain(ders [][]byte) ([]*x509.Certificate, error) {
	if len(ders) > MaxWireCertificates {
		return nil, ErrWireTooLarge
	}
	total := 0
	out := make([]*x509.Certificate, 0, len(ders))
	for _, der := range ders {
		total += len(der)
		if total > MaxWireChainBytes {
			return nil, ErrWireTooLarge
		}
		if len(der) == 0 {
			return nil, ErrWireInvalid
		}
		c, err := x509.ParseCertificate(der)
		if err != nil || !bytes.Equal(c.Raw, der) {
			return nil, ErrWireInvalid
		}
		out = append(out, c)
	}
	return out, nil
}

// consistentTrust applies every deterministic relation between a reported
// TrustStatus, the leaf, the endpoint host and the observation time.
func consistentTrust(t TrustStatus, leaf *x509.Certificate, host string, at time.Time) error {
	if (leaf.VerifyHostname(host) == nil) != t.HostnameVerified {
		return ErrWireInvalid
	}
	chainPart := t.Reason
	switch {
	case t.ChainVerified && t.HostnameVerified:
		if t.Reason != "" {
			return ErrWireInvalid
		}
	case t.ChainVerified && !t.HostnameVerified:
		if t.Reason != ReasonHostnameMismatch {
			return ErrWireInvalid
		}
	default: // chain not verified
		if !t.HostnameVerified {
			suffix := " and " + ReasonHostnameMismatch
			if !strings.HasSuffix(t.Reason, suffix) {
				return ErrWireInvalid
			}
			chainPart = strings.TrimSuffix(t.Reason, suffix)
		}
		// chainPart must be a value chainReason produces. chainReason decides
		// between "not yet valid" and "expired" from the leaf's own notBefore,
		// so that choice is checked; the others depend on the vantage's roots.
		switch chainPart {
		case ReasonNotYetValid:
			if !at.Before(leaf.NotBefore) {
				return ErrWireInvalid
			}
		case ReasonExpired:
			if at.Before(leaf.NotBefore) {
				return ErrWireInvalid
			}
		case ReasonUnknownAuthority, ReasonHostnameMismatch, ReasonUnusableChain:
		default:
			return ErrWireInvalid
		}
	}
	// A verified chain contains the leaf, which must be valid at ObservedAt.
	if t.ChainVerified && (at.Before(leaf.NotBefore) || at.After(leaf.NotAfter)) {
		return ErrWireInvalid
	}
	return nil
}
