// Signal-handler factory for the agent runtime. Closes the HTTP server and
// arms a force-exit timer so a stuck connection can't extend the lifetime
// past terminationGracePeriodSeconds. Lives in its own module so tests can
// import it without booting express.

import type { Logger } from './logging.js';

// shutdownGracePeriodMs caps how long shutdown waits for active connections
// (including long-lived SSE chat streams) to finish. Picked to fit inside the
// default Kubernetes terminationGracePeriodSeconds (30s) so SIGKILL never
// beats us when something is genuinely stuck.
export const shutdownGracePeriodMs = 25_000;

export interface ClosableServer {
  close(callback?: (closeError?: Error) => void): void;
}

export interface ShutdownDeps {
  server: ClosableServer;
  logger: Logger;
  exit: (code: number) => void;
  scheduleForceExit?: (callback: () => void, ms: number) => { unref(): void };
}

// createShutdownHandler builds the signal handler that closes the HTTP server
// gracefully and arms a force-exit timer in case close() never returns. The
// dependencies are injectable so a unit test can observe close + timer
// without actually exiting the process.
export function createShutdownHandler(deps: ShutdownDeps): (signal: NodeJS.Signals) => void {
  const scheduleForceExit = deps.scheduleForceExit ?? setTimeout;
  return (signal: NodeJS.Signals): void => {
    deps.logger.info('agent_runtime.shutdown_signal', { signal });
    const forceExit = scheduleForceExit(() => {
      deps.logger.warn('agent_runtime.forced_exit', { graceMs: shutdownGracePeriodMs });
      deps.exit(1);
    }, shutdownGracePeriodMs);
    forceExit.unref();
    deps.server.close((closeError) => {
      if (closeError) {
        deps.logger.error('agent_runtime.close_failed', { reason: closeError.message });
        deps.exit(1);
        return;
      }
      deps.exit(0);
    });
  };
}
