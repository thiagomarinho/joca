package telemetry

import (
	"sync"
	"time"
)

// CallRecord holds metadata about a single provider API call.
type CallRecord struct {
	Pipeline string
	Provider string // "github" or "aws"
	Method   string // "CurrentStatus" or "RecentRuns"
	At       time.Time
	Duration time.Duration
	Err      error
}

// ProviderStats holds aggregated stats for a provider.
type ProviderStats struct {
	Calls    int
	Errors   int
	TotalDur time.Duration
}

// PipelineStats holds aggregated stats for a pipeline.
type PipelineStats struct {
	Provider string
	Calls    int
	Errors   int
}

// MethodStats holds aggregated stats for a method.
type MethodStats struct {
	Calls int
}

// Summary is a snapshot of aggregated telemetry data.
type Summary struct {
	Enabled     bool
	TotalCalls  int
	TotalErrors int
	TotalDur    time.Duration
	Since       time.Time
	ByProvider  map[string]ProviderStats
	ByPipeline  map[string]PipelineStats
	ByMethod    map[string]MethodStats
}

// AvgDuration returns the average call duration, or 0 when there are no calls.
func (s Summary) AvgDuration() time.Duration {
	if s.TotalCalls == 0 {
		return 0
	}
	return s.TotalDur / time.Duration(s.TotalCalls)
}

// Recorder tracks API call telemetry. It is thread-safe.
type Recorder struct {
	mu      sync.Mutex
	enabled bool
	since   time.Time
	records []CallRecord
}

// NewRecorder creates a new disabled Recorder.
func NewRecorder() *Recorder {
	return &Recorder{}
}

// SetEnabled enables or disables recording. On first enable, resets the since timestamp.
func (r *Recorder) SetEnabled(enabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if enabled && !r.enabled {
		r.since = time.Now()
		r.records = nil
	}
	r.enabled = enabled
}

// IsEnabled reports whether recording is enabled.
func (r *Recorder) IsEnabled() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.enabled
}

// Record adds a call record. It is a no-op when disabled.
func (r *Recorder) Record(rec CallRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.enabled {
		return
	}
	r.records = append(r.records, rec)
}

// Clear resets all records and resets the since timestamp to now.
func (r *Recorder) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = nil
	r.since = time.Now()
}

// Summary returns an aggregated snapshot of all recorded calls.
func (r *Recorder) Summary() Summary {
	r.mu.Lock()
	defer r.mu.Unlock()

	s := Summary{
		Enabled:    r.enabled,
		Since:      r.since,
		ByProvider: make(map[string]ProviderStats),
		ByPipeline: make(map[string]PipelineStats),
		ByMethod:   make(map[string]MethodStats),
	}

	for _, rec := range r.records {
		s.TotalCalls++
		s.TotalDur += rec.Duration
		if rec.Err != nil {
			s.TotalErrors++
		}

		ps := s.ByProvider[rec.Provider]
		ps.Calls++
		ps.TotalDur += rec.Duration
		if rec.Err != nil {
			ps.Errors++
		}
		s.ByProvider[rec.Provider] = ps

		pl := s.ByPipeline[rec.Pipeline]
		pl.Provider = rec.Provider
		pl.Calls++
		if rec.Err != nil {
			pl.Errors++
		}
		s.ByPipeline[rec.Pipeline] = pl

		ms := s.ByMethod[rec.Method]
		ms.Calls++
		s.ByMethod[rec.Method] = ms
	}

	return s
}
