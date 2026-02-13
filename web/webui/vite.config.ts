import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    rollupOptions: {
      output: {
        manualChunks: {
          vendor: ['react', 'react-dom', 'react-router-dom'],
          query: ['@tanstack/react-query'],
          terminal: ['xterm', 'xterm-addon-fit'],
        },
      },
    },
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '^/(cluster|client|aerospike|config|files|inventory|data|net|agi|tls|xdr|attach|volumes|templates|roster|conf|logs|images|installer)': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
