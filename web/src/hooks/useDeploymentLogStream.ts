import { useEffect, useRef, useState } from "react";
import type { DeploymentLogTail } from "../api";
import { mergeLogOverlap } from "../logStream";

export type LogStreamState =
  | "loading tail"
  | "connecting"
  | "live"
  | "reconnecting"
  | "ended"
  | "error";

/** One JSON object per WebSocket text frame from `/api/deployments/.../logs/live`. */
export type DeploymentLogWsMessage =
  | { t: "hello"; v?: number; source?: string; resume?: boolean; eof?: number; cursor?: number; deployment_id?: string }
  | { t: "chunk"; end?: number; seq?: number; d?: string }
  | { t: "heartbeat"; seq?: number }
  | { t: "resync"; reason?: string; eof?: number }
  | { t: "end"; reason?: string; detail?: string }
  | { t: "error"; code?: string; msg?: string };

function parseWsMessage(raw: string): DeploymentLogWsMessage | null {
  try {
    const v = JSON.parse(raw) as DeploymentLogWsMessage;
    if (v && typeof v === "object" && "t" in v && typeof (v as { t: unknown }).t === "string") {
      return v;
    }
  } catch {
    /* ignore */
  }
  return null;
}

export function useDeploymentLogStream(opts: {
  deploymentId: string;
  source: "build" | "container";
  /** When false, the WebSocket is closed and reconnect timers cleared. */
  active: boolean;
  /** When true, incoming log chunks are not appended (scroll lock / pause). */
  paused: boolean;
  fetchTail: () => Promise<DeploymentLogTail>;
  shouldReconnect: () => boolean;
  /** Optional refresh (e.g. refetch deployments) before reconnecting after a drop. */
  onReconnectTick?: () => Promise<void>;
  /** Debounce before showing "reconnecting" in the UI (ms). */
  reconnectingLabelDelayMs?: number;
  /** Delay before attempting reconnect after close (ms). */
  reconnectDelayMs?: number;
}) {
  const {
    deploymentId,
    source,
    active,
    paused,
    fetchTail,
    shouldReconnect,
    onReconnectTick,
    reconnectingLabelDelayMs = 700,
    reconnectDelayMs = 400,
  } = opts;

  const [lines, setLines] = useState("");
  const [streamState, setStreamState] = useState<LogStreamState>("connecting");
  const [lastStreamDetail, setLastStreamDetail] = useState<string | null>(null);
  const [tailError, setTailError] = useState<string | null>(null);

  const wsRef = useRef<WebSocket | null>(null);
  const pausedRef = useRef(paused);
  const connectionGenRef = useRef(0);
  /** Exclusive byte offset in the build log file (from server `chunk.end`). */
  const buildCursorRef = useRef(0);
  const needContainerOverlapRef = useRef(false);

  useEffect(() => {
    pausedRef.current = paused;
  }, [paused]);

  useEffect(() => {
    if (!active || !deploymentId) {
      connectionGenRef.current += 1;
      wsRef.current?.close();
      wsRef.current = null;
      return;
    }

    let cancelled = false;
    let reconnectTimer: number | undefined;
    let pendingStateTimer: number | undefined;
    let hasConnectedOnce = false;

    function clearPendingState() {
      if (pendingStateTimer !== undefined) {
        window.clearTimeout(pendingStateTimer);
        pendingStateTimer = undefined;
      }
    }

    function scheduleState(next: LogStreamState, delayMs: number) {
      clearPendingState();
      pendingStateTimer = window.setTimeout(() => {
        if (!cancelled) setStreamState(next);
      }, delayMs);
    }

    function clearReconnectTimer() {
      if (reconnectTimer !== undefined) {
        window.clearTimeout(reconnectTimer);
        reconnectTimer = undefined;
      }
    }

    function connect() {
      if (cancelled) return;
      connectionGenRef.current += 1;
      const gen = connectionGenRef.current;
      wsRef.current?.close();

      const protocol = window.location.protocol === "https:" ? "wss" : "ws";
      const params = new URLSearchParams({ source, format: "json" });
      if (source === "build" && buildCursorRef.current > 0) {
        params.set("cursor", String(buildCursorRef.current));
      }
      const ws = new WebSocket(
        `${protocol}://${window.location.host}/api/deployments/${deploymentId}/logs/live?${params.toString()}`,
      );
      wsRef.current = ws;
      needContainerOverlapRef.current = source === "container";

      if (!hasConnectedOnce) setStreamState("connecting");
      ws.onopen = () => {
        if (cancelled || gen !== connectionGenRef.current) {
          ws.close();
          return;
        }
        clearPendingState();
        hasConnectedOnce = true;
        setStreamState("live");
        setLastStreamDetail(null);
      };
      ws.onerror = () => {
        if (!cancelled && gen === connectionGenRef.current && !hasConnectedOnce) {
          setStreamState("error");
          setLastStreamDetail("websocket_error");
        }
      };
      ws.onclose = () => {
        if (cancelled || gen !== connectionGenRef.current) return;
        clearReconnectTimer();
        const reconnect = shouldReconnect();
        if (reconnect) {
          scheduleState("reconnecting", reconnectingLabelDelayMs);
          reconnectTimer = window.setTimeout(async () => {
            if (cancelled || gen !== connectionGenRef.current) return;
            try {
              if (onReconnectTick) await onReconnectTick();
            } catch {
              /* keep trying while build may still be running */
            }
            if (cancelled || gen !== connectionGenRef.current) return;
            if (!shouldReconnect()) {
              clearPendingState();
              setStreamState("ended");
              return;
            }
            connect();
          }, reconnectDelayMs);
          return;
        }
        clearPendingState();
        setStreamState("ended");
      };
      ws.onmessage = (event) => {
        if (pausedRef.current) return;
        if (gen !== connectionGenRef.current) return;
        const raw = typeof event.data === "string" ? event.data : "";
        const msg = parseWsMessage(raw);
        if (!msg) return;

        switch (msg.t) {
          case "hello":
            return;
          case "heartbeat":
            return;
          case "resync":
            if (source === "build") {
              setLines("");
              buildCursorRef.current = 0;
            }
            return;
          case "error": {
            const code = typeof msg.code === "string" ? msg.code : "";
            const detail = msg.msg || code || "error";
            const recoverable =
              code === "docker_unavailable" || code === "stat_failed" || code === "container_not_found";
            if (recoverable && shouldReconnect()) {
              setLastStreamDetail(detail);
              scheduleState("reconnecting", reconnectingLabelDelayMs);
              ws.close();
              return;
            }
            setLastStreamDetail(detail);
            clearPendingState();
            clearReconnectTimer();
            setStreamState("ended");
            connectionGenRef.current += 1;
            ws.close();
            return;
          }
          case "end": {
            const reason = typeof msg.reason === "string" ? msg.reason : "";
            const detail =
              reason +
              (typeof msg.detail === "string" && msg.detail ? `: ${msg.detail}` : "");
            const recoverableEnd =
              reason === "stream_error" || reason === "catch_up_error" || reason === "stream_ended";
            if (recoverableEnd && shouldReconnect()) {
              setLastStreamDetail(detail || reason);
              scheduleState("reconnecting", reconnectingLabelDelayMs);
              ws.close();
              return;
            }
            setLastStreamDetail(detail || reason || "end");
            clearPendingState();
            clearReconnectTimer();
            setStreamState("ended");
            connectionGenRef.current += 1;
            ws.close();
            return;
          }
          case "chunk": {
            const d = typeof msg.d === "string" ? msg.d : "";
            if (d.length === 0) return;
            if (source === "build" && typeof msg.end === "number" && Number.isFinite(msg.end)) {
              buildCursorRef.current = msg.end;
              setLines((prev) => `${prev}${d}`);
              return;
            }
            if (source === "container") {
              if (needContainerOverlapRef.current) {
                needContainerOverlapRef.current = false;
                setLines((prev) => mergeLogOverlap(prev, d));
              } else {
                setLines((prev) => `${prev}${d}`);
              }
            }
            return;
          }
          default:
            return;
        }
      };
    }

    (async () => {
      try {
        setStreamState("loading tail");
        setTailError(null);
        const { text, eofOffset } = await fetchTail();
        if (cancelled) return;
        setLines(text);
        if (source === "build") {
          buildCursorRef.current = eofOffset;
        }
      } catch (err) {
        if (!cancelled) {
          setLines("");
          if (source === "build") {
            buildCursorRef.current = 0;
          }
          setTailError(err instanceof Error ? err.message : "failed to load log tail");
        }
      }
      if (cancelled) return;
      connect();
    })();

    return () => {
      cancelled = true;
      clearPendingState();
      clearReconnectTimer();
      connectionGenRef.current += 1;
      wsRef.current?.close();
      wsRef.current = null;
    };
  }, [active, deploymentId, source, fetchTail, shouldReconnect, onReconnectTick, reconnectingLabelDelayMs, reconnectDelayMs]);

  return {
    lines,
    setLines,
    streamState,
    lastStreamDetail,
    tailError,
  };
}
