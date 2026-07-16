export const MAX_ENV_FILE_BYTES = 1024 * 1024
export const MAX_ENV_ENTRIES = 100
export const MAX_ENV_KEY_BYTES = 128
export const MAX_ENV_VALUE_BYTES = 8 * 1024

export type EnvFileEntry = { key: string; value: string }
export type EnvFileError = { line: number; message: string }
export type EnvFileResult = { entries: EnvFileEntry[]; errors: EnvFileError[] }

const encoder = new TextEncoder()

export function parseEnvFile(source: string): EnvFileResult {
  if (encoder.encode(source).byteLength > MAX_ENV_FILE_BYTES) {
    return { entries: [], errors: [{ line: 0, message: "file exceeds 1 MiB" }] }
  }

  const entries: EnvFileEntry[] = []
  const errors: EnvFileError[] = []
  const seen = new Set<string>()

  source.split(/\r?\n/).forEach((raw, index) => {
    const lineNumber = index + 1
    const line = raw.trim()
    if (!line || line.startsWith("#")) return

    const normalized = line.startsWith("export ") ? line.slice(7).trimStart() : line
    const separator = normalized.indexOf("=")
    if (separator <= 0) {
      errors.push({ line: lineNumber, message: "expected KEY=VALUE" })
      return
    }

    const key = normalized.slice(0, separator).trim().replaceAll("-", "_").toUpperCase()
    const keyBytes = encoder.encode(key).byteLength
    if (!/^[A-Z][A-Z0-9_]*$/.test(key)) {
      errors.push({ line: lineNumber, message: "key must start with a letter and contain only letters, numbers, or underscores" })
      return
    }
    if (keyBytes > MAX_ENV_KEY_BYTES) {
      errors.push({ line: lineNumber, message: `key exceeds ${MAX_ENV_KEY_BYTES} bytes` })
      return
    }
    if (key === "PORT") {
      errors.push({ line: lineNumber, message: "PORT is managed by HostForge" })
      return
    }
    if (seen.has(key)) {
      errors.push({ line: lineNumber, message: `duplicate key ${key}` })
      return
    }

    const parsed = parseValue(normalized.slice(separator + 1))
    if (parsed.error) {
      errors.push({ line: lineNumber, message: parsed.error })
      return
    }
    if (parsed.value.includes("\0")) {
      errors.push({ line: lineNumber, message: "value contains a NUL byte" })
      return
    }
    if (encoder.encode(parsed.value).byteLength > MAX_ENV_VALUE_BYTES) {
      errors.push({ line: lineNumber, message: `value exceeds ${MAX_ENV_VALUE_BYTES} bytes` })
      return
    }

    seen.add(key)
    entries.push({ key, value: parsed.value })
  })

  if (entries.length > MAX_ENV_ENTRIES) {
    errors.push({ line: 0, message: `file contains more than ${MAX_ENV_ENTRIES} variables` })
  }

  return errors.length ? { entries: [], errors } : { entries, errors: [] }
}

function parseValue(raw: string): { value: string; error?: string } {
  const value = raw.trim()
  if (!value) return { value: "" }

  const quote = value[0]
  if (quote === '"' || quote === "'") {
    if (value.length < 2 || value[value.length - 1] !== quote) {
      return { value: "", error: "quoted value is not terminated" }
    }
    const inner = value.slice(1, -1)
    return { value: quote === '"' ? decodeDoubleQuoted(inner) : inner }
  }

  const comment = value.search(/\s+#/)
  return { value: (comment >= 0 ? value.slice(0, comment) : value).trimEnd() }
}

function decodeDoubleQuoted(value: string) {
  return value.replace(/\\([nrt\\"])/g, (_, escaped: string) => {
    if (escaped === "n") return "\n"
    if (escaped === "r") return "\r"
    if (escaped === "t") return "\t"
    return escaped
  })
}

export function envExample(keys: string[]) {
  return [...new Set(keys)].sort((left, right) => left.localeCompare(right)).map((key) => `${key}=`).join("\n") + (keys.length ? "\n" : "")
}
