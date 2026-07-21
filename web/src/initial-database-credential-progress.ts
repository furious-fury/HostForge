export type InitialDatabaseCredentialProgress = {
  environmentId: string
  environmentName: string
  connectionId: string
  operationId: string
}

export function initialDatabaseCredentialProgress(value: unknown): InitialDatabaseCredentialProgress[] {
  if (!Array.isArray(value)) return []
  return value.flatMap((entry) => {
    if (!entry || typeof entry !== "object") return []
    const candidate = entry as Partial<InitialDatabaseCredentialProgress>
    if (![candidate.environmentId, candidate.environmentName, candidate.connectionId, candidate.operationId].every((item) => typeof item === "string" && item.length > 0)) return []
    return [{
      environmentId: candidate.environmentId as string,
      environmentName: candidate.environmentName as string,
      connectionId: candidate.connectionId as string,
      operationId: candidate.operationId as string,
    }]
  })
}
