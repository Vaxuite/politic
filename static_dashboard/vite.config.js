import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// For GitHub Pages on the default project subpath, set VITE_BASE=/politic/
// (or whatever your repo name is). Defaults to '/' so `npm run dev` works.
export default defineConfig({
  plugins: [vue()],
  base: process.env.VITE_BASE || '/',
  optimizeDeps: {
    // duckdb-wasm ships browser-only ESM that Vite needs to leave alone.
    exclude: ['@duckdb/duckdb-wasm'],
  },
  worker: { format: 'es' },
  server: { port: 5174 },
})
