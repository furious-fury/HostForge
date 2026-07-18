import pg from "pg"

const connectionString = process.env.DATABASE_URL
if (!connectionString?.includes("sslmode=verify-full")) {
  throw new Error("DATABASE_URL must contain sslmode=verify-full")
}

const client = new pg.Client({
  connectionString,
  connectionTimeoutMillis: 10_000,
  statement_timeout: 10_000,
})

try {
  await client.connect()
  const result = await client.query(`
    SELECT current_user AS role_name,
           COALESCE((SELECT ssl FROM pg_stat_ssl WHERE pid = pg_backend_pid()), false) AS tls_active
  `)
  if (result.rowCount !== 1 || !result.rows[0].role_name || result.rows[0].tls_active !== true) {
    throw new Error("pg connected without the expected TLS session metadata")
  }
  console.log("PASS: pg connects through the gateway with verified TLS")
} finally {
  await client.end().catch(() => undefined)
}
