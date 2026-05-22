// Entrypoint: load config, build dependencies, mount routes, listen.

import express from 'express';

import { BackendClient } from './backendClient.js';
import { loadConfig } from './config.js';
import { createLogger } from './logging.js';
import { createChatHandler } from './orchestrator/chat.js';
import { createShutdownHandler } from './shutdown.js';

function main(): void {
  const config = loadConfig();
  const logger = createLogger(config.logLevel);

  const backendClient = new BackendClient(config.backendUrl);
  const chatHandler = createChatHandler({
    apiKey: config.anthropicApiKey,
    client: backendClient,
    logger,
  });

  const app = express();
  app.use(express.json({ limit: '1mb' }));

  // Two health paths because the ALB Ingress routes /api/agent/* straight to
  // this runtime without stripping the prefix, so /api/agent/health needs an
  // explicit handler. /health stays for backward-compat and in-cluster probes.
  const healthOk = (_request: express.Request, response: express.Response): void => {
    response.json({ status: 'ok' });
  };
  app.get('/health', healthOk);
  app.get('/api/agent/health', healthOk);

  app.post('/api/agent/chat', (request, response) => {
    chatHandler(request, response).catch((caught) => {
      const reason = caught instanceof Error ? caught.message : String(caught);
      logger.error('agent.chat_unhandled', { reason });
      if (!response.headersSent) {
        response.status(500).json({ error: reason });
      } else if (!response.writableEnded) {
        response.end();
      }
    });
  });

  const server = app.listen(config.port, () => {
    logger.info('agent_runtime.listening', {
      port: config.port,
      backendUrl: config.backendUrl,
    });
  });

  const shutdown = createShutdownHandler({
    server,
    logger,
    exit: (code) => process.exit(code),
  });
  process.on('SIGTERM', shutdown);
  process.on('SIGINT', shutdown);
}

main();
