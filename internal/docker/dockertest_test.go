package docker

import (
	"net/http"
	"testing"

	"github.com/furious-fury/HostForge/internal/dockertest"
	"github.com/moby/moby/client"
)

// These helpers were defined in this package's test files until
// internal/dockertest was extracted so other packages could use them. They
// stay as aliases so the existing call sites here read unchanged.

type roundTripFunc = dockertest.RoundTripFunc

func newDockerHTTPTestClient(t *testing.T, transport roundTripFunc) *client.Client {
	t.Helper()
	return dockertest.NewClient(t, transport)
}

func dockerResponse(request *http.Request, status int, body string) *http.Response {
	return dockertest.Response(request, status, body)
}
