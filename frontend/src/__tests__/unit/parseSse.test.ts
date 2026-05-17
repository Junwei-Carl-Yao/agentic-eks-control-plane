import { describe, expect, it } from 'vitest';

import { SseLineParser } from '@/sse/parseSse';

// Spec contract (Phase 4 / Phase 5):
// - frames are `data: <json>\n\n`
// - heartbeats are SSE comments (lines starting with ':') and must be ignored
// - blank lines terminate frames
// - frames may be split across chunks; trailing partial frame must NOT emit
// - parser must not break on multiple frames in one chunk

describe('SseLineParser', () => {
  it('emits a single frame on a complete data + blank-line chunk', () => {
    const parser = new SseLineParser();
    const frames = parser.feed('data: {"type":"text","delta":"hi"}\n\n');
    expect(frames).toHaveLength(1);
    expect(frames[0].data).toBe('{"type":"text","delta":"hi"}');
  });

  it('reassembles a frame whose data field is split across chunks', () => {
    const parser = new SseLineParser();
    const firstChunk = parser.feed('data: {"typ');
    expect(firstChunk).toHaveLength(0);
    const secondChunk = parser.feed('e":"text","delta":"hi"}\n\n');
    expect(secondChunk).toHaveLength(1);
    expect(secondChunk[0].data).toBe('{"type":"text","delta":"hi"}');
  });

  it('emits multiple frames when several arrive in a single chunk', () => {
    const parser = new SseLineParser();
    const frames = parser.feed(
      'data: {"type":"text","delta":"a"}\n\ndata: {"type":"text","delta":"b"}\n\n',
    );
    expect(frames).toHaveLength(2);
    expect(frames[0].data).toBe('{"type":"text","delta":"a"}');
    expect(frames[1].data).toBe('{"type":"text","delta":"b"}');
  });

  it('ignores comment / heartbeat lines starting with ":"', () => {
    const parser = new SseLineParser();
    const frames = parser.feed(': ping\n\ndata: {"type":"done"}\n\n');
    expect(frames).toHaveLength(1);
    expect(frames[0].data).toBe('{"type":"done"}');
  });

  it('does not emit anything when only heartbeats arrive', () => {
    const parser = new SseLineParser();
    const frames = parser.feed(':ping\n\n:another\n\n');
    expect(frames).toHaveLength(0);
  });

  it('does not emit a trailing partial frame missing its blank-line terminator', () => {
    const parser = new SseLineParser();
    const frames = parser.feed('data: {"type":"text","delta":"hi"}\n');
    expect(frames).toHaveLength(0);
  });

  it('preserves text concatenation when a heartbeat sits between two data frames', () => {
    const parser = new SseLineParser();
    const frames = parser.feed(
      'data: {"type":"text","delta":"Hel"}\n\n: keepalive\n\ndata: {"type":"text","delta":"lo"}\n\n',
    );
    expect(frames).toHaveLength(2);
    const decoded = frames.map((frame) => JSON.parse(frame.data));
    const concatenated = decoded
      .map((event) => (event.type === 'text' ? (event.delta as string) : ''))
      .join('');
    expect(concatenated).toBe('Hello');
  });

  it('handles CRLF line terminators as well as LF', () => {
    const parser = new SseLineParser();
    const frames = parser.feed('data: {"type":"done"}\r\n\r\n');
    expect(frames).toHaveLength(1);
    expect(frames[0].data).toBe('{"type":"done"}');
  });

  it('treats an empty data: line as an empty data field', () => {
    const parser = new SseLineParser();
    const frames = parser.feed('data:\n\n');
    expect(frames).toHaveLength(1);
    expect(frames[0].data).toBe('');
  });

  it('accumulates state across feed() calls without losing earlier frames', () => {
    const parser = new SseLineParser();
    const firstFrames = parser.feed('data: {"type":"text","delta":"x"}\n\n');
    const secondFrames = parser.feed('data: {"type":"done"}\n\n');
    expect(firstFrames.map((frame) => frame.data)).toEqual(['{"type":"text","delta":"x"}']);
    expect(secondFrames.map((frame) => frame.data)).toEqual(['{"type":"done"}']);
  });
});
