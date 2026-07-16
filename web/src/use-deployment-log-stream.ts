import { useEffect, useRef, useState } from "react"

type ConnectionState = "idle" | "connecting" | "connected" | "reconnecting" | "ended" | "error"

type LogMessage = {
  t?: "hello" | "chunk" | "heartbeat" | "resync" | "end" | "error"
  d?: string
  end?: number
  eof?: number
  code?: string
  msg?: string
  resume?: boolean
  reason?: string
  status?: string
  deployment_id?: string
  application_id?: string
  service_id?: string
  environment_id?: string
}

const maxBufferedCharacters = 1_000_000

export function useDeploymentLogStream(deploymentID: string, enabled: boolean, source: "build" | "container" = "build") {
  const [text, setText] = useState("")
  const [connection, setConnection] = useState<ConnectionState>("idle")
  const [error, setError] = useState("")
  const cursor = useRef(0)

  useEffect(() => {
    if (!enabled || !deploymentID) {
      return
    }

    let socket: WebSocket | undefined
    let reconnectTimer: number | undefined
    let stopped = false
    let attempts = 0
    let connectedOnce = false

    const connect = () => {
      if (stopped) return
      setConnection(attempts ? "reconnecting" : "connecting")
      const protocol = window.location.protocol === "https:" ? "wss:" : "ws:"
      const params = new URLSearchParams({ source, format: "json", cursor: String(cursor.current) })
      socket = new WebSocket(`${protocol}//${window.location.host}/api/deployments/${encodeURIComponent(deploymentID)}/logs/live?${params}`)
      socket.onopen = () => {
        if (source === "container" && connectedOnce) setText("")
        connectedOnce = true
        attempts = 0
        setError("")
        setConnection("connected")
      }
      socket.onmessage = (event) => {
        let message: LogMessage
        try {
          message = JSON.parse(String(event.data)) as LogMessage
        } catch {
          setError("The server returned an invalid log frame.")
          setConnection("error")
          return
        }
        if (message.t === "chunk") {
          if (typeof message.end === "number") cursor.current = message.end
          if (message.d) setText((current) => (current + message.d).slice(-maxBufferedCharacters))
        } else if (message.t === "resync") {
          cursor.current = 0
          setText("")
          socket?.close()
        } else if (message.t === "end") {
          if (typeof message.eof === "number") cursor.current = message.eof
          stopped = true
          setConnection("ended")
          socket?.close()
        } else if (message.t === "error") {
          setError(message.msg || message.code || "The log stream failed.")
          setConnection("error")
        }
      }
      socket.onclose = () => {
        if (stopped) return
        attempts += 1
        if (attempts >= 6) {
          stopped = true
          setError("The log stream could not reconnect. Check the server connection and session.")
          setConnection("error")
          return
        }
        setConnection("reconnecting")
        reconnectTimer = window.setTimeout(connect, Math.min(10_000, 500 * 2 ** attempts))
      }
      socket.onerror = () => socket?.close()
    }

    connect()
    return () => {
      stopped = true
      if (reconnectTimer) window.clearTimeout(reconnectTimer)
      socket?.close()
    }
  }, [deploymentID, enabled, source])

  return { text, connection: enabled ? connection : "idle" as ConnectionState, error, clear: () => setText("") }
}
