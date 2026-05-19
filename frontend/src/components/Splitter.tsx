import type { MouseEvent as ReactMouseEvent } from 'react';

interface SplitterProps {
  axis: 'v' | 'h';
  current: number;
  setCurrent: (next: number) => void;
  min: number;
  max: number;
}

// Drag handler: the splitter resizes the trailing panel (chat on the right,
// bottom strip below the zones). Moving the bar toward the edge shrinks the
// trailing panel, so we subtract the delta from the starting value.
export function Splitter({ axis, current, setCurrent, min, max }: SplitterProps) {
  const onMouseDown = (event: ReactMouseEvent<HTMLDivElement>) => {
    event.preventDefault();
    const startPosition = axis === 'v' ? event.clientX : event.clientY;
    const startValue = current;

    const onMove = (moveEvent: MouseEvent) => {
      const delta = (axis === 'v' ? moveEvent.clientX : moveEvent.clientY) - startPosition;
      const next = Math.max(min, Math.min(max, startValue - delta));
      setCurrent(next);
    };
    const onUp = () => {
      window.removeEventListener('mousemove', onMove);
      window.removeEventListener('mouseup', onUp);
      document.body.style.cursor = '';
      document.body.style.userSelect = '';
      document.body.classList.remove('zm-dragging');
    };

    document.body.style.cursor = axis === 'v' ? 'col-resize' : 'row-resize';
    document.body.style.userSelect = 'none';
    document.body.classList.add('zm-dragging');
    window.addEventListener('mousemove', onMove);
    window.addEventListener('mouseup', onUp);
  };

  return (
    <div
      className={`zm-splitter zm-splitter-${axis}`}
      onMouseDown={onMouseDown}
      role="separator"
      aria-orientation={axis === 'v' ? 'vertical' : 'horizontal'}
      aria-hidden
    />
  );
}
