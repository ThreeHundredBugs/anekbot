package stats

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ThreeHundredBugs/anekbot/internal/llm"
)

// Compile-time check: *Stats must satisfy llm.Recorder for SetRecorder wiring.
var _ llm.Recorder = (*Stats)(nil)

func TestRecordAnek_TotalsAndPerUser(t *testing.T) {
	s := New()
	s.RecordAnek(1, "alice", "message", "classic")
	s.RecordAnek(1, "alice", "inline", "ai")
	s.RecordAnek(2, "bob", "message", "classic")

	snap := s.Snapshot(10)
	if snap.TotalAneks != 3 {
		t.Errorf("TotalAneks = %d, want 3", snap.TotalAneks)
	}
	if snap.TotalAIAneks != 1 {
		t.Errorf("TotalAIAneks = %d, want 1", snap.TotalAIAneks)
	}
	if snap.TotalUsers != 2 {
		t.Errorf("TotalUsers = %d, want 2", snap.TotalUsers)
	}
}

func TestSnapshot_TopUsersSortedByCountThenID(t *testing.T) {
	s := New()
	for i := 0; i < 3; i++ {
		s.RecordAnek(1, "alice", "message", "classic")
	}
	s.RecordAnek(2, "bob", "message", "classic")
	s.RecordAnek(3, "carol", "message", "classic")

	snap := s.Snapshot(2)
	if len(snap.TopUsers) != 2 {
		t.Fatalf("TopUsers = %d entries, want 2 (topN)", len(snap.TopUsers))
	}
	if snap.TopUsers[0].UserID != 1 || snap.TopUsers[0].Count != 3 {
		t.Errorf("TopUsers[0] = %+v, want alice with count 3", snap.TopUsers[0])
	}
	if snap.TopUsers[1].UserID != 2 {
		t.Errorf("TopUsers[1] = %+v, want bob (lower id breaks the tie with carol)", snap.TopUsers[1])
	}
}

func TestRecordAnek_ZeroUserIDNotTracked(t *testing.T) {
	s := New()
	s.RecordAnek(0, "", "message", "classic")

	snap := s.Snapshot(10)
	if snap.TotalAneks != 1 {
		t.Errorf("TotalAneks = %d, want 1 (still counted globally)", snap.TotalAneks)
	}
	if snap.TotalUsers != 0 {
		t.Errorf("TotalUsers = %d, want 0 (userID 0 means unknown sender)", snap.TotalUsers)
	}
}

func TestNilStats_MethodsAreNoops(t *testing.T) {
	var s *Stats
	s.RecordAnek(1, "alice", "message", "classic")
	s.RecordQuestionAnswered(1, "alice")
	s.RecordSwearingReaction()
	s.RecordPromotionShown()
	s.ObserveRequest("gemini", true, time.Millisecond)
	s.ObserveFallback()
	s.ObserveRateLimitRejection("per_user")
	s.IncConcurrency()
	s.DecConcurrency()

	if snap := s.Snapshot(10); snap.TotalAneks != 0 || snap.TotalUsers != 0 {
		t.Errorf("nil Stats.Snapshot = %+v, want zero value", snap)
	}
}

func TestHandler_RequiresBearerToken(t *testing.T) {
	s := New()
	s.RecordAnek(1, "alice", "message", "classic")
	handler := s.Handler("secret")

	tests := []struct {
		name   string
		header string
		want   int
	}{
		{"no header", "", http.StatusUnauthorized},
		{"wrong token", "Bearer nope", http.StatusUnauthorized},
		{"not bearer", "Basic c2VjcmV0", http.StatusUnauthorized},
		{"correct token", "Bearer secret", http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestHandler_ServesPrometheusFormat(t *testing.T) {
	s := New()
	s.RecordAnek(1, "alice", "message", "classic")
	handler := s.Handler("secret")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `anekbot_aneks_sent_total{source="message",kind="classic"} 1`) {
		t.Errorf("body missing expected metric line, got:\n%s", body)
	}
}
