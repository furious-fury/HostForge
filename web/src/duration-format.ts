export function formatDuration(durationMS: number) {
  if (durationMS < 1000) return `${durationMS} ms`
  const seconds = Math.round((durationMS / 1000) * 10) / 10
  return `${seconds} s`
}
