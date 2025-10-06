import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  return {
    plugins: [react()],
    define: {
      __APP_VERSION__: JSON.stringify('0.1.0'),
    },
    server: {
      port: 1000,
      host: '0.0.0.0',
      allowedHosts: ['soft-ravens-float.loca.lt', '.loca.lt']
    },
    build: {
      outDir: 'dist'
    },
    envPrefix: 'VITE_'
  };
});
