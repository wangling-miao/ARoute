import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'node:path'

export default defineConfig({
  plugins: [react()],
  base: '/admin/',
  root: path.resolve(__dirname, '../admin'),
  define: {
    __VARIANT__: JSON.stringify('compact'),
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, '../admin/src'),
    },
  },
  build: {
    outDir: path.resolve(__dirname, 'dist'),
    emptyOutDir: true,
    sourcemap: false,
  },
})
