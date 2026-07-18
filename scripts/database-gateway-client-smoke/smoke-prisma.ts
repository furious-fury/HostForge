import { PrismaPg } from "@prisma/adapter-pg"
import { PrismaClient } from "@prisma/client"

const connectionString = process.env.DATABASE_URL
if (!connectionString?.includes("sslmode=verify-full")) {
  throw new Error("DATABASE_URL must contain sslmode=verify-full")
}

const adapter = new PrismaPg({ connectionString })
const prisma = new PrismaClient({ adapter })

try {
  const rows = await prisma.$queryRaw<Array<{ role_name: string; tls_active: boolean }>>`
    SELECT current_user AS role_name,
           COALESCE((SELECT ssl FROM pg_stat_ssl WHERE pid = pg_backend_pid()), false) AS tls_active
  `
  if (!rows[0]?.role_name || rows[0].tls_active !== true) {
    throw new Error("Prisma connected without the expected TLS session metadata")
  }
  console.log("PASS: Prisma connects through the gateway with verified TLS")
} finally {
  await prisma.$disconnect()
}
