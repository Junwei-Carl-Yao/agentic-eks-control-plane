// Internal smoke test: starts the mock backend, launches the runtime as a
// subprocess pointed at the mock, and POSTs a single chat request to verify
// the SSE stream produces the expected frame types. Not part of the eval
// harness — used only to verify the wiring.

import { spawn } from "node:child_process";
import { setTimeout as sleep } from "node:timers/promises";

import { startMockBackend } from "./evals/mockBackend.js";

const mock = await startMockBackend();
process.stdout.write(`mock backend on ${mock.url}\n`);

const runtime = spawn(process.execPath, ["dist/index.js"], {
  env: {
    ...process.env,
    PORT: "8082",
    BACKEND_URL: mock.url,
  },
  stdio: "inherit",
});

await sleep(1500);

let exitCode = 0;
try {
  const response = await fetch("http://127.0.0.1:8082/api/agent/chat", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      transcript: [],
      message: "Scale web to 2 replicas in api-smoke",
    }),
  });
  process.stdout.write(`HTTP status: ${response.status}\n`);
  if (!response.body) throw new Error("response has no body");
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let frames = 0;
  let sawTextFrame = false;
  let sawToolCall = false;
  let sawDone = false;
  const startTime = Date.now();
  while (!sawDone && Date.now() - startTime < 60_000) {
    const next = await reader.read();
    if (next.done) break;
    const chunk = decoder.decode(next.value);
    for (const line of chunk.split(/\r?\n/)) {
      if (line.startsWith("data: ")) {
        frames += 1;
        const payload = line.slice(6);
        if (payload.includes('"type":"tool_call"')) sawToolCall = true;
        if (payload.includes('"type":"text"')) sawTextFrame = true;
        if (payload.includes('"type":"done"')) sawDone = true;
      }
    }
  }
  process.stdout.write(
    `frames=${frames} text=${sawTextFrame} tool_call=${sawToolCall} done=${sawDone}\n`,
  );
  process.stdout.write(`recorded calls on backend: ${mock.calls.length}\n`);
  if (!sawDone || !sawToolCall) {
    exitCode = 1;
  }
} catch (caught) {
  process.stderr.write(`smoke failure: ${caught instanceof Error ? caught.message : String(caught)}\n`);
  exitCode = 1;
} finally {
  runtime.kill();
  await mock.stop();
}

process.exit(exitCode);
