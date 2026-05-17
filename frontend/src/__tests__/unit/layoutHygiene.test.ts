import { describe, expect, it } from 'vitest';
import { existsSync, readFileSync } from 'node:fs';
import path from 'node:path';

// Spec (§5.3): single-page operator view with two panes — no router. Project
// rule (CLAUDE.md): "default to removing, not adding" — the legacy
// frontend/src/pages directory must not exist after Phase 5.

const frontendSrc = path.resolve(__dirname, '../../');

describe('layout hygiene', () => {
  it('App.tsx does not import react-router', () => {
    const appSource = readFileSync(path.join(frontendSrc, 'App.tsx'), 'utf8');
    expect(appSource).not.toMatch(/from\s+['"]react-router/);
    expect(appSource).not.toMatch(/<Routes/);
    expect(appSource).not.toMatch(/<Route\b/);
    expect(appSource).not.toMatch(/<BrowserRouter/);
  });

  it('frontend/src/pages directory does NOT exist', () => {
    const pagesDir = path.join(frontendSrc, 'pages');
    expect(existsSync(pagesDir)).toBe(false);
  });
});
