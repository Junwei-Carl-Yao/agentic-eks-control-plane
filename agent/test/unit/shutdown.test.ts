// Verifies the SIGTERM/SIGINT shutdown handler: it must call server.close()
// and arm an unref'd force-exit timer, exiting with the right code based on
// whether close() reports an error.

import { describe, expect, it, vi } from 'vitest';

import { createLogger } from '../../src/logging.js';
import {
  createShutdownHandler,
  shutdownGracePeriodMs,
  type ClosableServer,
} from '../../src/shutdown.js';

interface RecordedTimer {
  callback: () => void;
  delayMs: number;
  unrefCount: number;
}

function makeFakeTimer(): {
  schedule: (callback: () => void, delayMs: number) => { unref(): void };
  timers: RecordedTimer[];
} {
  const timers: RecordedTimer[] = [];
  return {
    schedule: (callback, delayMs) => {
      const record: RecordedTimer = { callback, delayMs, unrefCount: 0 };
      timers.push(record);
      return {
        unref() {
          record.unrefCount += 1;
        },
      };
    },
    timers,
  };
}

function silentLogger(): ReturnType<typeof createLogger> {
  return {
    debug: vi.fn(),
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
  };
}

describe('createShutdownHandler', () => {
  it('calls server.close and arms an unref-ed force-exit timer on signal', () => {
    let closeCallback: ((closeError?: Error) => void) | undefined;
    const fakeServer: ClosableServer = {
      close: vi.fn((callback?: (closeError?: Error) => void) => {
        closeCallback = callback;
      }),
    };
    const logger = silentLogger();
    const exit = vi.fn();
    const timerFactory = makeFakeTimer();

    const shutdown = createShutdownHandler({
      server: fakeServer,
      logger,
      exit,
      scheduleForceExit: timerFactory.schedule,
    });

    shutdown('SIGTERM');

    expect(fakeServer.close).toHaveBeenCalledTimes(1);

    expect(timerFactory.timers).toHaveLength(1);
    const forceExitTimer = timerFactory.timers[0]!;
    expect(forceExitTimer.delayMs).toBe(shutdownGracePeriodMs);
    expect(forceExitTimer.unrefCount).toBe(1);

    expect(logger.info).toHaveBeenCalledWith('agent_runtime.shutdown_signal', {
      signal: 'SIGTERM',
    });
    expect(exit).not.toHaveBeenCalled();

    // Now run the close callback as if the server finished draining.
    expect(closeCallback).toBeDefined();
    closeCallback!();
    expect(exit).toHaveBeenCalledWith(0);
  });

  it('exits 1 when server.close reports an error', () => {
    let closeCallback: ((closeError?: Error) => void) | undefined;
    const fakeServer: ClosableServer = {
      close: vi.fn((callback?: (closeError?: Error) => void) => {
        closeCallback = callback;
      }),
    };
    const logger = silentLogger();
    const exit = vi.fn();
    const timerFactory = makeFakeTimer();

    const shutdown = createShutdownHandler({
      server: fakeServer,
      logger,
      exit,
      scheduleForceExit: timerFactory.schedule,
    });

    shutdown('SIGINT');

    closeCallback!(new Error('socket leak'));

    expect(logger.error).toHaveBeenCalledWith('agent_runtime.close_failed', {
      reason: 'socket leak',
    });
    expect(exit).toHaveBeenCalledWith(1);
  });

  it('force-exit timer logs and exits 1 when fired (close never returns)', () => {
    const fakeServer: ClosableServer = {
      close: vi.fn(),
    };
    const logger = silentLogger();
    const exit = vi.fn();
    const timerFactory = makeFakeTimer();

    const shutdown = createShutdownHandler({
      server: fakeServer,
      logger,
      exit,
      scheduleForceExit: timerFactory.schedule,
    });

    shutdown('SIGTERM');

    // Simulate the kubelet hitting us with shutdownGracePeriodMs and close()
    // never invoking its callback.
    timerFactory.timers[0]!.callback();

    expect(logger.warn).toHaveBeenCalledWith('agent_runtime.forced_exit', {
      graceMs: shutdownGracePeriodMs,
    });
    expect(exit).toHaveBeenCalledWith(1);
  });
});
