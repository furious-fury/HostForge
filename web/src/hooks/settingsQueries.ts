import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  fetchSettings,
  postCaddySync,
  postCaddyValidate,
  postDetectPublicIPv4,
  postRefreshSystemStatus,
  type DetectIPv4Result,
  type HostForgeSettings,
} from "../api/settings";
import { fleetKeys } from "./fleetQueries";

export const settingsKeys = {
  root: ["settings"] as const,
};

const settingsStaleTime = 60_000;

export function useSettingsQuery() {
  return useQuery({
    queryKey: settingsKeys.root,
    queryFn: fetchSettings,
    staleTime: settingsStaleTime,
    retry: 1,
  });
}

export function useSettingsMutations() {
  const qc = useQueryClient();

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: settingsKeys.root });
    void qc.invalidateQueries({ queryKey: fleetKeys.systemStatus });
  };

  const caddyValidate = useMutation({
    mutationFn: postCaddyValidate,
    onSuccess: invalidate,
  });

  const caddySync = useMutation({
    mutationFn: postCaddySync,
    onSuccess: invalidate,
  });

  const refreshStatus = useMutation({
    mutationFn: postRefreshSystemStatus,
    onSuccess: invalidate,
  });

  const detectIPv4 = useMutation({
    mutationFn: postDetectPublicIPv4,
    onSuccess: invalidate,
  });

  return { caddyValidate, caddySync, refreshStatus, detectIPv4, invalidate };
}

export type { HostForgeSettings, DetectIPv4Result };
