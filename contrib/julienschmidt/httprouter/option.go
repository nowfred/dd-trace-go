// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package httprouter

import (
	"math"

	"github.com/nowfred/dd-trace-go/ddtrace"
	"github.com/nowfred/dd-trace-go/internal"
	"github.com/nowfred/dd-trace-go/internal/globalconfig"
	"github.com/nowfred/dd-trace-go/internal/namingschema"
	"github.com/nowfred/dd-trace-go/internal/normalizer"
)

const defaultServiceName = "http.router"

type routerConfig struct {
	serviceName   string
	spanOpts      []ddtrace.StartSpanOption
	analyticsRate float64
	headerTags    *internal.LockMap
}

// RouterOption represents an option that can be passed to New.
type RouterOption func(*routerConfig)

func defaults(cfg *routerConfig) {
	if internal.BoolEnv("DD_TRACE_HTTPROUTER_ANALYTICS_ENABLED", false) {
		cfg.analyticsRate = 1.0
	} else {
		cfg.analyticsRate = globalconfig.AnalyticsRate()
	}
	cfg.serviceName = namingschema.ServiceName(defaultServiceName)
	cfg.headerTags = globalconfig.HeaderTagMap()
}

// WithServiceName sets the given service name for the returned router.
func WithServiceName(name string) RouterOption {
	return func(cfg *routerConfig) {
		cfg.serviceName = name
	}
}

// WithSpanOptions applies the given set of options to the span started by the router.
func WithSpanOptions(opts ...ddtrace.StartSpanOption) RouterOption {
	return func(cfg *routerConfig) {
		cfg.spanOpts = opts
	}
}

// WithAnalytics enables Trace Analytics for all started spans.
func WithAnalytics(on bool) RouterOption {
	return func(cfg *routerConfig) {
		if on {
			cfg.analyticsRate = 1.0
		} else {
			cfg.analyticsRate = math.NaN()
		}
	}
}

// WithAnalyticsRate sets the sampling rate for Trace Analytics events
// correlated to started spans.
func WithAnalyticsRate(rate float64) RouterOption {
	return func(cfg *routerConfig) {
		if rate >= 0.0 && rate <= 1.0 {
			cfg.analyticsRate = rate
		} else {
			cfg.analyticsRate = math.NaN()
		}
	}
}

// WithHeaderTags enables the integration to attach HTTP request headers as span tags.
// Warning:
// Using this feature can risk exposing sensitive data such as authorization tokens to Datadog.
// Special headers can not be sub-selected. E.g., an entire Cookie header would be transmitted, without the ability to choose specific Cookies.
func WithHeaderTags(headers []string) RouterOption {
	headerTagsMap := normalizer.HeaderTagSlice(headers)
	return func(cfg *routerConfig) {
		cfg.headerTags = internal.NewLockMap(headerTagsMap)
	}
}
