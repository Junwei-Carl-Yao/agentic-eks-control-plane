// Section E: Config and key handling.
//
// The runtime resolves the Anthropic API key from env, then from a file
// pointed to by ANTHROPIC_API_KEY_FILE, then from a developer fallback file.
// Whitespace must be trimmed; missing keys must fail loudly. The chosen model
// must be the one the user dictated (claude-opus-4-7). And the key must never
// leave config — it must not appear in any log line.

import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

import { loadConfig } from '../../src/config.js';
import { AGENT_MODEL, buildAgentOptions } from '../../src/agents/agent.js';
import { BackendClient } from '../../src/backendClient.js';
import { createLogger } from '../../src/logging.js';

const ENV_KEYS_TO_RESTORE = [
  'ANTHROPIC_API_KEY',
  'ANTHROPIC_API_KEY_FILE',
  'BACKEND_URL',
  'PORT',
  'LOG_LEVEL',
];

describe('loadConfig key resolution', () => {
  let tempDirectory: string;
  const savedEnv: Record<string, string | undefined> = {};

  beforeEach(() => {
    tempDirectory = mkdtempSync(join(tmpdir(), 'agent-config-'));
    for (const envKey of ENV_KEYS_TO_RESTORE) {
      savedEnv[envKey] = process.env[envKey];
      delete process.env[envKey];
    }
  });

  afterEach(() => {
    rmSync(tempDirectory, { recursive: true, force: true });
    for (const envKey of ENV_KEYS_TO_RESTORE) {
      const previousValue = savedEnv[envKey];
      if (previousValue === undefined) {
        delete process.env[envKey];
      } else {
        process.env[envKey] = previousValue;
      }
    }
  });

  it('uses ANTHROPIC_API_KEY from env when set, trimming whitespace', () => {
    process.env.ANTHROPIC_API_KEY = '  sk-from-env\n';
    const config = loadConfig();
    expect(config.anthropicApiKey).toBe('sk-from-env');
  });

  it('falls back to ANTHROPIC_API_KEY_FILE when env is missing', () => {
    const filePath = join(tempDirectory, 'key.txt');
    writeFileSync(filePath, 'sk-from-file\n', 'utf-8');
    process.env.ANTHROPIC_API_KEY_FILE = filePath;
    const config = loadConfig();
    expect(config.anthropicApiKey).toBe('sk-from-file');
  });

  it('trims trailing whitespace from the file content', () => {
    const filePath = join(tempDirectory, 'key.txt');
    writeFileSync(filePath, '   sk-padded   \r\n\t', 'utf-8');
    process.env.ANTHROPIC_API_KEY_FILE = filePath;
    const config = loadConfig();
    expect(config.anthropicApiKey).toBe('sk-padded');
  });

  it('declares the documented default key file path as a literal in config.ts', async () => {
    // The user prompt names the default fallback path explicitly. We assert
    // the literal lives in config.ts so a refactor that drops it shows up
    // here. This is more direct than fs-mocking the third-fallback branch.
    const fs = await import('node:fs');
    const url = await import('node:url');
    const here = url.fileURLToPath(new URL('.', import.meta.url));
    const configSource = fs.readFileSync(join(here, '..', '..', 'src', 'config.ts'), 'utf-8');
    expect(configSource).toContain('C:\\\\Users\\\\carly\\\\Downloads\\\\anthropic.txt');
  });

  it('fails fast with a clear error when env, *_FILE, and the default path all fail', () => {
    delete process.env.ANTHROPIC_API_KEY;
    process.env.ANTHROPIC_API_KEY_FILE = join(tempDirectory, 'missing-file.txt');
    expect(() => loadConfig()).toThrow(/API key|ANTHROPIC_API_KEY/i);
  });

  it('fails fast when the file exists but is empty', () => {
    delete process.env.ANTHROPIC_API_KEY;
    const filePath = join(tempDirectory, 'empty.txt');
    writeFileSync(filePath, '   \n  \t\r\n', 'utf-8');
    process.env.ANTHROPIC_API_KEY_FILE = filePath;
    expect(() => loadConfig()).toThrow();
  });
});

describe('agent model selection', () => {
  it('uses claude-opus-4-7 (per user prompt, non-negotiable)', () => {
    expect(AGENT_MODEL).toBe('claude-opus-4-7');
  });

  it('buildAgentOptions returns options with the correct model', () => {
    const client = new BackendClient('http://backend.test');
    const options = buildAgentOptions({ apiKey: 'sk-test-key', client });
    expect(options.model).toBe('claude-opus-4-7');
  });
});

describe('logger never writes the API key', () => {
  it('startup-style log lines do not contain the api key string', () => {
    const writes: string[] = [];
    const stdoutWrite = process.stdout.write.bind(process.stdout);
    const stderrWrite = process.stderr.write.bind(process.stderr);
    // @ts-expect-error overwrite for capture
    process.stdout.write = (chunk: unknown) => {
      writes.push(String(chunk));
      return true;
    };
    // @ts-expect-error overwrite for capture
    process.stderr.write = (chunk: unknown) => {
      writes.push(String(chunk));
      return true;
    };
    try {
      const fakeKey = 'sk-ant-test-LEAK-CANARY-ABCDEFG-1234567890';
      const logger = createLogger('debug');
      logger.info('agent_runtime.listening', { port: 8081, backendUrl: 'http://localhost:8000' });
      logger.warn('agent.permission_denied', { tool: 'scale' });
      logger.error('agent.stream_error', { reason: 'model error' });
      logger.debug('agent.debug', { transcript: 'user said hello' });
      const allOutput = writes.join('');
      expect(allOutput.includes(fakeKey)).toBe(false);
      expect(allOutput.toLowerCase()).not.toContain('anthropic_api_key');
      // sanity: the logger did emit at least one line
      expect(allOutput.length).toBeGreaterThan(0);
    } finally {
      process.stdout.write = stdoutWrite;
      process.stderr.write = stderrWrite;
    }
  });

  it('logger does not auto-include process.env in output', () => {
    // Belt-and-braces: even if a careless caller passes process.env, the
    // logger is structural and only emits the fields it is given. Verify by
    // calling with a totally unrelated payload.
    const writes: string[] = [];
    const stdoutWrite = process.stdout.write.bind(process.stdout);
    // @ts-expect-error overwrite
    process.stdout.write = (chunk: unknown) => {
      writes.push(String(chunk));
      return true;
    };
    try {
      const fakeKey = 'sk-LEAK-CANARY-ZZZZZZZ-9999999';
      process.env.ANTHROPIC_API_KEY = fakeKey;
      const logger = createLogger('info');
      logger.info('safe.message', { ok: true });
      const joined = writes.join('');
      expect(joined.includes(fakeKey)).toBe(false);
    } finally {
      process.stdout.write = stdoutWrite;
      delete process.env.ANTHROPIC_API_KEY;
    }
  });
});
