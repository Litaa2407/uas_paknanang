import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/auth': 'http://localhost:8000',
      '/campaigns': 'http://localhost:8000',
      '/donations': 'http://localhost:8000',
      '/graphql': 'http://localhost:8000',
      '/health': 'http://localhost:8000',
      '/registry': 'http://localhost:8000',
      '/webhook': 'http://localhost:8000',
      '/ws': {
        target: 'ws://localhost:8000',
        ws: true,
      },
    },
  },
});
