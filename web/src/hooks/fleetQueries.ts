import { keepPreviousData, useQuery } from "@tanstack/react-query";
import {
  fetchAllDeployments,
  fetchProjects,
  fetchSystemStatus,
  type SystemStatus,
} from "../api";

export const fleetKeys = {
  projects: ["projects"] as const,
  deployments: (limit: number) => ["deployments", "list", limit] as const,
  systemStatus: ["system", "status"] as const,
};

const staleTime = 45_000;

export function useProjectsQuery() {
  return useQuery({
    queryKey: fleetKeys.projects,
    queryFn: fetchProjects,
    staleTime,
  });
}

export function useDeploymentsListQuery(limit: number, options?: { keepPreviousWhileFetching?: boolean }) {
  return useQuery({
    queryKey: fleetKeys.deployments(limit),
    queryFn: () => fetchAllDeployments(limit),
    staleTime,
    retry: 1,
    placeholderData: options?.keepPreviousWhileFetching ? keepPreviousData : undefined,
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
