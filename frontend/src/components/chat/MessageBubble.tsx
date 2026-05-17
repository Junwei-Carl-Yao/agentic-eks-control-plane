import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

interface MessageBubbleProps {
  role: 'user' | 'assistant';
  content: string;
  pending?: boolean;
}

export function MessageBubble({ role, content, pending }: MessageBubbleProps) {
  const isUser = role === 'user';
  return (
    <div className={isUser ? 'flex justify-end' : 'flex justify-start'}>
      <div
        className={
          isUser
            ? 'max-w-[85%] rounded-lg bg-sky-600/90 px-3 py-2 text-sm text-white'
            : 'max-w-[85%] rounded-lg bg-slate-800 px-3 py-2 text-sm text-slate-100'
        }
      >
        {content.length > 0 && isUser ? (
          <div className="whitespace-pre-wrap">{content}</div>
        ) : content.length > 0 ? (
          <AssistantMarkdown content={content} />
        ) : pending ? (
          <TypingDots />
        ) : null}
      </div>
    </div>
  );
}

function AssistantMarkdown({ content }: { content: string }) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      components={{
        a({ children, href }) {
          return (
            <a
              className="text-sky-300 underline decoration-sky-300/50 underline-offset-2 hover:text-sky-200"
              href={href}
              rel="noreferrer"
              target="_blank"
            >
              {children}
            </a>
          );
        },
        blockquote({ children }) {
          return (
            <blockquote className="border-l-2 border-slate-500 pl-3 text-slate-300">
              {children}
            </blockquote>
          );
        },
        code({ children, className }) {
          return (
            <code
              className={`rounded bg-slate-950/80 px-1 py-0.5 font-mono text-xs text-sky-100 ${className ?? ''}`}
            >
              {children}
            </code>
          );
        },
        h1({ children }) {
          return <h1 className="text-lg font-semibold text-white">{children}</h1>;
        },
        h2({ children }) {
          return <h2 className="text-base font-semibold text-white">{children}</h2>;
        },
        h3({ children }) {
          return <h3 className="text-sm font-semibold text-white">{children}</h3>;
        },
        h4({ children }) {
          return <h4 className="text-sm font-semibold text-white">{children}</h4>;
        },
        h5({ children }) {
          return <h5 className="text-sm font-semibold text-white">{children}</h5>;
        },
        h6({ children }) {
          return <h6 className="text-sm font-semibold text-white">{children}</h6>;
        },
        input({ checked, type }) {
          return (
            <input
              checked={checked}
              className="mr-2 align-middle accent-sky-500"
              disabled
              readOnly
              type={type}
            />
          );
        },
        li({ children }) {
          return <li className="pl-1">{children}</li>;
        },
        ol({ children }) {
          return <ol className="list-decimal space-y-1 pl-5">{children}</ol>;
        },
        p({ children }) {
          return <p className="whitespace-pre-wrap">{children}</p>;
        },
        pre({ children }) {
          return (
            <pre className="overflow-x-auto rounded bg-slate-950/80 px-2 py-1.5 text-slate-100 [&_code]:bg-transparent [&_code]:p-0 [&_code]:text-slate-100">
              {children}
            </pre>
          );
        },
        strong({ children }) {
          return <strong className="font-semibold text-white">{children}</strong>;
        },
        table({ children }) {
          return (
            <div className="overflow-x-auto">
              <table className="min-w-full border-collapse text-left text-xs">{children}</table>
            </div>
          );
        },
        td({ children }) {
          return <td className="border border-slate-700 px-2 py-1">{children}</td>;
        },
        th({ children }) {
          return (
            <th className="border border-slate-600 bg-slate-900 px-2 py-1 font-semibold text-slate-100">
              {children}
            </th>
          );
        },
        ul({ children }) {
          return <ul className="list-disc space-y-1 pl-5">{children}</ul>;
        },
      }}
    >
      {content}
    </ReactMarkdown>
  );
}

function TypingDots() {
  return (
    <div className="flex items-center gap-1">
      <Dot delayMs={0} />
      <Dot delayMs={150} />
      <Dot delayMs={300} />
    </div>
  );
}

function Dot({ delayMs }: { delayMs: number }) {
  return (
    <span
      className="inline-block h-1.5 w-1.5 animate-bounce rounded-full bg-slate-400"
      style={{ animationDelay: `${delayMs}ms` }}
    />
  );
}
