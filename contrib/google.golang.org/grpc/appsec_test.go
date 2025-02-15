// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"

	pappsec "github.com/nowfred/dd-trace-go/appsec"
	"github.com/nowfred/dd-trace-go/ddtrace/mocktracer"
	"github.com/nowfred/dd-trace-go/internal/appsec"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestAppSec(t *testing.T) {
	appsec.Start()
	defer appsec.Stop()
	if !appsec.Enabled() {
		t.Skip("appsec disabled")
	}

	setup := func() (FixtureClient, mocktracer.Tracer, func()) {
		rig, err := newRig(false)
		require.NoError(t, err)

		mt := mocktracer.Start()

		return rig.client, mt, func() {
			rig.Close()
			mt.Stop()
		}
	}

	t.Run("unary", func(t *testing.T) {
		client, mt, cleanup := setup()
		defer cleanup()

		// Send a XSS attack in the payload along with the canary value in the RPC metadata
		ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("dd-canary", "dd-test-scanner-log"))
		res, err := client.Ping(ctx, &FixtureRequest{Name: "<script>evilJSCode;</script>"})
		// Check that the handler was properly called
		require.NoError(t, err)
		require.Equal(t, "passed", res.Message)

		finished := mt.FinishedSpans()
		require.Len(t, finished, 1)

		// The request should have the attack attempts
		event, _ := finished[0].Tag("_dd.appsec.json").(string)
		require.NotNil(t, event)
		require.True(t, strings.Contains(event, "crs-941-110")) // XSS attack attempt
		require.True(t, strings.Contains(event, "ua0-600-55x")) // canary rule attack attempt
	})

	t.Run("stream", func(t *testing.T) {
		client, mt, cleanup := setup()
		defer cleanup()

		// Send a XSS attack in the payload along with the canary value in the RPC metadata
		ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("dd-canary", "dd-test-scanner-log"))
		stream, err := client.StreamPing(ctx)
		require.NoError(t, err)

		// Send a XSS attack
		err = stream.Send(&FixtureRequest{Name: "<script>evilJSCode;</script>"})
		require.NoError(t, err)

		// Check that the handler was properly called
		res, err := stream.Recv()
		require.Equal(t, "passed", res.Message)
		require.NoError(t, err)

		for i := 0; i < 5; i++ { // Fire multiple times, each time should result in a detected event
			// Send a SQLi attack
			err = stream.Send(&FixtureRequest{Name: fmt.Sprintf("-%[1]d' and %[1]d=%[1]d union select * from users--", i)})
			require.NoError(t, err)

			// Check that the handler was properly called
			res, err = stream.Recv()
			require.Equal(t, "passed", res.Message)
			require.NoError(t, err)
		}

		err = stream.CloseSend()
		require.NoError(t, err)
		// to flush the spans
		stream.Recv()

		finished := mt.FinishedSpans()
		require.Len(t, finished, 14)

		// The request should have the attack attempts
		event := finished[len(finished)-1].Tag("_dd.appsec.json")
		require.NotNil(t, event, "the _dd.appsec.json tag was not found")

		jsonText := event.(string)
		type trigger struct {
			Rule struct {
				ID string `json:"id"`
			} `json:"rule"`
		}
		var parsed struct {
			Triggers []trigger `json:"triggers"`
		}
		err = json.Unmarshal([]byte(jsonText), &parsed)
		require.NoError(t, err)

		histogram := map[string]uint8{}
		for _, tr := range parsed.Triggers {
			histogram[tr.Rule.ID]++
		}

		require.EqualValues(t, 1, histogram["crs-941-110"]) // XSS attack attempt
		require.EqualValues(t, 5, histogram["crs-942-270"]) // SQL-injection attack attempt
		require.EqualValues(t, 1, histogram["ua0-600-55x"]) // canary rule attack attempt

		require.Len(t, histogram, 3)
	})
}

// Test that http blocking works by using custom rules/rules data
func TestBlocking(t *testing.T) {
	t.Setenv("DD_APPSEC_RULES", "../../../internal/appsec/testdata/blocking.json")
	appsec.Start()
	defer appsec.Stop()
	if !appsec.Enabled() {
		t.Skip("appsec disabled")
	}

	setup := func() (FixtureClient, mocktracer.Tracer, func()) {
		rig, err := newRig(false)
		require.NoError(t, err)

		mt := mocktracer.Start()

		return rig.client, mt, func() {
			rig.Close()
			mt.Stop()
		}
	}

	t.Run("unary-block", func(t *testing.T) {
		client, mt, cleanup := setup()
		defer cleanup()

		// Send a XSS attack in the payload along with the canary value in the RPC metadata
		ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("dd-canary", "dd-test-scanner-log", "x-client-ip", "1.2.3.4"))
		reply, err := client.Ping(ctx, &FixtureRequest{Name: "<script>alert('xss');</script>"})

		require.Nil(t, reply)
		require.Equal(t, codes.Aborted, status.Code(err))

		finished := mt.FinishedSpans()
		require.Len(t, finished, 1)
		// The request should have the attack attempts
		event, _ := finished[0].Tag("_dd.appsec.json").(string)
		require.NotNil(t, event)
		require.True(t, strings.Contains(event, "blk-001-001"))
	})

	t.Run("unary-no-block", func(t *testing.T) {
		client, _, cleanup := setup()
		defer cleanup()

		// Send a XSS attack in the payload along with the canary value in the RPC metadata
		ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("dd-canary", "dd-test-scanner-log", "x-client-ip", "1.2.3.5"))
		reply, err := client.Ping(ctx, &FixtureRequest{Name: "<script>alert('xss');</script>"})

		require.Equal(t, "passed", reply.Message)
		require.Equal(t, codes.OK, status.Code(err))
	})

	t.Run("stream-block", func(t *testing.T) {
		client, mt, cleanup := setup()
		defer cleanup()

		ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("dd-canary", "dd-test-scanner-log", "x-client-ip", "1.2.3.4"))
		stream, err := client.StreamPing(ctx)
		require.NoError(t, err)
		reply, err := stream.Recv()

		require.Equal(t, codes.Aborted, status.Code(err))
		require.Nil(t, reply)

		finished := mt.FinishedSpans()
		require.Len(t, finished, 1)
		// The request should have the attack attempts
		event, _ := finished[0].Tag("_dd.appsec.json").(string)
		require.NotNil(t, event)
		require.True(t, strings.Contains(event, "blk-001-001"))
	})

	t.Run("stream-no-block", func(t *testing.T) {
		client, _, cleanup := setup()
		defer cleanup()

		ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("dd-canary", "dd-test-scanner-log", "x-client-ip", "1.2.3.5"))
		stream, err := client.StreamPing(ctx)
		require.NoError(t, err)

		// Send a XSS attack
		err = stream.Send(&FixtureRequest{Name: "<script>alert('xss');</script>"})
		require.NoError(t, err)
		reply, err := stream.Recv()
		require.Equal(t, codes.OK, status.Code(err))
		require.Equal(t, "passed", reply.Message)

		err = stream.CloseSend()
		require.NoError(t, err)
	})

}

// Test that user blocking works by using custom rules/rules data
func TestUserBlocking(t *testing.T) {
	t.Setenv("DD_APPSEC_RULES", "../../../internal/appsec/testdata/blocking.json")
	appsec.Start()
	defer appsec.Stop()
	if !appsec.Enabled() {
		t.Skip("appsec disabled")
	}

	setup := func() (FixtureClient, mocktracer.Tracer, func()) {
		rig, err := newAppsecRig(false)
		require.NoError(t, err)

		mt := mocktracer.Start()

		return rig.client, mt, func() {
			rig.Close()
			mt.Stop()
		}
	}

	t.Run("unary-block", func(t *testing.T) {
		client, mt, cleanup := setup()
		defer cleanup()

		// Send a XSS attack in the payload along with the canary value in the RPC metadata
		ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("user-id", "blocked-user-1"))
		reply, err := client.Ping(ctx, &FixtureRequest{Name: "<script>alert('xss');</script>"})

		require.Nil(t, reply)
		require.Equal(t, codes.Aborted, status.Code(err))

		finished := mt.FinishedSpans()
		require.Len(t, finished, 1)
		// The request should have the XSS and user ID attack attempts
		event, _ := finished[0].Tag("_dd.appsec.json").(string)
		require.NotNil(t, event)
		require.True(t, strings.Contains(event, "blk-001-002"))
		require.True(t, strings.Contains(event, "crs-941-110"))
	})

	t.Run("unary-no-block", func(t *testing.T) {
		client, _, cleanup := setup()
		defer cleanup()

		ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("user-id", "legit user"))
		reply, err := client.Ping(ctx, &FixtureRequest{Name: "<script>alert('xss');</script>"})

		require.Equal(t, "passed", reply.Message)
		require.Equal(t, codes.OK, status.Code(err))
	})

	// This test checks that IP blocking happens BEFORE user blocking, since user blocking needs the request handler
	// to be invoked while IP blocking doesn't
	t.Run("unary-mixed-block", func(t *testing.T) {
		client, mt, cleanup := setup()
		defer cleanup()

		ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("user-id", "blocked-user-1", "x-forwarded-for", "1.2.3.4"))
		reply, err := client.Ping(ctx, &FixtureRequest{})

		require.Nil(t, reply)
		require.Equal(t, codes.Aborted, status.Code(err))

		finished := mt.FinishedSpans()
		require.Len(t, finished, 1)
		event, _ := finished[0].Tag("_dd.appsec.json").(string)
		require.NotNil(t, event)
		require.True(t, strings.Contains(event, "blk-001-001"))
	})

	t.Run("stream-block", func(t *testing.T) {
		client, mt, cleanup := setup()
		defer cleanup()

		ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("user-id", "blocked-user-1"))
		stream, err := client.StreamPing(ctx)
		require.NoError(t, err)
		reply, err := stream.Recv()

		require.Equal(t, codes.Aborted, status.Code(err))
		require.Nil(t, reply)

		finished := mt.FinishedSpans()
		require.Len(t, finished, 1)
		// The request should have the attack attempts
		event, _ := finished[0].Tag("_dd.appsec.json").(string)
		require.NotNil(t, event)
		require.True(t, strings.Contains(event, "blk-001-002"))
	})

	t.Run("stream-no-block", func(t *testing.T) {
		client, _, cleanup := setup()
		defer cleanup()

		ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("user-id", "legit user"))
		stream, err := client.StreamPing(ctx)
		require.NoError(t, err)

		// Send a XSS attack
		err = stream.Send(&FixtureRequest{Name: "<script>alert('xss');</script>"})
		require.NoError(t, err)
		reply, err := stream.Recv()
		require.Equal(t, codes.OK, status.Code(err))
		require.Equal(t, "passed", reply.Message)

		err = stream.CloseSend()
		require.NoError(t, err)
	})
	// This test checks that IP blocking happens BEFORE user blocking, since user blocking needs the request handler
	// to be invoked while IP blocking doesn't
	t.Run("stream-mixed-block", func(t *testing.T) {
		client, mt, cleanup := setup()
		defer cleanup()

		ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("user-id", "blocked-user-1", "x-forwarded-for", "1.2.3.4"))
		stream, err := client.StreamPing(ctx)
		require.NoError(t, err)
		reply, err := stream.Recv()

		require.Equal(t, codes.Aborted, status.Code(err))
		require.Nil(t, reply)

		finished := mt.FinishedSpans()
		require.Len(t, finished, 1)
		// The request should have IP related the attack attempts
		event, _ := finished[0].Tag("_dd.appsec.json").(string)
		require.NotNil(t, event)
		require.True(t, strings.Contains(event, "blk-001-001"))
	})
}

func newAppsecRig(traceClient bool, interceptorOpts ...Option) (*appsecRig, error) {
	interceptorOpts = append([]InterceptorOption{WithServiceName("grpc")}, interceptorOpts...)

	server := grpc.NewServer(
		grpc.UnaryInterceptor(UnaryServerInterceptor(interceptorOpts...)),
		grpc.StreamInterceptor(StreamServerInterceptor(interceptorOpts...)),
	)

	fixtureServer := new(appsecFixtureServer)
	RegisterFixtureServer(server, fixtureServer)

	li, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	_, port, _ := net.SplitHostPort(li.Addr().String())
	// start our test fixtureServer.
	go server.Serve(li)

	opts := []grpc.DialOption{grpc.WithInsecure()}
	if traceClient {
		opts = append(opts,
			grpc.WithUnaryInterceptor(UnaryClientInterceptor(interceptorOpts...)),
			grpc.WithStreamInterceptor(StreamClientInterceptor(interceptorOpts...)),
		)
	}
	conn, err := grpc.Dial(li.Addr().String(), opts...)
	if err != nil {
		return nil, fmt.Errorf("error dialing: %s", err)
	}
	return &appsecRig{
		fixtureServer: fixtureServer,
		listener:      li,
		port:          port,
		server:        server,
		conn:          conn,
		client:        NewFixtureClient(conn),
	}, err
}

// rig contains all of the servers and connections we'd need for a
// grpc integration test
type appsecRig struct {
	fixtureServer *appsecFixtureServer
	server        *grpc.Server
	port          string
	listener      net.Listener
	conn          *grpc.ClientConn
	client        FixtureClient
}

func (r *appsecRig) Close() {
	r.server.Stop()
	r.conn.Close()
}

type appsecFixtureServer struct {
	UnimplementedFixtureServer
	s fixtureServer
}

func (s *appsecFixtureServer) StreamPing(stream Fixture_StreamPingServer) (err error) {
	ctx := stream.Context()
	md, _ := metadata.FromIncomingContext(ctx)
	ids := md.Get("user-id")
	if err := pappsec.SetUser(ctx, ids[0]); err != nil {
		return err
	}
	return s.s.StreamPing(stream)
}
func (s *appsecFixtureServer) Ping(ctx context.Context, in *FixtureRequest) (*FixtureReply, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	ids := md.Get("user-id")
	if err := pappsec.SetUser(ctx, ids[0]); err != nil {
		return nil, err
	}

	return s.s.Ping(ctx, in)
}
