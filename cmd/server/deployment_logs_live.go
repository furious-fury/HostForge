package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hostforge/hostforge/internal/deploylogs"
	"github.com/hostforge/hostforge/internal/docker"
	logsapi "github.com/hostforge/hostforge/internal/logs"
	"github.com/hostforge/hostforge/internal/models"
)

const (
	deploymentLogAppHeartbeatInterval = 20 * time.Second
	deploymentLogMaxJSONChunkBytes    = 48 * 1024
)

func parseQueryInt64(r *http.Request, key string, def int64) int64 {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return def
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return def
	}
	if v < 0 {
		return 0
	}
	return v
}

func defaultLogSource(deployment models.Deployment) string {
	if deployment.Status == models.DeploymentSuccess {
		return "container"
	}
	return "build"
}

// wsLogSink serializes WebSocket text writes and control pings (gorilla/websocket is not safe for concurrent writers).
type wsLogSink struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (w *wsLogSink) writeText(b []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteMessage(websocket.TextMessage, b)
}

func (w *wsLogSink) writeJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.conn.WriteMessage(websocket.TextMessage, b)
}

func (w *wsLogSink) ping() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	deadline := time.Now().Add(8 * time.Second)
	return w.conn.WriteControl(websocket.PingMessage, nil, deadline)
}

func (w *wsLogSink) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if err := w.writeText(p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// startDeploymentLogKeepalive sends periodic WebSocket pings so reverse proxies and dev proxies
// do not treat idle log periods (no new build output) as dead connections.
func startDeploymentLogKeepalive(ctx context.Context, log *slog.Logger, sink *wsLogSink, cancel context.CancelFunc, deploymentID, source string) {
	t := time.NewTicker(15 * time.Second)
	go func() {
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := sink.ping(); err != nil {
					if log != nil {
						log.Warn("deployment log ws ping failed", "deployment_id", deploymentID, "source", source, "err", err)
					}
					cancel()
					return
				}
			}
		}
	}()
}

func startDeploymentLogAppHeartbeat(ctx context.Context, sink *wsLogSink, cancel context.CancelFunc) {
	t := time.NewTicker(deploymentLogAppHeartbeatInterval)
	go func() {
		defer t.Stop()
		seq := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				seq++
				if err := sink.writeJSON(map[string]any{
					"t":   deploylogs.TypeHeartbeat,
					"seq": seq,
				}); err != nil {
					cancel()
					return
				}
			}
		}
	}()
}

func (s *server) handleDeploymentLogsLive(w http.ResponseWriter, r *http.Request, deploymentID string) {
	reqLog := s.requestLog(r)
	remoteIP := requestIP(r)

	deployment, err := s.store.GetDeploymentByID(r.Context(), deploymentID)
	if err != nil {
		if errorsIsNoRows(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"status": "error", "error": "deployment_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"status": "error", "error": "deployment_lookup_failed"})
		return
	}

	conn, err := logUpgrader.Upgrade(w, r, nil)
	if err != nil {
		reqLog.Warn("deployment log ws upgrade failed", "deployment_id", deploymentID, "remote_ip", remoteIP, "err", err)
		return
	}
	defer conn.Close()

	source := strings.TrimSpace(r.URL.Query().Get("source"))
	if source == "" {
		source = defaultLogSource(deployment)
	}
	resumeCursor := parseQueryInt64(r, "cursor", 0)

	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "raw" {
		sink := &wsLogSink{conn: conn}
		_ = sink.writeJSON(map[string]any{
			"t":    deploylogs.TypeError,
			"code": "unsupported_format",
			"msg":  "supported formats: json, raw",
		})
		return
	}

	sessionStart := time.Now()
	defer func() {
		reqLog.Info(
			"deployment log ws session ended",
			"deployment_id", deploymentID,
			"source", source,
			"format", format,
			"duration_ms", time.Since(sessionStart).Milliseconds(),
		)
	}()

	reqLog.Info("deployment log ws opened", "deployment_id", deploymentID, "source", source, "remote_ip", remoteIP, "format", format, "cursor", resumeCursor)

	if format == "raw" {
		s.handleDeploymentLogsLiveRaw(r, conn, reqLog, deploymentID, source, deployment, resumeCursor)
		return
	}

	// Intentionally no SetReadDeadline / pong handler: many dev/HTTP proxies (incl. Vite's
	// http-proxy) do not reliably forward WebSocket control frames. We rely on write-side
	// failures via the keepalive ping to tear down dead connections instead.

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				reqLog.Debug("deployment log ws read ended", "deployment_id", deploymentID, "source", source, "err", err)
				cancel()
				return
			}
		}
	}()

	sink := &wsLogSink{conn: conn}
	startDeploymentLogKeepalive(ctx, reqLog, sink, cancel, deploymentID, source)
	startDeploymentLogAppHeartbeat(ctx, sink, cancel)

	switch source {
	case "build":
		s.streamBuildLogJSON(ctx, reqLog, sink, deployment, resumeCursor)
	case "container":
		s.streamContainerLogJSON(ctx, reqLog, sink, deploymentID)
	default:
		_ = sink.writeJSON(map[string]any{
			"t":    deploylogs.TypeError,
			"code": "unsupported_source",
			"msg":  "source must be build or container",
		})
	}
}

// handleDeploymentLogsLiveRaw streams plain text for compatibility (curl, older clients).
func (s *server) handleDeploymentLogsLiveRaw(r *http.Request, conn *websocket.Conn, reqLog *slog.Logger, deploymentID, source string, deployment models.Deployment, _ int64) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				cancel()
				return
			}
		}
	}()
	sink := &wsLogSink{conn: conn}
	startDeploymentLogKeepalive(ctx, reqLog, sink, cancel, deploymentID, source)
	switch source {
	case "build":
		s.streamBuildLogRaw(ctx, sink, deployment)
	case "container":
		s.streamContainerLogRaw(ctx, sink, deploymentID)
	default:
		_ = sink.writeText([]byte("error: unsupported source"))
	}
}

func (s *server) streamBuildLogRaw(ctx context.Context, sink *wsLogSink, deployment models.Deployment) {
	if strings.TrimSpace(deployment.LogsPath) == "" {
		_ = sink.writeText([]byte("error: deployment log path is empty"))
		return
	}
	initial, err := logsapi.TailFile(deployment.LogsPath, logsapi.DefaultTailBytes, 0)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = sink.writeText([]byte("error: failed to tail deployment log"))
		return
	}
	if len(initial) > 0 {
		_ = sink.writeText(initial)
	}
	if err := logsapi.FollowFile(ctx, deployment.LogsPath, 500*time.Millisecond, func(chunk []byte) error {
		if len(chunk) == 0 {
			return nil
		}
		return sink.writeText(chunk)
	}); err != nil && !errors.Is(err, context.Canceled) {
		_ = sink.writeText([]byte("error: build log stream ended"))
	}
}

func (s *server) streamContainerLogRaw(ctx context.Context, sink *wsLogSink, deploymentID string) {
	containerRec, err := s.store.GetContainerByDeploymentID(ctx, deploymentID)
	if err != nil {
		_ = sink.writeText([]byte("error: container record not found for deployment"))
		return
	}
	cli, err := docker.NewClient(ctx)
	if err != nil {
		_ = sink.writeText([]byte("error: docker unavailable"))
		return
	}
	defer cli.Close()
	if err := docker.StreamContainerLogs(ctx, cli, containerRec.DockerContainerID, docker.LogStreamOptions{
		Follow:     true,
		Tail:       "200",
		ShowStdout: true,
		ShowStderr: true,
	}, sink); err != nil && !errors.Is(err, context.Canceled) {
		_ = sink.writeText([]byte("error: container log stream ended"))
	}
}

func (s *server) streamBuildLogJSON(ctx context.Context, log *slog.Logger, sink *wsLogSink, deployment models.Deployment, resumeCursor int64) {
	path := strings.TrimSpace(deployment.LogsPath)
	if path == "" {
		_ = sink.writeJSON(map[string]any{"t": deploylogs.TypeError, "code": "log_path_empty", "msg": "deployment log path is empty"})
		return
	}

	st, statErr := os.Stat(path)
	eof := int64(0)
	if statErr != nil {
		if !errors.Is(statErr, os.ErrNotExist) {
			_ = sink.writeJSON(map[string]any{"t": deploylogs.TypeError, "code": "stat_failed", "msg": "failed to stat deployment log"})
			log.Warn("deployment build log stat failed", "deployment_id", deployment.ID, "err", statErr)
			return
		}
	} else {
		eof = st.Size()
	}

	cursor := resumeCursor
	if cursor > eof {
		_ = sink.writeJSON(map[string]any{"t": deploylogs.TypeResync, "reason": "truncated", "eof": eof})
		cursor = 0
		eof = 0
		if st2, e2 := os.Stat(path); e2 == nil {
			eof = st2.Size()
		}
	}

	_ = sink.writeJSON(map[string]any{
		"t":             deploylogs.TypeHello,
		"v":             deploylogs.ProtocolVersion,
		"source":        "build",
		"resume":        true,
		"eof":           eof,
		"cursor":        cursor,
		"deployment_id": deployment.ID,
	})

	if cursor < eof {
		if err := s.emitBuildLogCatchUp(ctx, sink, path, cursor, eof); err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Warn("deployment build log catch-up failed", "deployment_id", deployment.ID, "err", err)
				_ = sink.writeJSON(map[string]any{"t": deploylogs.TypeEnd, "reason": "catch_up_error"})
			}
			return
		}
		cursor = eof
	}

	onRotated := func() error {
		_ = sink.writeJSON(map[string]any{"t": deploylogs.TypeResync, "reason": "rotated"})
		return nil
	}

	err := logsapi.FollowFileFromOffset(ctx, path, cursor, 500*time.Millisecond, onRotated, func(data []byte, endOffset int64) error {
		if len(data) == 0 {
			return nil
		}
		return emitBuildLogJSONChunks(sink, data, endOffset)
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Warn("deployment build log follow ended", "deployment_id", deployment.ID, "err", err)
		_ = sink.writeJSON(map[string]any{"t": deploylogs.TypeEnd, "reason": "stream_error", "detail": err.Error()})
	}
}

func emitBuildLogJSONChunks(sink *wsLogSink, data []byte, endOffset int64) error {
	if len(data) == 0 {
		return nil
	}
	remaining := data
	baseEnd := endOffset - int64(len(data))
	for len(remaining) > 0 {
		n := len(remaining)
		if n > deploymentLogMaxJSONChunkBytes {
			n = deploymentLogMaxJSONChunkBytes
		}
		chunk := remaining[:n]
		remaining = remaining[n:]
		partEnd := baseEnd + int64(len(chunk))
		baseEnd = partEnd
		if err := sink.writeJSON(map[string]any{
			"t":   deploylogs.TypeChunk,
			"end": partEnd,
			"d":   string(chunk),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *server) emitBuildLogCatchUp(ctx context.Context, sink *wsLogSink, path string, start, end int64) error {
	if start >= end {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, deploymentLogMaxJSONChunkBytes)
	off := start
	for off < end {
		if err := ctx.Err(); err != nil {
			return err
		}
		toRead := int64(len(buf))
		if end-off < toRead {
			toRead = end - off
		}
		n, readErr := f.ReadAt(buf[:toRead], off)
		if readErr != nil && readErr != io.EOF {
			return readErr
		}
		if n <= 0 {
			break
		}
		chunk := buf[:n]
		off += int64(n)
		if err := emitBuildLogJSONChunks(sink, chunk, off); err != nil {
			return err
		}
	}
	return nil
}

// containerJSONWriter buffers docker log bytes into framed JSON chunks.
type containerJSONWriter struct {
	sink *wsLogSink
	buf  bytes.Buffer
	seq  uint64
	mu   sync.Mutex
}

func (w *containerJSONWriter) flush() error {
	if w.buf.Len() == 0 {
		return nil
	}
	w.seq++
	data := append([]byte(nil), w.buf.Bytes()...)
	w.buf.Reset()
	return w.sink.writeJSON(map[string]any{
		"t":   deploylogs.TypeChunk,
		"seq": w.seq,
		"d":   string(data),
	})
}

func (w *containerJSONWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = w.buf.Write(p)
	for w.buf.Len() >= 8192 {
		if err := w.flush(); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

func (w *containerJSONWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.flush()
}

func (s *server) streamContainerLogJSON(ctx context.Context, log *slog.Logger, sink *wsLogSink, deploymentID string) {
	containerRec, err := s.store.GetContainerByDeploymentID(ctx, deploymentID)
	if err != nil {
		_ = sink.writeJSON(map[string]any{"t": deploylogs.TypeError, "code": "container_not_found", "msg": "container record not found for deployment"})
		return
	}
	cli, err := docker.NewClient(ctx)
	if err != nil {
		_ = sink.writeJSON(map[string]any{"t": deploylogs.TypeError, "code": "docker_unavailable", "msg": "docker unavailable"})
		return
	}
	defer cli.Close()

	_ = sink.writeJSON(map[string]any{
		"t":             deploylogs.TypeHello,
		"v":             deploylogs.ProtocolVersion,
		"source":        "container",
		"resume":        false,
		"deployment_id": deploymentID,
		"container_id":  containerRec.DockerContainerID,
	})

	out := &containerJSONWriter{sink: sink}
	streamErr := docker.StreamContainerLogs(ctx, cli, containerRec.DockerContainerID, docker.LogStreamOptions{
		Follow:     true,
		Tail:       "200",
		ShowStdout: true,
		ShowStderr: true,
	}, out)
	if closeErr := out.Close(); closeErr != nil && streamErr == nil {
		streamErr = closeErr
	}
	if streamErr != nil && !errors.Is(streamErr, context.Canceled) {
		log.Warn("deployment container log stream ended", "deployment_id", deploymentID, "err", streamErr)
		_ = sink.writeJSON(map[string]any{"t": deploylogs.TypeEnd, "reason": "stream_error", "detail": streamErr.Error()})
	}
}
