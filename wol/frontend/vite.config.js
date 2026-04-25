import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: 'dist',
    assetsDir: 'assets'
  },
  define: {
    'import.meta.env.VITE_VERSION': JSON.stringify(process.env.npm_package_version || '1.0.0')
  }
})
