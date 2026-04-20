import { keepPreviousData, useQuery } from "@tanstack/react-query";
import {
  fetchAllDeployments,
  fetchProjects,
  fetchSystemStatus,
  type ApiDeployment,
  type ApiProject,
  type SystemStatus,
} from "../api";

export const fleetKeys = {
  projects: ["projects"] as const,
  deployments: (limit: number) => ["deployments", "list", limit] as const,
  systemStatus: ["system", "status"] as const,
};

const staleTime = 45_000;

function projectHasInFlightDeploy(projects: ApiProject[] | undefined): boolean {
  if (!projects?.length) return false;
  return projects.some((p) => {
    const s = p.latest_deployment?.status?.toUpperCase();
    return s === "QUEUED" || s === "BUILDING";
  });
}

function deploymentListHasInFlight(rows: ApiDeployment[] | undefined): boolean {
  if (!rows?.length) return false;
  return rows.some((d) => {
    const s = d.status?.toUpperCase();
    return s === "QUEUED" || s === "BUILDING";
  });
}

export function useProjectsQuery(options?: { refetchWhileInFlight?: boolean }) {
  return useQuery({
    queryKey: fleetKeys.projects,
    queryFn: fetchProjects,
    staleTime,
    refetchInterval: (q) =>
      options?.refetchWhileInFlight && projectHasInFlightDeploy(q.state.data as ApiProject[] | undefined)
        ? 2000
        : false,
  });
}

export function useDeploymentsListQuery(
  limit: number,
  options?: { keepPreviousWhileFetching?: boolean; refetchWhileInFlight?: boolean },
) {
  return useQuery({
    queryKey: fleetKeys.deployments(limit),
    queryFn: () => fetchAllDeployments(limit),
    staleTime,
    retry: 1,
    placeholderData: options?.keepPreviousWhileFetching ? keepPreviousData : undefined,
    refetchInterval: (q) =>
      options?.refetchWhileInFlight && deploymentListHasInFlight(q.state.data as ApiDeployment[] | undefined)
        ? 2000
        : false,
  });
}

export function useSystemStatusQuery() {
  return useQuery({
    queryKey: fleetKeys.systemStatus,
    queryFn: async (): Promise<SystemStatus | null> => {
      try {
        return await fetchSystemStatus();
      } catch {
        return null;
      }
    },
    staleTime,
    retry: 0,
  });
}
