import { describe, expect, it } from "vitest"

import { formatDuration } from "@/duration-format"

describe("formatDuration", () => {
  it("keeps short stages in milliseconds and promotes larger values to seconds", () => {
    expect(formatDuration(850)).toBe("850 ms")
    expect(formatDuration(1000)).toBe("1 s")
    expect(formatDuration(1450)).toBe("1.5 s")
    expect(formatDuration(42000)).toBe("42 s")
  })
})
