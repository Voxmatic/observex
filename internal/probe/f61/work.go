package f61

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Assignment is one check this vantage must probe (PROPOSED FANOUT-1).
type Assignment struct {
	CheckID     string `json:"check_id"`
	Endpoint    string `json:"endpoint"`
	IntervalSec int    `json:"interval_sec"`
	TimeoutSec  int    `json:"timeout_sec"`
	PhaseSec    int    `json:"phase_sec"`
}

// WorkSet is the processor's answer to a work request.
type WorkSet struct {
	VantageID           string       `json:"vantage_id"`
	Assignments         []Assignment `json:"assignments"`
	RefreshAfterSec     int          `json:"refresh_after_sec"`
	CredentialExpiresAt time.Time    `json:"credential_expires_at"`
}

// maxWorkResponse bounds the work document the probe will read.
const maxWorkResponse = 2 << 20

// WorkURL is where work is fetched.
func (c *Client) WorkURL() string { return c.base.String() + "/v1/synthetic/probe/work" }

// Work fetches the checks assigned to this vantage. The processor decides
// them from the verified credential; the probe asserts nothing.
func (c *Client) Work(ctx context.Context) (WorkSet, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.WorkURL(), nil)
	if err != nil {
		return WorkSet{}, 0, ErrTransport
	}
	req.Header.Set("Authorization", c.bearer())
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return WorkSet{}, 0, fmt.Errorf("%w: %w", ErrTransport, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxWorkResponse+1))
	if err != nil {
		return WorkSet{}, resp.StatusCode, fmt.Errorf("%w: %w", ErrTransport, err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return WorkSet{}, resp.StatusCode, ErrCredentialRejected
	case http.StatusServiceUnavailable:
		return WorkSet{}, resp.StatusCode, ErrIntakeUnavailable
	default:
		return WorkSet{}, resp.StatusCode, fmt.Errorf("%w (status %d)", ErrUnexpectedResponse, resp.StatusCode)
	}
	if len(raw) > maxWorkResponse {
		return WorkSet{}, resp.StatusCode, fmt.Errorf("%w: work document too large", ErrUnexpectedResponse)
	}
	var ws WorkSet
	if err := json.Unmarshal(raw, &ws); err != nil {
		return WorkSet{}, resp.StatusCode, fmt.Errorf("%w: work document malformed", ErrUnexpectedResponse)
	}
	// Keep only well-formed assignments; the processor already bounds them.
	kept := ws.Assignments[:0]
	for _, a := range ws.Assignments {
		if a.CheckID != "" && a.Endpoint != "" && a.IntervalSec >= 10 && a.IntervalSec <= 3600 &&
			a.TimeoutSec >= 1 && a.TimeoutSec <= 60 && a.PhaseSec >= 0 && a.PhaseSec < a.IntervalSec {
			kept = append(kept, a)
		}
	}
	ws.Assignments = kept
	if ws.RefreshAfterSec < 10 || ws.RefreshAfterSec > 3600 {
		ws.RefreshAfterSec = 60
	}
	return ws, resp.StatusCode, nil
}
