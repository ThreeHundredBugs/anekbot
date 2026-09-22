package stats

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/VictoriaMetrics/metrics"
)

type UserID int64

type Stats struct {
	set *metrics.Set

	llmFallbackTotal    *metrics.Counter
	swearingReactions   *metrics.Counter
	promotionsShown     *metrics.Counter
	questionsAnswered   *metrics.Counter
	llmConcurrencyInUse *metrics.Gauge
	uniqueUsers         *metrics.Gauge

	totalAneksClassic atomic.Int64
	totalAneksAI      atomic.Int64

	mu      sync.Mutex
	perUser map[UserID]*userCount
}

type userCount struct {
	username string
	count    int64
}

func New() *Stats {
	metrics.ExposeMetadata(true)

	set := metrics.NewSet()
	s := &Stats{
		set:     set,
		perUser: make(map[UserID]*userCount),

		llmFallbackTotal:  set.NewCounter("anekbot_llm_fallback_total"),
		swearingReactions: set.NewCounter("anekbot_swearing_reactions_total"),
		promotionsShown:   set.NewCounter("anekbot_promotions_shown_total"),
		questionsAnswered: set.NewCounter("anekbot_questions_answered_total"),
	}
	s.llmConcurrencyInUse = set.NewGauge("anekbot_llm_concurrency_in_use", nil)
	s.uniqueUsers = set.NewGauge("anekbot_unique_users", nil)
	return s
}

// RecordAnek attributes a delivered joke to user, and to the totals for source
// ("message"/"inline") and kind ("classic"/"ai"). s may be nil.
func (s *Stats) RecordAnek(user UserID, username, source, kind string) {
	if s == nil {
		return
	}
	s.set.GetOrCreateCounter(labeled("anekbot_aneks_sent_total", "source", source, "kind", kind)).Inc()
	if kind == "ai" {
		s.totalAneksAI.Add(1)
	} else {
		s.totalAneksClassic.Add(1)
	}
	s.recordUser(user, username)
}

func (s *Stats) RecordQuestionAnswered(user UserID, username string) {
	if s == nil {
		return
	}
	s.questionsAnswered.Inc()
	s.recordUser(user, username)
}

func (s *Stats) RecordSwearingReaction() {
	if s == nil {
		return
	}
	s.swearingReactions.Inc()
}

func (s *Stats) RecordPromotionShown() {
	if s == nil {
		return
	}
	s.promotionsShown.Inc()
}

func (s *Stats) ObserveRequest(provider string, ok bool, duration time.Duration) {
	if s == nil {
		return
	}
	status := "error"
	if ok {
		status = "ok"
	}
	s.set.GetOrCreateCounter(labeled("anekbot_llm_requests_total", "provider", provider, "status", status)).Inc()
	s.set.GetOrCreateHistogram(labeled("anekbot_llm_request_duration_seconds", "provider", provider)).Update(duration.Seconds())
}

func (s *Stats) ObserveFallback() {
	if s == nil {
		return
	}
	s.llmFallbackTotal.Inc()
}

func (s *Stats) ObserveRateLimitRejection(scope string) {
	if s == nil {
		return
	}
	s.set.GetOrCreateCounter(labeled("anekbot_rate_limit_rejections_total", "scope", scope)).Inc()
}

func (s *Stats) IncConcurrency() {
	if s != nil {
		s.llmConcurrencyInUse.Inc()
	}
}

func (s *Stats) DecConcurrency() {
	if s != nil {
		s.llmConcurrencyInUse.Dec()
	}
}

func (s *Stats) recordUser(user UserID, username string) {
	if user == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	uc, ok := s.perUser[user]
	if !ok {
		uc = &userCount{}
		s.perUser[user] = uc
		s.uniqueUsers.Set(float64(len(s.perUser)))
	}
	if username != "" {
		uc.username = username
	}
	uc.count++
}

type UserTotal struct {
	UserID   UserID
	Username string
	Count    int64
}

type Snapshot struct {
	TotalAneks   int64
	TotalAIAneks int64
	TotalUsers   int
	TopUsers     []UserTotal
}

// Snapshot returns current totals and the topN users by recorded anek count. s may be nil.
func (s *Stats) Snapshot(topN int) Snapshot {
	if s == nil {
		return Snapshot{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	totals := make([]UserTotal, 0, len(s.perUser))
	for id, uc := range s.perUser {
		totals = append(totals, UserTotal{UserID: id, Username: uc.username, Count: uc.count})
	}
	sort.Slice(totals, func(i, j int) bool {
		if totals[i].Count != totals[j].Count {
			return totals[i].Count > totals[j].Count
		}
		return totals[i].UserID < totals[j].UserID
	})
	if len(totals) > topN {
		totals = totals[:topN]
	}

	return Snapshot{
		TotalAneks:   s.totalAneksClassic.Load() + s.totalAneksAI.Load(),
		TotalAIAneks: s.totalAneksAI.Load(),
		TotalUsers:   len(s.perUser),
		TopUsers:     totals,
	}
}

// Handler serves Prometheus metrics, requiring "Authorization: Bearer <token>" to match
// token via constant-time comparison. token must be non-empty; callers should not mount
// this handler otherwise.
func (s *Stats) Handler(token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validBearerToken(r, token) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		s.set.WritePrometheus(w)
		metrics.WriteGoMetrics(w)
		metrics.WriteProcessMetrics(w)
	})
}

func validBearerToken(r *http.Request, token string) bool {
	const prefix = "Bearer "
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, prefix) {
		return false
	}
	got := auth[len(prefix):]
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

// labeled builds a VictoriaMetrics-style metric name with inline labels, e.g.
// labeled("foo", "a", "1", "b", "2") -> `foo{a="1",b="2"}`. kv must have an even length.
func labeled(name string, kv ...string) string {
	var b strings.Builder
	b.WriteString(name)
	b.WriteByte('{')
	for i := 0; i < len(kv); i += 2 {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%s=%q", kv[i], kv[i+1])
	}
	b.WriteByte('}')
	return b.String()
}
