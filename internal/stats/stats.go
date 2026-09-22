package stats

import (
	"crypto/subtle"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type UserID int64

type Stats struct {
	registry *prometheus.Registry

	aneksTotal          *prometheus.CounterVec
	llmRequestsTotal    *prometheus.CounterVec
	llmRequestDuration  *prometheus.HistogramVec
	llmFallbackTotal    prometheus.Counter
	rateLimitRejections *prometheus.CounterVec
	swearingReactions   prometheus.Counter
	promotionsShown     prometheus.Counter
	questionsAnswered   prometheus.Counter
	llmConcurrencyInUse prometheus.Gauge
	uniqueUsers         prometheus.Gauge

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
	s := &Stats{
		registry: prometheus.NewRegistry(),
		perUser:  make(map[UserID]*userCount),

		aneksTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "anekbot_aneks_sent_total",
			Help: "Jokes delivered to users, by source and kind.",
		}, []string{"source", "kind"}),
		llmRequestsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "anekbot_llm_requests_total",
			Help: "LLM provider requests, by provider and outcome.",
		}, []string{"provider", "status"}),
		llmRequestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "anekbot_llm_request_duration_seconds",
			Help:    "LLM provider request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"provider"}),
		llmFallbackTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "anekbot_llm_fallback_total",
			Help: "Requests answered by a fallback provider after the primary one failed.",
		}),
		rateLimitRejections: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "anekbot_rate_limit_rejections_total",
			Help: "LLM requests rejected by a rate or concurrency limit, by scope.",
		}, []string{"scope"}),
		swearingReactions: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "anekbot_swearing_reactions_total",
			Help: "Messages the swearing handler reacted to.",
		}),
		promotionsShown: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "anekbot_promotions_shown_total",
			Help: "Promotion buttons attached to a message.",
		}),
		questionsAnswered: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "anekbot_questions_answered_total",
			Help: "@mention questions answered by the LLM.",
		}),
		llmConcurrencyInUse: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "anekbot_llm_concurrency_in_use",
			Help: "LLM requests currently in flight.",
		}),
		uniqueUsers: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "anekbot_unique_users",
			Help: "Distinct users seen since process start.",
		}),
	}

	s.registry.MustRegister(
		s.aneksTotal, s.llmRequestsTotal, s.llmRequestDuration, s.llmFallbackTotal,
		s.rateLimitRejections, s.swearingReactions, s.promotionsShown, s.questionsAnswered,
		s.llmConcurrencyInUse, s.uniqueUsers,
		collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return s
}

// RecordAnek attributes a delivered joke to user, and to the Prometheus totals for source
// ("message"/"inline") and kind ("classic"/"ai"). s may be nil.
func (s *Stats) RecordAnek(user UserID, username, source, kind string) {
	if s == nil {
		return
	}
	s.aneksTotal.WithLabelValues(source, kind).Inc()
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
	s.llmRequestsTotal.WithLabelValues(provider, status).Inc()
	s.llmRequestDuration.WithLabelValues(provider).Observe(duration.Seconds())
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
	s.rateLimitRejections.WithLabelValues(scope).Inc()
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
	metrics := promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !validBearerToken(r, token) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		metrics.ServeHTTP(w, r)
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
