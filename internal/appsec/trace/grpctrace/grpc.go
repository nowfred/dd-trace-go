// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package grpctrace

import (
	"github.com/nowfred/dd-trace-go/ddtrace"
	"github.com/nowfred/dd-trace-go/internal/appsec/trace"
	"github.com/nowfred/dd-trace-go/internal/appsec/trace/httptrace"
	"github.com/nowfred/dd-trace-go/internal/log"
)

// SetSecurityEventsTags sets the AppSec events span tags.
func SetSecurityEventsTags(span ddtrace.Span, events []any) {
	if err := setSecurityEventsTags(span, events); err != nil {
		log.Error("appsec: unexpected error while creating the appsec events tags: %v", err)
	}
}

func setSecurityEventsTags(span ddtrace.Span, events []any) error {
	if events == nil {
		return nil
	}
	return trace.SetEventSpanTags(span, events)
}

// SetRequestMetadataTags sets the gRPC request metadata span tags.
func SetRequestMetadataTags(span ddtrace.Span, md map[string][]string) {
	for h, v := range httptrace.NormalizeHTTPHeaders(md) {
		span.SetTag("grpc.metadata."+h, v)
	}
}
