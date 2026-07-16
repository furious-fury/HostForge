import { describe, expect, it } from "vitest"

import { envExample, MAX_ENV_VALUE_BYTES, parseEnvFile } from "./env-file"

describe("parseEnvFile", () => {
  it("parses comments, exports, normalized keys, empty values, and quoted values", () => {
    const result = parseEnvFile(`# ignored\nexport api-url=https://example.test/path#fragment\nEMPTY=\nDOUBLE="line\\nnext"\nSINGLE=' literal # value '\nPLAIN=value # comment\n`)

    expect(result.errors).toEqual([])
    expect(result.entries).toEqual([
      { key: "API_URL", value: "https://example.test/path#fragment" },
      { key: "EMPTY", value: "" },
      { key: "DOUBLE", value: "line\nnext" },
      { key: "SINGLE", value: " literal # value " },
      { key: "PLAIN", value: "value" },
    ])
  })

  it("rejects malformed, duplicate, reserved, and oversized entries atomically", () => {
    const result = parseEnvFile(`_INVALID=value\nPORT=3000\nDUPLICATE=first\nduplicate=second\nOPEN="missing\nLARGE=${"x".repeat(MAX_ENV_VALUE_BYTES + 1)}`)

    expect(result.entries).toEqual([])
    expect(result.errors.map((error) => error.message)).toEqual(expect.arrayContaining([
      expect.stringContaining("key must start"),
      expect.stringContaining("PORT is managed"),
      expect.stringContaining("duplicate key DUPLICATE"),
      expect.stringContaining("not terminated"),
      expect.stringContaining("value exceeds"),
    ]))
  })
})

describe("envExample", () => {
  it("exports sorted unique names without values", () => {
    expect(envExample(["SECRET", "API_URL", "SECRET"])).toBe("API_URL=\nSECRET=\n")
  })
})
