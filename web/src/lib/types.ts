import { useQuery } from "@tanstack/react-query";
import { api } from "./api";

// The registered check types and notification channels. They only change
// with a new agent version, so they're fetched once per page load.

export function useCheckTypes() {
  return useQuery({ queryKey: ["check-types"], queryFn: api.checkTypes, staleTime: Infinity });
}

export function useNotifierChannels() {
  return useQuery({ queryKey: ["notifier-types"], queryFn: api.notifierTypes, staleTime: Infinity });
}

/** Display name for a check type, e.g. "PostgreSQL"; the raw type until loaded. */
export function useCheckTypeLabel() {
  const types = useCheckTypes();
  return (type: string) => types.data?.find((t) => t.type === type)?.label ?? type;
}

export function useChannelLabel() {
  const channels = useNotifierChannels();
  return (type: string) => channels.data?.find((c) => c.type === type)?.label ?? type;
}
