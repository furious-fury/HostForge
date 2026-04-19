import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { fetchHostHistory, fetchHostSnapshot, type HostHistory, type HostSnapshot } from "../api/host";

export const hostKeys = {
  snapshot: ["host", "snapshot"] as const,
  history: (points: number) => ["host", "history", points] as const,
};

export function useHostSnapshot() {
  return useQuery({
    queryKey: hostKeys.snapshot,
    queryFn: async (): Promise<HostSnapshot | null> => {
      try {
        return await fetchHostSnapshot();
      } catch {
        return null;
      }
    },
    staleTime: 4000,
    refetchInterval: 5000,
    retry: 0,
  });
}

export function useHostHistory(points: number) {
  return useQuery({
    queryKey: hostKeys.history(points),
    queryFn: async (): Promise<HostHistory | null> => {
      try {
        return await fetchHostHistory(points);
      } catch {
        return null;
      }
    },
    staleTime: 10_000,
    refetchInterval: 10_000,
    placeholderData: keepPreviousData,
    retry: 0,
  });
}
