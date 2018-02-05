// Package redigo provides functions to trace the garyburd/redigo package (https://github.com/garyburd/redigo).
package redigo

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"

	redis "github.com/garyburd/redigo/redis"

	"github.com/DataDog/dd-trace-go/tracer"
	"github.com/DataDog/dd-trace-go/tracer/ext"
)

// Conn is an implementation of the redis.Conn interface that supports tracing
type Conn struct {
	redis.Conn
	*params
}

// params contains fields and metadata useful for command tracing
type params struct {
	config  *dialConfig
	network string
	host    string
	port    string
}

// parseOptions parses a set of arbitrary options (which can be of type redis.DialOption
// or the local DialOption) and returns the corresponding redis.DialOption set as well as
// a configured dialConfig.
func parseOptions(options ...interface{}) ([]redis.DialOption, *dialConfig) {
	dialOpts := []redis.DialOption{}
	cfg := new(dialConfig)
	defaults(cfg)
	for _, opt := range options {
		switch o := opt.(type) {
		case redis.DialOption:
			dialOpts = append(dialOpts, o)
		case DialOption:
			o(cfg)
		}
	}
	return dialOpts, cfg
}

// Dial dials into the network address and returns a traced redis.Conn.
// The set of supported options must be either of type redis.DialOption or this package's DialOption.
func Dial(network, address string, options ...interface{}) (redis.Conn, error) {
	dialOpts, cfg := parseOptions(options...)
	c, err := redis.Dial(network, address, dialOpts...)
	if err != nil {
		return nil, err
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	cfg.tracer.SetServiceInfo(cfg.serviceName, "redis", ext.AppTypeDB)
	tc := Conn{c, &params{cfg, network, host, port}}
	return tc, nil
}

// DialURL connects to a Redis server at the given URL using the Redis
// URI scheme. URLs should follow the draft IANA specification for the
// scheme (https://www.iana.org/assignments/uri-schemes/prov/redis).
// The returned redis.Conn is traced.
func DialURL(rawurl string, options ...interface{}) (redis.Conn, error) {
	dialOpts, cfg := parseOptions(options...)
	u, err := url.Parse(rawurl)
	if err != nil {
		return Conn{}, err
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		host = u.Host
		port = "6379"
	}
	if host == "" {
		host = "localhost"
	}
	network := "tcp"
	c, err := redis.DialURL(rawurl, dialOpts...)
	tc := Conn{c, &params{cfg, network, host, port}}
	return tc, err
}

// newChildSpan creates a span inheriting from the given context. It adds to the span useful metadata about the traced Redis connection
func (tc Conn) newChildSpan(ctx context.Context) *tracer.Span {
	p := tc.params
	span := p.config.tracer.NewChildSpanFromContext("redis.command", ctx)
	span.Service = p.config.serviceName
	span.SetMeta("out.network", p.network)
	span.SetMeta("out.port", p.port)
	span.SetMeta("out.host", p.host)
	return span
}

// Do wraps redis.Conn.Do. It sends a command to the Redis server and returns the received reply.
// In the process it emits a span containing key information about the command sent.
// When passed a context.Context as the final argument, Do will ensure that any span created
// inherits from this context. The rest of the arguments are passed through to the Redis server unchanged.
func (tc Conn) Do(commandName string, args ...interface{}) (reply interface{}, err error) {
	var (
		ctx context.Context
		ok  bool
	)
	if n := len(args); n > 0 {
		ctx, ok = args[n-1].(context.Context)
		if ok {
			args = args[:n-1]
		}
	}

	span := tc.newChildSpan(ctx)
	defer func() {
		if err != nil {
			span.SetError(err)
		}
		span.Finish()
	}()

	span.SetMeta("redis.args_length", strconv.Itoa(len(args)))

	if len(commandName) > 0 {
		span.Resource = commandName
	} else {
		// When the command argument to the Do method is "", then the Do method will flush the output buffer
		// See https://godoc.org/github.com/garyburd/redigo/redis#hdr-Pipelining
		span.Resource = "redigo.Conn.Flush"
	}
	var b bytes.Buffer
	b.WriteString(commandName)
	for _, arg := range args {
		b.WriteString(" ")
		switch arg := arg.(type) {
		case string:
			b.WriteString(arg)
		case int:
			b.WriteString(strconv.Itoa(arg))
		case int32:
			b.WriteString(strconv.FormatInt(int64(arg), 10))
		case int64:
			b.WriteString(strconv.FormatInt(arg, 10))
		case fmt.Stringer:
			b.WriteString(arg.String())
		}
	}
	span.SetMeta("redis.raw_command", b.String())
	return tc.Conn.Do(commandName, args...)
}