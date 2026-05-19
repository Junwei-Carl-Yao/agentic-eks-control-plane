import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import path from 'node:path';

// Spec (§5.2): "types mirror backend Go API models". We assert the field names
// on the TS interfaces match the json tags from
// backend/internal/kubernetes/types.go for representative DTOs (Deployment,
// Pod, Event, Service, Node). If a field is renamed, that's a real spec
// mismatch — flag it.

const typesPath = path.resolve(__dirname, '../../types/index.ts');
const typesSource = readFileSync(typesPath, 'utf8');

// Pull all `name: type;` lines out of an interface body. Plenty good enough for
// the contract here — we only care about the property names, not their types.
function fieldNamesOf(interfaceName: string): string[] {
  // type aliases ('export type Foo = ...') have no field list.
  const pattern = new RegExp(
    String.raw`export\s+interface\s+${interfaceName}\s*\{([\s\S]*?)\}`,
    'm',
  );
  const match = typesSource.match(pattern);
  if (!match) {
    throw new Error(`interface ${interfaceName} not found in types/index.ts`);
  }
  const body = match[1];
  const lineRegex = /^\s*([A-Za-z_][A-Za-z0-9_]*)\??\s*:/gm;
  const names: string[] = [];
  let lineMatch: RegExpExecArray | null;
  while ((lineMatch = lineRegex.exec(body)) !== null) {
    names.push(lineMatch[1]);
  }
  return names.sort();
}

describe('TS types mirror backend Go DTOs (json tag parity)', () => {
  // From backend/internal/kubernetes/types.go Deployment json tags:
  //   name, namespace, replicas, availableReplicas, updatedReplicas, paused
  it('Deployment has the same fields as the Go DTO', () => {
    expect(fieldNamesOf('Deployment')).toEqual(
      ['availableReplicas', 'name', 'namespace', 'paused', 'replicas', 'updatedReplicas'].sort(),
    );
  });

  // Go Pod: name, namespace, phase, labels (omitempty), nodeName (omitempty),
  // restartCount, createdAt, cpuUsage, memoryUsage (host-relative fractions).
  it('Pod has the same fields as the Go DTO', () => {
    expect(fieldNamesOf('Pod')).toEqual(
      [
        'cpuUsage',
        'createdAt',
        'labels',
        'memoryUsage',
        'name',
        'namespace',
        'nodeName',
        'phase',
        'restartCount',
      ].sort(),
    );
  });

  // Go Event: namespace, reason, message, type, time, object (omitempty).
  // Frontend names this ClusterEvent (Event collides with the DOM type) but
  // the wire field names must still match the Go json tags.
  it('ClusterEvent has the same fields as the Go Event DTO', () => {
    expect(fieldNamesOf('ClusterEvent')).toEqual(
      ['message', 'namespace', 'object', 'reason', 'time', 'type'].sort(),
    );
  });

  // Go Service: name, namespace, type, clusterIP, ports
  it('Service has the same fields as the Go DTO', () => {
    expect(fieldNamesOf('Service')).toEqual(
      ['clusterIP', 'name', 'namespace', 'ports', 'type'].sort(),
    );
  });

  // Go Node: name, zone (omitempty), instanceType (omitempty), podCapacity,
  // cpuCapacity (omitempty), memoryCapacity (omitempty), ready. Addresses and
  // arbitrary labels stay off the wire.
  it('Node carries topology, capacity, and live usage but no addresses/labels', () => {
    expect(fieldNamesOf('Node')).toEqual(
      [
        'cpuCapacity',
        'cpuUsage',
        'instanceType',
        'memoryCapacity',
        'memoryUsage',
        'name',
        'podCapacity',
        'ready',
        'zone',
      ].sort(),
    );
  });

  // Go ClusterInfo: name, region, healthy.
  it('ClusterInfo has the same fields as the Go DTO', () => {
    expect(fieldNamesOf('ClusterInfo')).toEqual(['healthy', 'name', 'region'].sort());
  });

  // Go Namespace: name, phase
  it('Namespace has the same fields as the Go DTO', () => {
    expect(fieldNamesOf('Namespace')).toEqual(['name', 'phase']);
  });

  // Go ServicePort: name, port, targetPort, protocol, nodePort
  it('ServicePort has the same fields as the Go DTO', () => {
    expect(fieldNamesOf('ServicePort')).toEqual(
      ['name', 'nodePort', 'port', 'protocol', 'targetPort'].sort(),
    );
  });
});
