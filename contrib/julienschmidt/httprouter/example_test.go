// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package httprouter_test

import (
	"fmt"
	"log"
	"net/http"

	httptrace "github.com/nowfred/dd-trace-go/contrib/julienschmidt/httprouter"
	"github.com/nowfred/dd-trace-go/ddtrace/ext"
	"github.com/nowfred/dd-trace-go/ddtrace/tracer"

	"github.com/julienschmidt/httprouter"
)

func Index(w http.ResponseWriter, _ *http.Request, _ httprouter.Params) {
	fmt.Fprint(w, "Welcome!\n")
}

func Hello(w http.ResponseWriter, _ *http.Request, ps httprouter.Params) {
	fmt.Fprintf(w, "hello, %s!\n", ps.ByName("name"))
}

func Example() {
	router := httptrace.New()
	router.GET("/", Index)
	router.GET("/hello/:name", Hello)

	log.Fatal(http.ListenAndServe(":8080", router))
}

func Example_withServiceName() {
	router := httptrace.New(httptrace.WithServiceName("http.router"))
	router.GET("/", Index)
	router.GET("/hello/:name", Hello)

	log.Fatal(http.ListenAndServe(":8080", router))
}

func Example_withSpanOpts() {
	router := httptrace.New(
		httptrace.WithServiceName("http.router"),
		httptrace.WithSpanOptions(
			tracer.Tag(ext.SamplingPriority, ext.PriorityUserKeep),
		),
	)

	router.GET("/", Index)
	router.GET("/hello/:name", Hello)

	log.Fatal(http.ListenAndServe(":8080", router))
}
