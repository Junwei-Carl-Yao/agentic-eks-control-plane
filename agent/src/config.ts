// Loads runtime configuration from environment and the optional API key file.
// The Anthropic API key is resolved in three stages so the runtime can be
// driven by a CI secret, a mounted file, or a developer fallback path without
// any code change.

import { readFileSync } from 'node:fs';

const DEFAULT_KEY_FILE = 'C:\\Users\\carly\\Downloads\\anthropic.txt';
const DEFAULT_BACKEND_URL = 'http://localhost:8000';
const DEFAULT_PORT = 8081;

export interface RuntimeConfig {
  anthropicApiKey: string;
  backendUrl: string;
  port: number;
  logLevel: 'debug' | 'info' | 'warn' | 'error';
}

export function loadConfig(): RuntimeConfig {
  return {
    anthropicApiKey: resolveApiKey(),
    backendUrl: trimSlash(process.env.BACKEND_URL ?? DEFAULT_BACKEND_URL),
    port: parsePort(process.env.PORT) ?? DEFAULT_PORT,
    logLevel: parseLogLevel(process.env.LOG_LEVEL),
  };
}

function resolveApiKey(): string {
  const fromEnv = process.env.ANTHROPIC_API_KEY?.trim();
  if (fromEnv) {
    return fromEnv;
  }
  const keyFilePath = process.env.ANTHROPIC_API_KEY_FILE?.trim() || DEFAULT_KEY_FILE;
  try {
    const fileContent = readFileSync(keyFilePath, 'utf-8').trim();
    if (fileContent) {
      return fileContent;
    }
    throw new Error(`API key file ${keyFilePath} is empty`);
  } catch (caught) {
    const reason = caught instanceof Error ? caught.message : String(caught);
    throw new Error(
      `ANTHROPIC_API_KEY is not set and the fallback key file could not be read: ${reason}`,
    );
  }
}

function parsePort(raw: string | undefined): number | undefined {
  if (!raw) return undefined;
  const parsed = Number.parseInt(raw, 10);
  if (!Number.isFinite(parsed) || parsed <= 0 || parsed > 65535) {
    throw new Error(`PORT env value is invalid: ${raw}`);
  }
  return parsed;
}

function parseLogLevel(raw: string | undefined): RuntimeConfig['logLevel'] {
  const candidate = (raw ?? 'info').toLowerCase();
  if (
    candidate === 'debug' ||
    candidate === 'info' ||
    candidate === 'warn' ||
    candidate === 'error'
  ) {
    return candidate;
  }
  return 'info';
}

function trimSlash(url: string): string {
  return url.endsWith('/') ? url.slice(0, -1) : url;
}
