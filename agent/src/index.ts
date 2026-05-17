// Entrypoint: load config, build dependencies, mount routes, listen.

import express from 'express';

import { BackendClient } from './backendClient.js';
import { loadConfig } from './config.js';
import { createLogger } from './logging.js';
import { createChatHandler } from './orchestrator/chat.js';

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

  app.get('/health', (_request, response) => {
    response.json({ status: 'ok' });
  });

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

  app.listen(config.port, () => {
    logger.info('agent_runtime.listening', {
      port: config.port,
      backendUrl: config.backendUrl,
    });
  });
}

main();
