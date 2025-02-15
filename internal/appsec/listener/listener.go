// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

// Package listener provides functions and types used to listen to AppSec
// instrumentation events produced by code usintrumented using the functions and
// types found in github.com/nowfred/dd-trace-go/internal/appsec/emitter.
package listener

// ContextKey is used as a key to store operations in the request's context (gRPC/HTTP)
type ContextKey struct{}
