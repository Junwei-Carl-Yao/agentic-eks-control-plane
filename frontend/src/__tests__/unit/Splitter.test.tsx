import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render } from '@testing-library/react';

import { Splitter } from '@/components/Splitter';

// Spec (chat1.md):
//   "Add draggable vertical splitter between cluster area and chat panel" — width
//     range 300-720 (vertical bar, axis='v', subtract delta on right-drag).
//   "Add draggable horizontal splitter between zones and bottom panel" — height
//     range 120-500 (horizontal bar, axis='h', subtract delta on down-drag).
//   The bar should "show a thin highlight on hover, glow sky-blue while dragging,
//     and the body cursor flips to col-resize / row-resize so you always know
//     what's grabbable."
//
// These tests derive behavior from the spec, not from the implementation —
// we drive the bar with raw mouse events and assert the public side-effects.

function dispatchMouseMove(clientX: number, clientY: number) {
  // Splitter listens on window with addEventListener (a native MouseEvent),
  // not on the React tree. fireEvent on window dispatches correctly.
  fireEvent(window, new MouseEvent('mousemove', { clientX, clientY, bubbles: true }));
}

function dispatchMouseUp() {
  fireEvent(window, new MouseEvent('mouseup', { bubbles: true }));
}

beforeEach(() => {
  document.body.style.cursor = '';
  document.body.style.userSelect = '';
  document.body.classList.remove('zm-dragging');
});

afterEach(() => {
  // Drop any drag listeners the test may have leaked if it failed mid-drag.
  dispatchMouseUp();
  document.body.style.cursor = '';
  document.body.style.userSelect = '';
  document.body.classList.remove('zm-dragging');
});

describe('Splitter (horizontal, between zones and bottom panel)', () => {
  it('shrinks the trailing (bottom) panel when dragged downward — clamped to max 500', () => {
    // Drag the bar 1000px DOWN. Trailing panel height = startValue - delta;
    // startValue 320, delta +1000 → -680, but clamped to min 120 by spec.
    let height = 320;
    const setHeight = vi.fn((next: number) => {
      height = next;
    });
    const { container } = render(
      <Splitter axis="h" current={height} setCurrent={setHeight} min={120} max={500} />,
    );
    const bar = container.querySelector('.zm-splitter') as HTMLDivElement;
    expect(bar).not.toBeNull();
    fireEvent.mouseDown(bar, { clientY: 500, clientX: 0 });
    dispatchMouseMove(0, 1500); // delta +1000
    dispatchMouseUp();

    // setCurrent should have been called with the clamped lower bound.
    const lastCall = setHeight.mock.calls.at(-1);
    expect(lastCall?.[0]).toBe(120);
  });

  it('grows the trailing panel up to its 500 max when dragged upward beyond max', () => {
    let height = 320;
    const setHeight = vi.fn((next: number) => {
      height = next;
    });
    const { container } = render(
      <Splitter axis="h" current={height} setCurrent={setHeight} min={120} max={500} />,
    );
    const bar = container.querySelector('.zm-splitter') as HTMLDivElement;
    fireEvent.mouseDown(bar, { clientY: 500, clientX: 0 });
    dispatchMouseMove(0, -1000); // delta -1000 → 320+1000=1320 → clamped to 500
    dispatchMouseUp();
    const lastCall = setHeight.mock.calls.at(-1);
    expect(lastCall?.[0]).toBe(500);
  });

  it('produces values inside the [min,max] band for small drags', () => {
    const setHeight = vi.fn();
    const { container } = render(
      <Splitter axis="h" current={300} setCurrent={setHeight} min={120} max={500} />,
    );
    const bar = container.querySelector('.zm-splitter') as HTMLDivElement;
    fireEvent.mouseDown(bar, { clientY: 400, clientX: 0 });
    dispatchMouseMove(0, 450); // delta +50 → 300-50 = 250
    dispatchMouseUp();
    const lastCall = setHeight.mock.calls.at(-1);
    expect(lastCall?.[0]).toBe(250);
  });

  it('sets the body cursor to row-resize during a horizontal drag and clears it on mouseup', () => {
    const { container } = render(
      <Splitter axis="h" current={300} setCurrent={() => {}} min={120} max={500} />,
    );
    const bar = container.querySelector('.zm-splitter') as HTMLDivElement;
    fireEvent.mouseDown(bar, { clientY: 400, clientX: 0 });
    expect(document.body.style.cursor).toBe('row-resize');
    expect(document.body.classList.contains('zm-dragging')).toBe(true);
    expect(document.body.style.userSelect).toBe('none');
    dispatchMouseUp();
    expect(document.body.style.cursor).toBe('');
    expect(document.body.classList.contains('zm-dragging')).toBe(false);
    expect(document.body.style.userSelect).toBe('');
  });
});

describe('Splitter (vertical, between cluster region and chat panel)', () => {
  it('shrinks the trailing (chat) panel down to 300 when dragged right beyond min', () => {
    // Spec: vertical splitter width range 300-720. Trailing panel = chat (right).
    // Dragging right increases clientX → delta positive → width = start - delta.
    let width = 420;
    const setWidth = vi.fn((next: number) => {
      width = next;
    });
    const { container } = render(
      <Splitter axis="v" current={width} setCurrent={setWidth} min={300} max={720} />,
    );
    const bar = container.querySelector('.zm-splitter') as HTMLDivElement;
    fireEvent.mouseDown(bar, { clientX: 1000, clientY: 0 });
    dispatchMouseMove(2000, 0); // delta +1000 → clamped to min 300
    dispatchMouseUp();
    expect(setWidth.mock.calls.at(-1)?.[0]).toBe(300);
  });

  it('grows the chat panel up to its 720 max when dragged left beyond max', () => {
    let width = 420;
    const setWidth = vi.fn((next: number) => {
      width = next;
    });
    const { container } = render(
      <Splitter axis="v" current={width} setCurrent={setWidth} min={300} max={720} />,
    );
    const bar = container.querySelector('.zm-splitter') as HTMLDivElement;
    fireEvent.mouseDown(bar, { clientX: 1000, clientY: 0 });
    dispatchMouseMove(0, 0); // delta -1000 → 420+1000=1420 → clamped to 720
    dispatchMouseUp();
    expect(setWidth.mock.calls.at(-1)?.[0]).toBe(720);
  });

  it('sets the body cursor to col-resize during a vertical drag and clears it on mouseup', () => {
    const { container } = render(
      <Splitter axis="v" current={420} setCurrent={() => {}} min={300} max={720} />,
    );
    const bar = container.querySelector('.zm-splitter') as HTMLDivElement;
    fireEvent.mouseDown(bar, { clientX: 800, clientY: 0 });
    expect(document.body.style.cursor).toBe('col-resize');
    dispatchMouseUp();
    expect(document.body.style.cursor).toBe('');
  });

  it('stops listening for mousemove after mouseup (no leaked drag state)', () => {
    const setWidth = vi.fn();
    const { container } = render(
      <Splitter axis="v" current={420} setCurrent={setWidth} min={300} max={720} />,
    );
    const bar = container.querySelector('.zm-splitter') as HTMLDivElement;
    fireEvent.mouseDown(bar, { clientX: 500, clientY: 0 });
    dispatchMouseMove(450, 0); // one tracked move
    const callsAfterFirstMove = setWidth.mock.calls.length;
    dispatchMouseUp();
    dispatchMouseMove(100, 0); // should NOT be tracked
    expect(setWidth.mock.calls.length).toBe(callsAfterFirstMove);
  });
});

describe('Splitter — accessibility surface', () => {
  it('exposes a separator role with the correct aria-orientation per axis', () => {
    const horizontal = render(
      <Splitter axis="h" current={300} setCurrent={() => {}} min={120} max={500} />,
    );
    const horizontalBar = horizontal.container.querySelector('.zm-splitter') as HTMLDivElement;
    expect(horizontalBar.getAttribute('aria-orientation')).toBe('horizontal');
    horizontal.unmount();

    const vertical = render(
      <Splitter axis="v" current={420} setCurrent={() => {}} min={300} max={720} />,
    );
    const verticalBar = vertical.container.querySelector('.zm-splitter') as HTMLDivElement;
    expect(verticalBar.getAttribute('aria-orientation')).toBe('vertical');
  });
});
