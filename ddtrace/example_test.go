// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package ddtrace_test

import (
	"fmt"
	"log"
	"os"

	"github.com/nowfred/dd-trace-go/ddtrace"
	"github.com/nowfred/dd-trace-go/ddtrace/ext"
	"github.com/nowfred/dd-trace-go/ddtrace/mocktracer"
	"github.com/nowfred/dd-trace-go/ddtrace/opentracer"
	"github.com/nowfred/dd-trace-go/ddtrace/tracer"

	opentracing "github.com/opentracing/opentracing-go"
)

// The below example illustrates a simple use case using the "tracer" package,
// our native Datadog APM tracing client integration. For thorough documentation
// and further examples, visit its own godoc page.
func Example_datadog() {
	// Start the tracer and defer the Stop method.
	tracer.Start(tracer.WithAgentAddr("host:port"))
	defer tracer.Stop()

	// Start a root span.
	span := tracer.StartSpan("get.data")
	defer span.Finish()

	// Create a child of it, computing the time needed to read a file.
	child := tracer.StartSpan("read.file", tracer.ChildOf(span.Context()))
	child.SetTag(ext.ResourceName, "test.json")

	// If you are using 128 bit trace ids and want to generate the high
	// order bits, cast the span's context to ddtrace.SpanContextW3C.
	// See Issue #1677
	if w3Cctx, ok := child.Context().(ddtrace.SpanContextW3C); ok {
		fmt.Printf("128 bit trace id = %s\n", w3Cctx.TraceID128())
	}

	// Perform an operation.
	_, err := os.ReadFile("~/test.json")

	// We may finish the child span using the returned error. If it's
	// nil, it will be disregarded.
	child.Finish(tracer.WithError(err))
	if err != nil {
		log.Fatal(err)
	}
}

// The below example illustrates how to set up an opentracing.Tracer using Datadog's
// tracer.
func Example_opentracing() {
	// Start a Datadog tracer, optionally providing a set of options,
	// returning an opentracing.Tracer which wraps it.
	t := opentracer.New(tracer.WithAgentAddr("host:port"))
	defer tracer.Stop() // important for data integrity (flushes any leftovers)

	// Use it with the Opentracing API. The (already started) Datadog tracer
	// may be used in parallel with the Opentracing API if desired.
	opentracing.SetGlobalTracer(t)
}

// The code below illustrates a scenario of how one could use a mock tracer in tests
// to assert that spans are created correctly.
func Example_mocking() {
	// Setup the test environment: start the mock tracer.
	mt := mocktracer.Start()
	defer mt.Stop()

	// Run test code: in this example we will simply create a span to illustrate.
	tracer.StartSpan("test.span").Finish()

	// Assert the results: query the mock tracer for finished spans.
	spans := mt.FinishedSpans()
	if len(spans) != 1 {
		// fail
		panic("expected 1 span")
	}
	if spans[0].OperationName() != "test.span" {
		// fail
		panic("unexpected operation name")
	}
}
