import { useQuery } from "@tanstack/react-query";
import {
  fetchDeploymentSteps,
  fetchObservabilityDeploySteps,
  fetchObservabilityRequests,
  fetchObservabilitySummary,
  type DeployStepRow,
  type HTTPRequestRow,
  type ObservabilitySummary,
  type SystemStatus,
} from "../api";

export const observabilityKeys = {
  summary: ["observability", "summary"] as const,
  requests: (limit: number) => ["observability", "requests", limit] as const,
  deploySteps: (limit: number) => ["observability", "deploySteps", limit] as const,
  deploymentSteps: (deploymentID: string, limit: number) =>
    ["observability", "deploymentSteps", deploymentID, limit] as const,
};

const staleTime = 10_000;

export function useObservabilitySummaryQuery() {
  return useQuery({
    queryKey: observabilityKeys.summary,
    queryFn: async (): Promise<{ summary: ObservabilitySummary; system: SystemStatus }> => {
      return await fetchObservabilitySummary();
    },
    staleTime,
    retry: 1,
  });
}

export function useObservabilityRequestsQuery(limit = 100) {
  return useQuery({
    queryKey: observabilityKeys.requests(limit),
    queryFn: (): Promise<HTTPRequestRow[]> => fetchObservabilityRequests(limit),
    staleTime,
    retry: 1,
  });
}

export function useObservabilityDeployStepsQuery(limit = 200) {
  return useQuery({
    queryKey: observabilityKeys.deploySteps(limit),
    queryFn: (): Promise<DeployStepRow[]> => fetchObservabilityDeploySteps(limit),
    staleTime,
    retry: 1,
  });
}

export function useDeploymentStepsQuery(deploymentID: string, limit = 200) {
  return useQuery({
    queryKey: observabilityKeys.deploymentSteps(deploymentID, limit),
    queryFn: (): Promise<DeployStepRow[]> => fetchDeploymentSteps(deploymentID, limit),
    enabled: Boolean(deploymentID),
    staleTime,
    retry: 1,
  });
}
