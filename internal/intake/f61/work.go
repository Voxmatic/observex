package f61

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"sort"
	"time"

	wire "github.com/observex/platform/internal/wire/f61"
)

// Work delivery (PROPOSED FANOUT-1): a probe pulls the checks assigned to its
// vantage. The due-slot key is (check, vantage); each vantage probes each of
// its checks every interval_sec, at a stable phase within the interval.
const (
	// WorkRefreshAfter tells the probe when to ask again.
	WorkRefreshAfter = 60 * time.Second
	// MaxAssignments bounds one work response.
	MaxAssignments = 1000
	// Interval and timeout bounds mirror the synthetic_checks CHECK constraints.
	minIntervalSec, maxIntervalSec = 10, 3600
	minTimeoutSec, maxTimeoutSec   = 1, 60
)

// ErrWorkLookup: the organization's checks could not be read.
var ErrWorkLookup = errors.New("intake/f61: checks could not be listed")

// WorkLister lists the synthetic checks of one organization. Implementations
// filter by orgID in the query; Work filters again.
type WorkLister interface {
	ListChecks(ctx context.Context, orgID string) ([]CheckRecord, error)
}

// Assignment is one check a vantage must probe.
type Assignment struct {
	CheckID     string `json:"check_id"`
	Endpoint    string `json:"endpoint"`
	IntervalSec int    `json:"interval_sec"`
	TimeoutSec  int    `json:"timeout_sec"`
	// PhaseSec is the offset within the interval at which this vantage's due
	// slots for this check begin: slot n is due at n*interval + phase (Unix
	// seconds). Stable across restarts; spreads load without coordination.
	PhaseSec int `json:"phase_sec"`
}

// WorkSet is the answer to one work request.
type WorkSet struct {
	VantageID           string       `json:"vantage_id"`
	Assignments         []Assignment `json:"assignments"`
	RefreshAfterSec     int          `json:"refresh_after_sec"`
	CredentialExpiresAt time.Time    `json:"credential_expires_at"`
}

// Phase is the stable due-slot offset for (check, vantage) within interval.
func Phase(checkID, vantageID string, intervalSec int) int {
	if intervalSec <= 0 {
		return 0
	}
	h := sha256.Sum256([]byte(checkID + "\x00" + vantageID))
	return int(binary.BigEndian.Uint64(h[:8]) % uint64(intervalSec))
}

// Work authenticates the caller and returns the checks assigned to its
// vantage: its own organization's enabled ssl checks whose tenancy resolves
// and whose locations select its declared zone (LOC-1). Nothing else is
// returned, so a vantage never learns of another tenant's checks or of checks
// not assigned to it.
func (a *Authorizer) Work(ctx context.Context, authorization string, lister WorkLister) (WorkSet, error) {
	principal, err := a.Authenticate(ctx, authorization)
	if err != nil {
		return WorkSet{}, err
	}
	if lister == nil {
		return WorkSet{}, refuse(ErrUnavailable, ErrWorkLookup)
	}
	records, err := lister.ListChecks(ctx, principal.OrgID)
	if err != nil {
		return WorkSet{}, refuse(ErrUnavailable, errors.Join(ErrWorkLookup, err))
	}
	out := WorkSet{
		VantageID:           principal.VantageID,
		Assignments:         []Assignment{},
		RefreshAfterSec:     int(WorkRefreshAfter / time.Second),
		CredentialExpiresAt: principal.ExpiresAt,
	}
	for _, r := range records {
		if r.Row.OrgID != principal.OrgID || r.Type != CheckTypeSSL || !r.Enabled ||
			!Assigned(r.Locations, principal.Declared.NetworkZone) {
			continue
		}
		if _, err := wire.SubjectFromCheck(r.Row); err != nil {
			continue // tenancy unresolved: no work (TEN-2)
		}
		if r.IntervalSec < minIntervalSec || r.IntervalSec > maxIntervalSec ||
			r.TimeoutSec < minTimeoutSec || r.TimeoutSec > maxTimeoutSec {
			continue
		}
		out.Assignments = append(out.Assignments, Assignment{
			CheckID: r.Row.ID, Endpoint: r.Row.Target, IntervalSec: r.IntervalSec, TimeoutSec: r.TimeoutSec,
			PhaseSec: Phase(r.Row.ID, principal.VantageID, r.IntervalSec),
		})
	}
	sort.Slice(out.Assignments, func(i, j int) bool { return out.Assignments[i].CheckID < out.Assignments[j].CheckID })
	if len(out.Assignments) > MaxAssignments {
		out.Assignments = out.Assignments[:MaxAssignments]
	}
	return out, nil
}
