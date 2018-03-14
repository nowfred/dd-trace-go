package mocktracer

import (
	"strconv"
	"strings"
	"sync"

	"gopkg.in/DataDog/dd-trace-go.v0/ddtrace"
	"gopkg.in/DataDog/dd-trace-go.v0/ddtrace/internal"
	"gopkg.in/DataDog/dd-trace-go.v0/ddtrace/tracer"
)

var _ ddtrace.Tracer = (*mocktracer)(nil)
var _ Tracer = (*mocktracer)(nil)

// Tracer exposes an interface for querying the currently running
// mock tracer.
type Tracer interface {
	// FinishedSpans returns the set of finished spans.
	FinishedSpans() []Span

	// Reset resets the spans and services recorded in the tracer to zero.
	Reset()

	// Stop sets the active tracer to a no-op. The mock tracer becomes
	// inactive.
	Stop()
}

// Start sets the internal tracer to a mock and returns an interface
// which allows querying it. Call Start at the beginning of your tests
// to activate the mock tracer. When your test runs, use the returned
// interface to query your application's behaviour.
func Start() Tracer {
	var t mocktracer
	internal.GlobalTracer = &t
	internal.Testing = true
	return &t
}

type mocktracer struct {
	sync.RWMutex  // guards below spans
	finishedSpans []Span
}

// Stop deactivates the mock tracer and sets the active tracer to a no-op.
func (*mocktracer) Stop() {
	internal.GlobalTracer = &internal.NoopTracer{}
	internal.Testing = false
}

func (t *mocktracer) StartSpan(operationName string, opts ...ddtrace.StartSpanOption) ddtrace.Span {
	var cfg ddtrace.StartSpanConfig
	for _, fn := range opts {
		fn(&cfg)
	}
	return newSpan(t, operationName, &cfg)
}

func (t *mocktracer) FinishedSpans() []Span {
	t.RLock()
	defer t.RUnlock()
	return t.finishedSpans
}

func (t *mocktracer) Reset() {
	t.Lock()
	defer t.Unlock()
	t.finishedSpans = nil
}

func (t *mocktracer) addFinishedSpan(s Span) {
	t.Lock()
	defer t.Unlock()
	if t.finishedSpans == nil {
		t.finishedSpans = make([]Span, 0, 1)
	}
	t.finishedSpans = append(t.finishedSpans, s)
}

const (
	traceHeader   = "x-mock-trace-id"
	spanHeader    = "x-mock-span-id"
	baggagePrefix = "x-baggage-"
)

func (t *mocktracer) Extract(carrier interface{}) (ddtrace.SpanContext, error) {
	reader, ok := carrier.(tracer.TextMapReader)
	if !ok {
		return nil, tracer.ErrInvalidCarrier
	}
	var sc spanContext
	err := reader.ForeachKey(func(key, v string) error {
		k := strings.ToLower(key)
		if k == traceHeader {
			id, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return tracer.ErrSpanContextCorrupted
			}
			sc.traceID = id
		}
		if k == spanHeader {
			id, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return tracer.ErrSpanContextCorrupted
			}
			sc.spanID = id
		}
		if strings.HasPrefix(k, baggagePrefix) {
			sc.setBaggageItem(strings.TrimPrefix(k, baggagePrefix), v)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if sc.traceID == 0 || sc.spanID == 0 {
		return nil, tracer.ErrSpanContextNotFound
	}
	return &sc, err
}

func (t *mocktracer) Inject(context ddtrace.SpanContext, carrier interface{}) error {
	writer, ok := carrier.(tracer.TextMapWriter)
	if !ok {
		return tracer.ErrInvalidCarrier
	}
	ctx, ok := context.(*spanContext)
	if !ok || ctx.traceID == 0 || ctx.spanID == 0 {
		return tracer.ErrInvalidSpanContext
	}
	writer.Set(traceHeader, strconv.FormatUint(ctx.traceID, 10))
	writer.Set(spanHeader, strconv.FormatUint(ctx.spanID, 10))
	ctx.ForeachBaggageItem(func(k, v string) bool {
		writer.Set(baggagePrefix+k, v)
		return true
	})
	return nil
}