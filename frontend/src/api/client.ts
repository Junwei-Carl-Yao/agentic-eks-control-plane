import axios, { AxiosError } from 'axios';

import type {
  ClusterEvent,
  ClusterHealth,
  ClusterInfo,
  DenialResponse,
  Deployment,
  HorizontalPodAutoscaler,
  Ingress,
  Node,
  Pod,
  ReplicaSet,
} from '@/types';

// Base URL is empty by default so requests go through the Vite dev proxy. In
// production builds where the frontend ships behind the same ALB, an empty
// base URL still resolves correctly to /api/* on the same origin.
const baseURL = import.meta.env.VITE_API_BASE_URL ?? '';

export const apiClient = axios.create({
  baseURL,
  headers: { 'Content-Type': 'application/json' },
});

// ApiError preserves the backend denial body so the UI can render the
// guardrail reason instead of a generic message.
export class ApiError extends Error {
  status: number;
  denial?: DenialResponse;
  payload?: unknown;

  constructor(message: string, status: number, denial?: DenialResponse, payload?: unknown) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.denial = denial;
    this.payload = payload;
  }
}

function toApiError(error: unknown): ApiError {
  if (error instanceof AxiosError) {
    const status = error.response?.status ?? 0;
    const data = error.response?.data;
    if (status === 403 && data && typeof data === 'object' && 'decision' in data) {
      const denial = data as DenialResponse;
      return new ApiError(denial.error || 'denied', status, denial, data);
    }
    if (data && typeof data === 'object' && 'error' in data) {
      const message = String((data as { error: unknown }).error ?? error.message);
      return new ApiError(message, status, undefined, data);
    }
    return new ApiError(error.message, status, undefined, data);
  }
  if (error instanceof Error) {
    return new ApiError(error.message, 0);
  }
  return new ApiError('unknown error', 0);
}

async function get<T>(path: string, params?: Record<string, string>): Promise<T> {
  try {
    const response = await apiClient.get<T>(path, { params });
    return response.data;
  } catch (error) {
    throw toApiError(error);
  }
}

export const clusterApi = {
  info: () => get<ClusterInfo>('/api/cluster/info'),
  health: () => get<ClusterHealth>('/api/cluster/health'),
  listNodes: () => get<Node[]>('/api/cluster/nodes'),
  listDeployments: (namespace: string) =>
    get<Deployment[]>('/api/cluster/deployments', { namespace }),
  listPods: (namespace: string, labelSelector?: string) =>
    get<Pod[]>('/api/cluster/pods', labelSelector ? { namespace, labelSelector } : { namespace }),
  listEvents: (namespace: string) => get<ClusterEvent[]>('/api/cluster/events', { namespace }),
  listIngresses: (namespace: string) => get<Ingress[]>('/api/cluster/ingresses', { namespace }),
  listHpas: (namespace: string) =>
    get<HorizontalPodAutoscaler[]>('/api/cluster/hpas', { namespace }),
  listReplicaSets: (namespace: string) =>
    get<ReplicaSet[]>('/api/cluster/replicasets', { namespace }),
};
