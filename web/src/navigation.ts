export function navigateTo(to: string) {
  const next = new URL(to, window.location.href)
  if (next.origin !== window.location.origin) {
    window.location.assign(next.href)
    return
  }

  const current = `${window.location.pathname}${window.location.search}${window.location.hash}`
  const target = `${next.pathname}${next.search}${next.hash}`
  if (current === target) return

  window.history.pushState({}, "", target)
  window.dispatchEvent(new PopStateEvent("popstate"))
  window.scrollTo({ top: 0, behavior: "instant" })
}
