package budget

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DeepSeekStatusURL is the public, unauthenticated status widget endpoint.
const DeepSeekStatusURL = "https://status.deepseek.com/api/widget/v1/summary.json"

// maxStatusBodyBytes bounds the status response. The widget payload is a few
// hundred bytes; anything larger is a broken or hostile upstream.
const maxStatusBodyBytes = 1 << 20

// Values documented for overall.status in schema_version 1.0.
const (
	OverallOperational   = "operational"
	OverallDegraded      = "degraded"
	OverallPartialOutage = "partial_outage"
	OverallFullOutage    = "full_outage"
	OverallMaintenance   = "maintenance"
)

// Levels are the verdicts this package derives from overall.status.
const (
	LevelHealthy     = "healthy"
	LevelDegraded    = "degraded"
	LevelOutage      = "outage"
	LevelMaintenance = "maintenance"
	LevelUnknown     = "unknown"
)

// Defaults used when the upstream payload omits the polling hints.
const (
	defaultPollAfterSeconds = 30
	defaultMaxStaleSeconds  = 120
)

// StatusTime decodes the status timestamps tolerantly: RFC 3339 strings, unix
// seconds, null and empty values are all accepted. An unrecognized format
// leaves the zero time instead of failing the whole snapshot, so a cosmetic
// timestamp change upstream cannot take the health check down.
type StatusTime struct {
	time.Time
}

func (t *StatusTime) UnmarshalJSON(data []byte) error {
	t.Time = time.Time{}
	s := strings.TrimSpace(string(data))
	if s == "" || s == "null" || s == `""` {
		return nil
	}
	if s[0] == '"' {
		var str string
		if err := json.Unmarshal(data, &str); err != nil {
			return err
		}
		str = strings.TrimSpace(str)
		if str == "" {
			return nil
		}
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
			if v, err := time.Parse(layout, str); err == nil {
				t.Time = v
				return nil
			}
		}
		return nil
	}
	var secs float64
	if err := json.Unmarshal(data, &secs); err == nil {
		t.Time = time.Unix(int64(secs), 0).UTC()
	}
	return nil
}

func (t StatusTime) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(t.UTC().Format(time.RFC3339))
}

// StatusComponent is a component listed on an incident or maintenance.
type StatusComponent struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	GroupName string `json:"group_name"`
	Status    string `json:"status"`
}

// IncidentUpdate is the latest update attached to an incident.
type IncidentUpdate struct {
	Message string `json:"message"`
}

// StatusIncident is one entry of ongoing_incidents.
type StatusIncident struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	Phase              string            `json:"phase"`
	Impact             string            `json:"impact"`
	Status             string            `json:"status"`
	StartedAt          StatusTime        `json:"started_at"`
	LastUpdate         IncidentUpdate    `json:"last_update"`
	AffectedComponents []StatusComponent `json:"affected_components"`
}

// StatusMaintenance is one entry of in_progress_maintenances or
// scheduled_maintenances. The schema documents neither field names nor values,
// so both common spellings of the window are accepted.
type StatusMaintenance struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	Status             string            `json:"status"`
	Description        string            `json:"description"`
	StartsAt           StatusTime        `json:"starts_at"`
	EndsAt             StatusTime        `json:"ends_at"`
	ScheduledStartAt   StatusTime        `json:"scheduled_start_at"`
	ScheduledEndAt     StatusTime        `json:"scheduled_end_at"`
	AffectedComponents []StatusComponent `json:"affected_components"`
}

// Window returns the maintenance window, preferring the scheduled_* fields.
func (m StatusMaintenance) Window() (start, end StatusTime) {
	if !m.ScheduledStartAt.IsZero() || !m.ScheduledEndAt.IsZero() {
		return m.ScheduledStartAt, m.ScheduledEndAt
	}
	return m.StartsAt, m.EndsAt
}

// StatusSummary mirrors the widget JSON.
type StatusSummary struct {
	SchemaVersion          string              `json:"schema_version"`
	GeneratedAt            StatusTime          `json:"generated_at"`
	PollAfterSeconds       int                 `json:"poll_after_seconds"`
	MaxStaleSeconds        int                 `json:"max_stale_seconds"`
	Page                   StatusPage          `json:"page"`
	Overall                OverallStatus       `json:"overall"`
	OngoingIncidents       []StatusIncident    `json:"ongoing_incidents"`
	InProgressMaintenances []StatusMaintenance `json:"in_progress_maintenances"`
	ScheduledMaintenances  []StatusMaintenance `json:"scheduled_maintenances"`
}

type StatusPage struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type OverallStatus struct {
	Status string `json:"status"`
}

// Health is the derived verdict: whether DeepSeek is operating normally, plus
// the freshness of the snapshot it was derived from.
type Health struct {
	OK               bool       `json:"ok"`
	Level            string     `json:"level"`
	OverallStatus    string     `json:"overall_status"`
	Headline         string     `json:"headline"`
	Detail           string     `json:"detail"`
	IncidentCount    int        `json:"incident_count"`
	MaintenanceCount int        `json:"maintenance_count"`
	ScheduledCount   int        `json:"scheduled_maintenance_count"`
	Stale            bool       `json:"stale"`
	DataAgeSeconds   int64      `json:"data_age_seconds"`
	MaxStaleSeconds  int        `json:"max_stale_seconds"`
	PollAfterSeconds int        `json:"poll_after_seconds"`
	GeneratedAt      StatusTime `json:"generated_at"`
	CheckedAt        time.Time  `json:"checked_at"`
}

// StatusResult is one successful status check. OK reports that the check
// itself succeeded; Health.OK reports that the service is operational.
type StatusResult struct {
	OK          bool           `json:"ok"`
	Health      *Health        `json:"health"`
	Summary     *StatusSummary `json:"summary"`
	FetchedAt   time.Time      `json:"fetched_at"`
	ValidatedAt string         `json:"validated_at,omitempty"`
	Cached      bool           `json:"cached"`
	NotModified bool           `json:"not_modified"`
}

// Verdict derives the health verdict from a widget snapshot at the given time.
func Verdict(summary *StatusSummary, now time.Time) *Health {
	h := &Health{
		CheckedAt:        now.UTC(),
		Level:            LevelUnknown,
		MaxStaleSeconds:  defaultMaxStaleSeconds,
		PollAfterSeconds: defaultPollAfterSeconds,
		DataAgeSeconds:   -1,
		GeneratedAt:      StatusTime{},
	}
	if summary == nil {
		h.Headline = "No status data"
		h.Detail = "The status endpoint returned no summary."
		return h
	}

	h.OverallStatus = strings.ToLower(strings.TrimSpace(summary.Overall.Status))
	h.GeneratedAt = summary.GeneratedAt
	h.IncidentCount = len(summary.OngoingIncidents)
	h.MaintenanceCount = len(summary.InProgressMaintenances)
	h.ScheduledCount = len(summary.ScheduledMaintenances)
	h.PollAfterSeconds = pollInterval(summary)
	if summary.MaxStaleSeconds > 0 {
		h.MaxStaleSeconds = summary.MaxStaleSeconds
	}

	switch h.OverallStatus {
	case OverallOperational:
		h.Level = LevelHealthy
		h.Headline = "All systems operational"
	case OverallDegraded:
		h.Level = LevelDegraded
		h.Headline = "Service degraded"
	case OverallPartialOutage:
		h.Level = LevelOutage
		h.Headline = "Partial outage"
	case OverallFullOutage:
		h.Level = LevelOutage
		h.Headline = "Full outage"
	case OverallMaintenance:
		h.Level = LevelMaintenance
		h.Headline = "Maintenance in progress"
	default:
		h.Headline = "Unknown status"
		if h.OverallStatus == "" {
			h.Detail = "Upstream did not report an overall status."
		} else {
			h.Detail = fmt.Sprintf("Upstream reported an unrecognized status %q.", h.OverallStatus)
		}
	}

	if h.Level != LevelUnknown {
		h.Detail = healthDetail(h)
	}
	h.OK = h.Level == LevelHealthy

	if !summary.GeneratedAt.IsZero() {
		age := now.Sub(summary.GeneratedAt.Time)
		if age < 0 {
			// Clock skew between this host and the status page: not staleness.
			age = 0
		}
		h.DataAgeSeconds = int64(age.Seconds())
		h.Stale = h.DataAgeSeconds > int64(h.MaxStaleSeconds)
	}

	return h
}

// pollInterval returns the upstream-recommended polling interval in seconds.
func pollInterval(summary *StatusSummary) int {
	if summary == nil || summary.PollAfterSeconds <= 0 {
		return defaultPollAfterSeconds
	}
	return clampInt(summary.PollAfterSeconds, 5, 600)
}

func healthDetail(h *Health) string {
	var parts []string
	if h.IncidentCount > 0 {
		parts = append(parts, plural(h.IncidentCount, "ongoing incident", "ongoing incidents"))
	}
	if h.MaintenanceCount > 0 {
		parts = append(parts, plural(h.MaintenanceCount, "maintenance in progress", "maintenances in progress"))
	}
	if h.ScheduledCount > 0 {
		parts = append(parts, plural(h.ScheduledCount, "scheduled maintenance", "scheduled maintenances"))
	}
	if len(parts) == 0 {
		return "No ongoing incidents or maintenance."
	}
	return strings.Join(parts, ", ") + "."
}

func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", n, pluralForm)
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// StatusClient fetches the status widget, caches the snapshot for the
// upstream-recommended poll interval, and revalidates with the stored ETag.
// Its methods are safe for concurrent use.
type StatusClient struct {
	URL  string
	HTTP *http.Client

	now func() time.Time

	mu          sync.Mutex
	summary     *StatusSummary
	etag        string
	validatedAt string
	fetchedAt   time.Time
}

// NewStatusClient returns a client for the given widget URL.
func NewStatusClient(url string) *StatusClient {
	if url == "" {
		url = DeepSeekStatusURL
	}
	return &StatusClient{
		URL:  url,
		HTTP: &http.Client{Timeout: 6 * time.Second},
		now:  time.Now,
	}
}

var defaultStatusClient = NewStatusClient(DeepSeekStatusURL)

// FetchDeepSeekStatus checks the DeepSeek service status using the shared
// client and cache.
func FetchDeepSeekStatus(ctx context.Context) (*StatusResult, error) {
	return defaultStatusClient.Status(ctx)
}

// Status returns the current snapshot, fetching it when the cached copy is
// older than poll_after_seconds.
func (c *StatusClient) Status(ctx context.Context) (*StatusResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	nowFn := c.now
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn()
	if c.summary != nil {
		ttl := time.Duration(pollInterval(c.summary)) * time.Second
		if now.Sub(c.fetchedAt) < ttl {
			return c.result(now, true, false), nil
		}
	}

	summary, etag, validatedAt, notModified, err := c.fetch(ctx)
	if err != nil {
		return nil, err
	}
	if notModified {
		if c.summary == nil {
			return nil, fmt.Errorf("deepseek status: upstream returned 304 without a cached snapshot")
		}
		if etag != "" {
			c.etag = etag
		}
	} else {
		c.summary = summary
		c.etag = etag
	}
	if validatedAt != "" {
		c.validatedAt = validatedAt
	}
	c.fetchedAt = now

	return c.result(now, false, notModified), nil
}

// result builds a StatusResult from the cached snapshot. The health verdict is
// recomputed on every call so the reported data age stays accurate.
func (c *StatusClient) result(now time.Time, cached, notModified bool) *StatusResult {
	return &StatusResult{
		OK:          true,
		Health:      Verdict(c.summary, now),
		Summary:     c.summary,
		FetchedAt:   c.fetchedAt.UTC(),
		ValidatedAt: c.validatedAt,
		Cached:      cached,
		NotModified: notModified,
	}
}

func (c *StatusClient) fetch(ctx context.Context) (*StatusSummary, string, string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return nil, "", "", false, fmt.Errorf("deepseek status: failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if c.etag != "" {
		req.Header.Set("If-None-Match", c.etag)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", "", false, fmt.Errorf("deepseek status: request failed: %w", err)
	}
	defer resp.Body.Close()

	validatedAt := resp.Header.Get("X-Status-Validated-At")
	switch {
	case resp.StatusCode == http.StatusNotModified:
		return nil, resp.Header.Get("ETag"), validatedAt, true, nil
	case resp.StatusCode != http.StatusOK:
		return nil, "", validatedAt, false, fmt.Errorf("deepseek status: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxStatusBodyBytes+1))
	if err != nil {
		return nil, "", validatedAt, false, fmt.Errorf("deepseek status: failed to read response: %w", err)
	}
	if len(body) > maxStatusBodyBytes {
		return nil, "", validatedAt, false, fmt.Errorf("deepseek status: response exceeds %d bytes", maxStatusBodyBytes)
	}

	var summary StatusSummary
	if err := json.Unmarshal(body, &summary); err != nil {
		return nil, "", validatedAt, false, fmt.Errorf("deepseek status: failed to parse response: %w", err)
	}
	if strings.TrimSpace(summary.Overall.Status) == "" {
		return nil, "", validatedAt, false, fmt.Errorf("deepseek status: response is missing overall.status")
	}
	return &summary, resp.Header.Get("ETag"), validatedAt, false, nil
}
