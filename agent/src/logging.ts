// Structured JSON logger. Intentionally tiny so we never reach for a heavy
// dependency. The API key is loaded in config.ts and is never passed to log
// methods; the only sensitive surface left is user prompts and tool inputs,
// which we keep at debug only.

type Level = 'debug' | 'info' | 'warn' | 'error';

const ORDER: Record<Level, number> = { debug: 10, info: 20, warn: 30, error: 40 };

export interface Logger {
  debug(message: string, fields?: Record<string, unknown>): void;
  info(message: string, fields?: Record<string, unknown>): void;
  warn(message: string, fields?: Record<string, unknown>): void;
  error(message: string, fields?: Record<string, unknown>): void;
}

export function createLogger(minLevel: Level = 'info'): Logger {
  const threshold = ORDER[minLevel];
  function emit(level: Level, message: string, fields?: Record<string, unknown>): void {
    if (ORDER[level] < threshold) return;
    const record = {
      timestamp: new Date().toISOString(),
      level,
      message,
      ...(fields ?? {}),
    };
    const stream = level === 'error' || level === 'warn' ? process.stderr : process.stdout;
    stream.write(JSON.stringify(record) + '\n');
  }
  return {
    debug: (message, fields) => emit('debug', message, fields),
    info: (message, fields) => emit('info', message, fields),
    warn: (message, fields) => emit('warn', message, fields),
    error: (message, fields) => emit('error', message, fields),
  };
}
