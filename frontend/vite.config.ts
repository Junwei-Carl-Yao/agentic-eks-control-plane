import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'node:path';

// Order matters: the more specific /api/agent rule must precede /api so SSE
// requests for the runtime are not swallowed by the backend proxy.
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, 'src'),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api/agent': {
        target: 'http://localhost:8081',
        changeOrigin: true,
        ws: false,
      },
      '/api': {
        target: 'http://localhost:8000',
        changeOrigin: true,
      },
    },
  },
});
