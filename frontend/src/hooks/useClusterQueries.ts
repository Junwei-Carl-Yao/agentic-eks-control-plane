import { useQuery } from '@tanstack/react-query';

import { clusterApi } from '@/api/client';

const POLL_INTERVAL_MS = 5000;

export function useNamespaces() {
  return useQuery({
    queryKey: ['namespaces'],
    queryFn: () => clusterApi.listNamespaces(),
    refetchInterval: POLL_INTERVAL_MS,
  });
}

export function useNodes() {
  return useQuery({
    queryKey: ['nodes'],
    queryFn: () => clusterApi.listNodes(),
    refetchInterval: POLL_INTERVAL_MS,
  });
}

export function useDeployments(namespace: string) {
  return useQuery({
    queryKey: ['deployments', namespace],
    queryFn: () => clusterApi.listDeployments(namespace),
    refetchInterval: POLL_INTERVAL_MS,
    enabled: namespace.length > 0,
  });
}

export function usePods(namespace: string) {
  return useQuery({
    queryKey: ['pods', namespace],
    queryFn: () => clusterApi.listPods(namespace),
    refetchInterval: POLL_INTERVAL_MS,
    enabled: namespace.length > 0,
  });
}

export function useServices(namespace: string) {
  return useQuery({
    queryKey: ['services', namespace],
    queryFn: () => clusterApi.listServices(namespace),
    refetchInterval: POLL_INTERVAL_MS,
    enabled: namespace.length > 0,
  });
}

export function useEvents(namespace: string) {
  return useQuery({
    queryKey: ['events', namespace],
    queryFn: () => clusterApi.listEvents(namespace),
    refetchInterval: POLL_INTERVAL_MS,
    enabled: namespace.length > 0,
  });
}
