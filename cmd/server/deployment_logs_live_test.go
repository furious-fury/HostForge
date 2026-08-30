package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/furious-fury/HostForge/internal/models"
	"github.com/furious-fury/HostForge/internal/repository"
	"github.com/gorilla/websocket"
)

func newDeploymentLogTestEndpoint(t *testing.T) (*server, *httptest.Server) {
	t.Helper()
	s := newAPITestServer(t)
	s.cfg.APIToken = "test-management-token"
	s.cfg.SessionCookieName = "hostforge_session"
	s.cfg.SessionSecret = "test-session-secret-long-enough"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/deployments/", s.withRequestContext(s.requireManagementAuth(s.handleDeploymentRoutes)))
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)
	return s, httpServer
}

func createStreamingDeployment(t *testing.T, s *server, content string) (models.Deployment, string, string) {
	t.Helper()
	ctx := context.Background()
	application, err := s.store.CreateApplication(ctx, "Payments", "")
	if err != nil {
		t.Fatal(err)
	}
	environments, err := s.store.ListApplicationEnvironments(ctx, application.ID)
	if err != nil || len(environments) == 0 {
		t.Fatalf("list environments: %v, count=%d", err, len(environments))
	}
	service, err := s.store.CreateService(ctx, repository.CreateServiceInput{ApplicationID: application.ID, Name: "api", RepoURL: "https://github.com/acme/payments.git", InternalPort: 3000})
	if err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "deployment.log")
	if err := os.WriteFile(logPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	deployment, err := s.store.CreateServiceDeployment(ctx, repository.CreateServiceDeploymentInput{ServiceID: service.ID, EnvironmentID: environments[0].ID, LogsPath: logPath, Branch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.store.UpdateDeploymentStatus(ctx, deployment.ID, models.DeploymentBuilding, ""); err != nil {
		t.Fatal(err)
	}
	return deployment, application.ID, logPath
}

func deploymentLogWebSocketURL(serverURL, deploymentID, query string) string {
	return "ws" + strings.TrimPrefix(serverURL, "http") + "/api/deployments/" + deploymentID + "/logs/live?" + query
}

func readLogFrame(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(4 * time.Second))
	var frame map[string]any
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatalf("read WebSocket frame: %v", err)
	}
	return frame
}

func TestDeploymentLogWebSocketRequiresAuthentication(t *testing.T) {
	s, httpServer := newDeploymentLogTestEndpoint(t)
	deployment, _, _ := createStreamingDeployment(t, s, "build output\n")

	conn, response, err := websocket.DefaultDialer.Dial(deploymentLogWebSocketURL(httpServer.URL, deployment.ID, "source=build&format=json"), nil)
	if conn != nil {
		conn.Close()
	}
	if err == nil || response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized WebSocket upgrade, response=%v err=%v", response, err)
	}
}

func TestDeploymentLogWebSocketResumeContextAndTerminalEvent(t *testing.T) {
	s, httpServer := newDeploymentLogTestEndpoint(t)
	deployment, applicationID, logPath := createStreamingDeployment(t, s, "first\nsecond\n")
	header := http.Header{"Authorization": []string{"Bearer test-management-token"}}
	conn, response, err := websocket.DefaultDialer.Dial(deploymentLogWebSocketURL(httpServer.URL, deployment.ID, "source=build&format=json&cursor=6"), header)
	if err != nil {
		t.Fatalf("dial authenticated WebSocket: response=%v err=%v", response, err)
	}
	defer conn.Close()

	hello := readLogFrame(t, conn)
	if hello["t"] != "hello" || hello["application_id"] != applicationID || hello["service_id"] != deployment.ServiceID || hello["environment_id"] != deployment.EnvironmentID || hello["cursor"] != float64(6) {
		t.Fatalf("unexpected hello frame: %#v", hello)
	}
	catchUp := readLogFrame(t, conn)
	if catchUp["t"] != "chunk" || catchUp["d"] != "second\n" || catchUp["end"] != float64(len("first\nsecond\n")) {
		t.Fatalf("unexpected catch-up frame: %#v", catchUp)
	}

	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("final\n"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.store.UpdateDeploymentStatus(context.Background(), deployment.ID, models.DeploymentSuccess, ""); err != nil {
		t.Fatal(err)
	}

	var finalOutput string
	for {
		frame := readLogFrame(t, conn)
		switch frame["t"] {
		case "chunk":
			finalOutput += frame["d"].(string)
		case "end":
			if frame["reason"] != "deployment_terminal" || frame["status"] != models.DeploymentSuccess || frame["eof"] != float64(len("first\nsecond\nfinal\n")) {
				t.Fatalf("unexpected terminal frame: %#v", frame)
			}
			if finalOutput != "final\n" {
				t.Fatalf("final output=%q", finalOutput)
			}
			return
		}
	}
}

func TestDeploymentLogWebSocketResyncsTruncatedCursor(t *testing.T) {
	s, httpServer := newDeploymentLogTestEndpoint(t)
	deployment, _, _ := createStreamingDeployment(t, s, "short\n")
	header := http.Header{"Authorization": []string{"Bearer test-management-token"}}
	conn, response, err := websocket.DefaultDialer.Dial(deploymentLogWebSocketURL(httpServer.URL, deployment.ID, "source=build&format=json&cursor=999"), header)
	if err != nil {
		t.Fatalf("dial authenticated WebSocket: response=%v err=%v", response, err)
	}
	defer conn.Close()

	resync := readLogFrame(t, conn)
	if resync["t"] != "resync" || resync["reason"] != "truncated" || resync["eof"] != float64(len("short\n")) {
		t.Fatalf("unexpected resync frame: %#v", resync)
	}
	hello := readLogFrame(t, conn)
	if hello["t"] != "hello" || hello["cursor"] != float64(0) || hello["resume"] != true {
		t.Fatalf("unexpected hello after resync: %#v", hello)
	}
	chunk := readLogFrame(t, conn)
	if chunk["t"] != "chunk" || chunk["d"] != "short\n" {
		t.Fatalf("unexpected resync catch-up: %#v", chunk)
	}
}

func TestDeploymentStatusTerminal(t *testing.T) {
	for _, status := range []string{models.DeploymentSuccess, models.DeploymentFailed, models.DeploymentCancelled} {
		if !deploymentStatusTerminal(status) {
			t.Fatalf("expected %s to be terminal", status)
		}
	}
	if deploymentStatusTerminal(models.DeploymentBuilding) || deploymentStatusTerminal(models.DeploymentQueued) {
		t.Fatal("active deployment reported as terminal")
	}
}
