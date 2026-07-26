package utils

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	DispatchDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "pool_dispatch_duration_seconds",
		Help:    "dispatch account duration seconds",
		Buckets: []float64{0.005, 0.01, 0.05, 0.1, 0.5, 1, 2, 5},
	})

	RateLimitHits = promauto.NewCounter(prometheus.CounterOpts{
		Name: "pool_ratelimit_hits_total",
		Help: "rate limit rejected request count",
	})
)
