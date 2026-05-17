import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import path from 'node:path';

// Vitest config kept separate from vite.config.ts so we don't accidentally
// teach the dev server about test-only globals.
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, 'src'),
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/__tests__/setup.ts'],
    css: false,
    include: ['src/__tests__/**/*.{test,spec}.{ts,tsx}'],
  },
});
