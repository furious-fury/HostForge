export function formatRuntimeLogLine(line: string) {
  try {
    const entry = JSON.parse(line) as {
      logger?: string
      msg?: string
      duration?: number
      size?: number
      status?: number
      request?: { method?: string; uri?: string; client_ip?: string }
    }
    if (!entry.logger?.startsWith("http.log.access") || entry.msg !== "handled request") return line
    const duration = entry.duration === undefined ? "" : entry.duration < 0.001 ? "<1 ms" : `${Math.round(entry.duration * 1000)} ms`
    const size = formatLogBytes(entry.size || 0)
    return [
      "[access]",
      entry.status || "-",
      entry.request?.method || "-",
      entry.request?.uri || "/",
      duration && `· ${duration}`,
      size && `· ${size}`,
      entry.request?.client_ip && `· ${entry.request.client_ip}`,
    ].filter(Boolean).join(" ")
  } catch {
    return line
  }
}

function formatLogBytes(value: number) {
  if (!value) return ""
  if (value >= 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MB`
  if (value >= 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${value} B`
}
