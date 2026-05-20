import { useEffect, useState } from 'react';

import { ChatPanel } from '@/components/ChatPanel';
import { Splitter } from '@/components/Splitter';
import { ZoneMap } from '@/components/ZoneMap';

type Theme = 'dark' | 'light';

const THEME_STORAGE_KEY = 'eks-theme';

function readInitialTheme(): Theme {
  try {
    const stored = localStorage.getItem(THEME_STORAGE_KEY);
    if (stored === 'light' || stored === 'dark') return stored;
  } catch {
    /* localStorage unavailable (private mode, SSR) — fall through */
  }
  return 'dark';
}

export default function App() {
  const [chatWidth, setChatWidth] = useState(560);
  const [theme, setTheme] = useState<Theme>(readInitialTheme);

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme);
    try {
      localStorage.setItem(THEME_STORAGE_KEY, theme);
    } catch {
      /* storage may be disabled — theme still applied for the session */
    }
  }, [theme]);

  const nextTheme: Theme = theme === 'dark' ? 'light' : 'dark';

  return (
    <div className="app">
      <header className="app-header">
        <div className="app-header-l">
          <div className="app-brand">
            <span className="app-brand-mark" aria-hidden>
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none">
                <path
                  d="M8 1 L14 4.5 V11.5 L8 15 L2 11.5 V4.5 Z"
                  stroke="white"
                  strokeWidth="1.4"
                />
                <circle cx="8" cy="8" r="2" fill="white" />
              </svg>
            </span>
            <div className="app-brand-title">EKS Control Plane</div>
          </div>
        </div>

        <button
          type="button"
          className="app-theme-toggle"
          onClick={() => setTheme(nextTheme)}
          aria-label={`Switch to ${nextTheme} mode`}
          title={`Switch to ${nextTheme} mode`}
        >
          <span className={`app-theme-track app-theme-${theme}`}>
            <span className="app-theme-thumb">
              {theme === 'dark' ? (
                <svg width="11" height="11" viewBox="0 0 16 16" fill="none">
                  <path d="M13 9.5 A6 6 0 0 1 6.5 3 a5.5 5.5 0 1 0 6.5 6.5 z" fill="currentColor" />
                </svg>
              ) : (
                <svg width="11" height="11" viewBox="0 0 16 16" fill="none">
                  <circle cx="8" cy="8" r="3" fill="currentColor" />
                  <g stroke="currentColor" strokeWidth="1.4" strokeLinecap="round">
                    <path d="M8 1.5 v1.8" />
                    <path d="M8 12.7 v1.8" />
                    <path d="M1.5 8 h1.8" />
                    <path d="M12.7 8 h1.8" />
                    <path d="M3.4 3.4 l1.3 1.3" />
                    <path d="M11.3 11.3 l1.3 1.3" />
                    <path d="M12.6 3.4 l-1.3 1.3" />
                    <path d="M4.7 11.3 l-1.3 1.3" />
                  </g>
                </svg>
              )}
            </span>
          </span>
        </button>
      </header>

      <div className="app-main" style={{ gridTemplateColumns: `1fr 1px ${chatWidth}px` }}>
        <main className="app-cluster-region-l">
          <ZoneMap />
        </main>
        <Splitter axis="v" current={chatWidth} setCurrent={setChatWidth} min={300} max={720} />
        <aside className="app-chat-region">
          <ChatPanel />
        </aside>
      </div>
    </div>
  );
}
