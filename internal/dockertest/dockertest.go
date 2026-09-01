// Package dockertest builds Docker clients backed by a scripted HTTP
// transport, for tests that need to observe or shape Docker API traffic
// without a daemon.
//
// The seam is the transport rather than an interface, so NewClient returns a
// real *client.Client and drops into every signature that already takes one.
// That is what makes it usable from packages like internal/services, which
// thread a concrete client through ~20 functions: testing them needs no
// ContainerRuntime abstraction, only control over the responses.
//
// Named for net/http/httptest. It is a non-test package because test files
// cannot be imported across packages.
package dockertest

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/moby/moby/client"
)

// RoundTripFunc adapts a function to http.RoundTripper.
type RoundTripFunc func(*http.Request) (*http.Response, error)

func (fn RoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

// NewClient returns a Docker client whose every request is served by
// transport. The client is closed when the test finishes.
func NewClient(t *testing.T, transport RoundTripFunc) *client.Client {
	t.Helper()
	dockerClient, err := client.New(
		client.WithHost("http://docker.test"),
		client.WithHTTPClient(&http.Client{Transport: transport}),
		client.WithVersion("1.47"),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dockerClient.Close() })
	return dockerClient
}

// Response builds a response with body for request.
func Response(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status, Status: http.StatusText(status), Header: make(http.Header),
		Body: io.NopCloser(strings.NewReader(body)), Request: request,
	}
}
