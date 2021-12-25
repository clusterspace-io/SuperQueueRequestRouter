package main

import "github.com/prometheus/client_golang/prometheus"

var (
	TotalRequestsCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "total_reqs",
		Help: "Total number of http requests",
	})

	HTTPResponsesMetric = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_responses",
		Help: "Total number of http requests",
	}, []string{"code", "endpoint"})

	HTTPLatenciesMetric = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_latency",
		Help:    "Latency of HTTP responses in milliseconds",
		Buckets: prometheus.ExponentialBuckets(1, 3, 10),
	}, []string{"code", "endpoint"})
)

func SetupMetrics() {
	prometheus.Register(HTTPResponsesMetric)
	prometheus.Register(HTTPLatenciesMetric)
	prometheus.Register(TotalRequestsCounter)
}
