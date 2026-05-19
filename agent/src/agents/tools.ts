// SDK tool definitions, one per backend route. Each tool is a thin call into
// BackendClient — no policy decisions live here. The Zod schemas mirror the
// backend's typed request models; structural validation here helps the model
// avoid obviously-malformed calls but the backend still validates everything.

import { createSdkMcpServer, tool } from '@anthropic-ai/claude-agent-sdk';
import type { CallToolResult } from '@modelcontextprotocol/sdk/types.js';
import { z } from 'zod';

import { BackendClient } from '../backendClient.js';

export const MCP_SERVER_NAME = 'kubernetes';

function asToolResult(payload: unknown): CallToolResult {
  return {
    content: [
      {
        type: 'text',
        text: JSON.stringify(payload),
      },
    ],
  };
}

export function buildKubernetesMcpServer(client: BackendClient) {
  return createSdkMcpServer({
    name: MCP_SERVER_NAME,
    version: '0.1.0',
    tools: [
      tool(
        'health_check',
        'Check whether the backend control-plane API is reachable.',
        {},
        async () => asToolResult(await client.health()),
      ),
      tool(
        'cluster_info',
        'Return the cluster identity (name, region) and a live healthy flag.',
        {},
        async () => asToolResult(await client.clusterInfo()),
      ),
      tool(
        'list_deployments',
        'List Deployments in a namespace.',
        { namespace: z.string().min(1).describe('Kubernetes namespace') },
        async (args) => asToolResult(await client.listDeployments(args.namespace)),
      ),
      tool(
        'get_deployment',
        'Fetch a single Deployment by name and namespace.',
        {
          namespace: z.string().min(1).describe('Kubernetes namespace'),
          name: z.string().min(1).describe('Deployment name'),
        },
        async (args) => asToolResult(await client.getDeployment(args.namespace, args.name)),
      ),
      tool(
        'list_pods',
        'List Pods in a namespace, optionally filtered by labelSelector.',
        {
          namespace: z.string().min(1).describe('Kubernetes namespace'),
          labelSelector: z.string().optional().describe('Optional label selector, e.g. app=web'),
        },
        async (args) => asToolResult(await client.listPods(args.namespace, args.labelSelector)),
      ),
      tool(
        'list_events',
        'List recent Events in a namespace.',
        { namespace: z.string().min(1).describe('Kubernetes namespace') },
        async (args) => asToolResult(await client.listEvents(args.namespace)),
      ),
      tool(
        'tail_logs',
        'Tail logs from a specific container in a pod.',
        {
          namespace: z.string().min(1).describe('Kubernetes namespace'),
          pod: z.string().min(1).describe('Pod name'),
          container: z.string().min(1).describe('Container name within the pod'),
          lines: z.number().int().positive().describe('Number of lines to tail (positive integer)'),
        },
        async (args) =>
          asToolResult(await client.tailLogs(args.namespace, args.pod, args.container, args.lines)),
      ),
      tool(
        'list_services',
        'List Services in a namespace.',
        { namespace: z.string().min(1).describe('Kubernetes namespace') },
        async (args) => asToolResult(await client.listServices(args.namespace)),
      ),
      tool(
        'list_ingresses',
        'List Ingresses in a namespace.',
        { namespace: z.string().min(1).describe('Kubernetes namespace') },
        async (args) => asToolResult(await client.listIngresses(args.namespace)),
      ),
      tool(
        'list_hpas',
        'List HorizontalPodAutoscalers in a namespace.',
        { namespace: z.string().min(1).describe('Kubernetes namespace') },
        async (args) => asToolResult(await client.listHpas(args.namespace)),
      ),
      tool(
        'list_namespaces',
        'List namespaces visible to the operator (filtered by the backend allowlist).',
        {},
        async () => asToolResult(await client.listNamespaces()),
      ),
      tool('list_nodes', 'List node names in the cluster.', {}, async () =>
        asToolResult(await client.listNodes()),
      ),
      tool(
        'list_replicasets',
        'List ReplicaSets in a namespace.',
        { namespace: z.string().min(1).describe('Kubernetes namespace') },
        async (args) => asToolResult(await client.listReplicaSets(args.namespace)),
      ),
      tool(
        'scale',
        'Scale a Deployment to an explicit replica count.',
        {
          namespace: z.string().min(1).describe('Kubernetes namespace'),
          name: z.string().min(1).describe('Deployment name'),
          replicas: z
            .number()
            .int()
            .min(1)
            .describe('Target replica count (the backend caps this)'),
        },
        async (args) => asToolResult(await client.scale(args.namespace, args.name, args.replicas)),
      ),
      tool(
        'rollout_restart',
        'Trigger a rolling restart of a Deployment.',
        {
          namespace: z.string().min(1).describe('Kubernetes namespace'),
          name: z.string().min(1).describe('Deployment name'),
        },
        async (args) => asToolResult(await client.rolloutRestart(args.namespace, args.name)),
      ),
      tool(
        'pause_rollout',
        'Pause an in-progress Deployment rollout.',
        {
          namespace: z.string().min(1).describe('Kubernetes namespace'),
          name: z.string().min(1).describe('Deployment name'),
        },
        async (args) => asToolResult(await client.pauseRollout(args.namespace, args.name)),
      ),
      tool(
        'resume_rollout',
        'Resume a paused Deployment rollout.',
        {
          namespace: z.string().min(1).describe('Kubernetes namespace'),
          name: z.string().min(1).describe('Deployment name'),
        },
        async (args) => asToolResult(await client.resumeRollout(args.namespace, args.name)),
      ),
      tool(
        'rollback',
        'Roll a Deployment back to a previous revision (revision 0 means the immediately previous revision).',
        {
          namespace: z.string().min(1).describe('Kubernetes namespace'),
          name: z.string().min(1).describe('Deployment name'),
          revision: z.number().int().min(0).describe('Target revision; 0 means previous'),
        },
        async (args) =>
          asToolResult(await client.rollback(args.namespace, args.name, args.revision)),
      ),
    ],
  });
}

export const TOOL_NAMES = [
  'health_check',
  'cluster_info',
  'list_deployments',
  'get_deployment',
  'list_pods',
  'list_events',
  'tail_logs',
  'list_services',
  'list_ingresses',
  'list_hpas',
  'list_namespaces',
  'list_nodes',
  'list_replicasets',
  'scale',
  'rollout_restart',
  'pause_rollout',
  'resume_rollout',
  'rollback',
] as const;

export type AgentToolName = (typeof TOOL_NAMES)[number];

// MCP tool names are namespaced as `mcp__<server>__<tool>` when surfaced to
// the SDK's permission and allowlist machinery.
export function mcpToolName(name: AgentToolName): string {
  return `mcp__${MCP_SERVER_NAME}__${name}`;
}

export const ALLOWED_TOOL_NAMES: string[] = TOOL_NAMES.map(mcpToolName);
