package telemetry_test

import (
	"errors"
	"testing"
	"time"

	"github.com/thiagomarinho/joca/internal/telemetry"
)

func TestRecord_NoopWhenDisabled(t *testing.T) {
	r := telemetry.NewRecorder()
	r.Record(telemetry.CallRecord{Pipeline: "p", Provider: "github", Method: "CurrentStatus", At: time.Now()})
	s := r.Summary()
	if s.TotalCalls != 0 {
		t.Fatalf("expected 0 calls when disabled, got %d", s.TotalCalls)
	}
}

func TestRecord_StoresWhenEnabled(t *testing.T) {
	r := telemetry.NewRecorder()
	r.SetEnabled(true)
	r.Record(telemetry.CallRecord{Pipeline: "p", Provider: "github", Method: "CurrentStatus", At: time.Now(), Duration: 10 * time.Millisecond})
	s := r.Summary()
	if s.TotalCalls != 1 {
		t.Fatalf("expected 1 call, got %d", s.TotalCalls)
	}
}

func TestSummary_Aggregates(t *testing.T) {
	r := telemetry.NewRecorder()
	r.SetEnabled(true)

	errSample := errors.New("boom")

	r.Record(telemetry.CallRecord{Pipeline: "p1", Provider: "github", Method: "CurrentStatus", At: time.Now(), Duration: 100 * time.Millisecond})
	r.Record(telemetry.CallRecord{Pipeline: "p1", Provider: "github", Method: "RecentRuns", At: time.Now(), Duration: 200 * time.Millisecond, Err: errSample})
	r.Record(telemetry.CallRecord{Pipeline: "p2", Provider: "aws", Method: "CurrentStatus", At: time.Now(), Duration: 300 * time.Millisecond})

	s := r.Summary()

	if s.TotalCalls != 3 {
		t.Errorf("TotalCalls: want 3, got %d", s.TotalCalls)
	}
	if s.TotalErrors != 1 {
		t.Errorf("TotalErrors: want 1, got %d", s.TotalErrors)
	}

	gh := s.ByProvider["github"]
	if gh.Calls != 2 {
		t.Errorf("github calls: want 2, got %d", gh.Calls)
	}
	if gh.Errors != 1 {
		t.Errorf("github errors: want 1, got %d", gh.Errors)
	}

	aws := s.ByProvider["aws"]
	if aws.Calls != 1 {
		t.Errorf("aws calls: want 1, got %d", aws.Calls)
	}

	p1 := s.ByPipeline["p1"]
	if p1.Calls != 2 {
		t.Errorf("p1 calls: want 2, got %d", p1.Calls)
	}
	if p1.Provider != "github" {
		t.Errorf("p1 provider: want github, got %s", p1.Provider)
	}

	ms := s.ByMethod["CurrentStatus"]
	if ms.Calls != 2 {
		t.Errorf("CurrentStatus calls: want 2, got %d", ms.Calls)
	}
	mr := s.ByMethod["RecentRuns"]
	if mr.Calls != 1 {
		t.Errorf("RecentRuns calls: want 1, got %d", mr.Calls)
	}
}

func TestAvgDuration_ZeroWhenNoCalls(t *testing.T) {
	r := telemetry.NewRecorder()
	s := r.Summary()
	if s.AvgDuration() != 0 {
		t.Errorf("expected 0 avg duration with no calls")
	}
}

func TestAvgDuration_Computed(t *testing.T) {
	r := telemetry.NewRecorder()
	r.SetEnabled(true)
	r.Record(telemetry.CallRecord{Pipeline: "p", Provider: "github", Method: "CurrentStatus", At: time.Now(), Duration: 100 * time.Millisecond})
	r.Record(telemetry.CallRecord{Pipeline: "p", Provider: "github", Method: "RecentRuns", At: time.Now(), Duration: 200 * time.Millisecond})
	s := r.Summary()
	if s.AvgDuration() != 150*time.Millisecond {
		t.Errorf("expected 150ms avg, got %v", s.AvgDuration())
	}
}

func TestClear_ResetsRecords(t *testing.T) {
	r := telemetry.NewRecorder()
	r.SetEnabled(true)
	r.Record(telemetry.CallRecord{Pipeline: "p", Provider: "github", Method: "CurrentStatus", At: time.Now()})
	r.Clear()
	s := r.Summary()
	if s.TotalCalls != 0 {
		t.Errorf("expected 0 calls after Clear, got %d", s.TotalCalls)
	}
}
