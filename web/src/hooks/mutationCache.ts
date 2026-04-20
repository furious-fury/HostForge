import type { QueryClient } from "@tanstack/react-query";
import { fleetKeys } from "./fleetQueries";

/** Prefix for all deployment list queries (limit varies by page). */
export const deploymentListQueryPrefix = ["deployments", "list"] as const;

/**
 * After project mutations (deploy, delete, restart, …), refresh fleet caches so
 * Dashboard / Projects / Deployments stay aligned without a full page reload.
 */
export async function invalidateFleetProjectsAndDeployments(queryClient: QueryClient): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: fleetKeys.projects }),
    queryClient.invalidateQueries({ queryKey: deploymentListQueryPrefix }),
  ]);
}
