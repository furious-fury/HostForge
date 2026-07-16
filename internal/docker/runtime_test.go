package docker

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/moby/moby/client"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestStopAndRemoveTreatsMissingContainerAsRemoved(t *testing.T) {
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Status:     "404 Not Found",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"message":"No such container: missing"}`)),
			Request:    request,
		}, nil
	})}
	dockerClient, err := client.New(
		client.WithHost("http://docker.test"),
		client.WithHTTPClient(httpClient),
		client.WithVersion("1.47"),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer dockerClient.Close()

	if err := StopAndRemove(context.Background(), dockerClient, "missing"); err != nil {
		t.Fatalf("missing container cleanup should be idempotent: %v", err)
	}
}
