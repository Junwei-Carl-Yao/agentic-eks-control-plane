import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

interface MessageBubbleProps {
  role: 'user' | 'assistant';
  content: string;
  pending?: boolean;
}

export function MessageBubble({ role, content, pending }: MessageBubbleProps) {
  if (role === 'user') {
    return (
      <div className="cp-row cp-row-user">
        <div className="cp-bubble cp-bubble-user">
          <div style={{ whiteSpace: 'pre-wrap' }}>{content}</div>
        </div>
      </div>
    );
  }

  return (
    <div className="cp-row cp-row-assistant">
      <div className="cp-avatar" aria-hidden>
        <svg viewBox="0 0 20 20" width="16" height="16" fill="none">
          <path d="M10 2 L18 10 L10 18 L2 10 Z" fill="#818cf8" />
        </svg>
      </div>
      <div className={'cp-bubble cp-bubble-assistant' + (pending ? ' cp-bubble-pending' : '')}>
        {pending && content.length === 0 ? (
          <TypingDots />
        ) : (
          <div className="cp-md">
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
          </div>
        )}
      </div>
    </div>
  );
}

function TypingDots() {
  return (
    <>
      <span className="cp-dot" style={{ animationDelay: '0ms' }} />
      <span className="cp-dot" style={{ animationDelay: '150ms' }} />
      <span className="cp-dot" style={{ animationDelay: '300ms' }} />
    </>
  );
}
