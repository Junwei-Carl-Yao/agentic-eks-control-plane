// Tiny line-buffered SSE parser. We only need the `data:` field — comment
// lines starting with `:` (heartbeats) and any `event:` / `id:` fields are
// dropped. Frames are terminated by a blank line.

export interface SseFrame {
  data: string;
}

export class SseLineParser {
  private buffer = '';
  private dataLines: string[] = [];

  // feed appends a chunk and returns any complete frames it produced.
  feed(chunk: string): SseFrame[] {
    this.buffer += chunk;
    const frames: SseFrame[] = [];
    let newlineIndex = this.buffer.indexOf('\n');
    while (newlineIndex !== -1) {
      let line = this.buffer.slice(0, newlineIndex);
      this.buffer = this.buffer.slice(newlineIndex + 1);
      if (line.endsWith('\r')) {
        line = line.slice(0, -1);
      }
      if (line === '') {
        if (this.dataLines.length > 0) {
          frames.push({ data: this.dataLines.join('\n') });
          this.dataLines = [];
        }
      } else if (line.startsWith(':')) {
        // SSE comment / heartbeat — ignore.
      } else if (line.startsWith('data:')) {
        // Spec says strip exactly one leading space after the colon.
        const value = line.slice(5).startsWith(' ') ? line.slice(6) : line.slice(5);
        this.dataLines.push(value);
      }
      // Other fields (event:, id:, retry:) are ignored — the runtime does not
      // use them.
      newlineIndex = this.buffer.indexOf('\n');
    }
    return frames;
  }
}
