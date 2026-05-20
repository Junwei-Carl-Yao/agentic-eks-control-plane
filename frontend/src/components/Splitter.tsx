import { useEffect, useRef, useState } from 'react';
import type { MouseEvent as ReactMouseEvent } from 'react';

interface SplitterProps {
  axis: 'v' | 'h';
  current: number;
  setCurrent: (next: number) => void;
  min: number;
  max: number;
}

interface DragOrigin {
  startPosition: number;
  startValue: number;
}

// Drag handler: the splitter resizes the trailing panel (chat on the right,
// bottom strip below the zones). Moving the bar toward the edge shrinks the
// trailing panel, so we subtract the delta from the starting value.
//
// Listeners live in a useEffect tied to `dragging` state so React owns
// teardown — if the splitter unmounts mid-drag (parent re-layout, theme swap,
// etc.) the cleanup still fires and the document doesn't keep a frozen
// row-resize cursor with stuck mousemove handlers.
export function Splitter({ axis, current, setCurrent, min, max }: SplitterProps) {
  const [dragging, setDragging] = useState(false);
  const dragOriginRef = useRef<DragOrigin | null>(null);

  useEffect(() => {
    if (!dragging) return;
    const origin = dragOriginRef.current;
    if (!origin) return;

    const onMove = (moveEvent: MouseEvent) => {
      const delta = (axis === 'v' ? moveEvent.clientX : moveEvent.clientY) - origin.startPosition;
      const next = Math.max(min, Math.min(max, origin.startValue - delta));
      setCurrent(next);
    };
    const onUp = () => {
      setDragging(false);
    };

    document.body.style.cursor = axis === 'v' ? 'col-resize' : 'row-resize';
    document.body.style.userSelect = 'none';
    document.body.classList.add('zm-dragging');
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);

    return () => {
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', onUp);
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
      document.body.classList.remove('zm-dragging');
    };
  }, [dragging, axis, min, max, setCurrent]);

  const onMouseDown = (event: ReactMouseEvent<HTMLDivElement>) => {
    event.preventDefault();
    dragOriginRef.current = {
      startPosition: axis === 'v' ? event.clientX : event.clientY,
      startValue: current,
    };
    setDragging(true);
  };

  return (
    <div
      className={`zm-splitter zm-splitter-${axis}`}
      onMouseDown={onMouseDown}
      role="separator"
      aria-orientation={axis === 'v' ? 'vertical' : 'horizontal'}
    />
  );
}
