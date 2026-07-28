import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';

function manualChunks(id) {
  if (id.includes('/node_modules/posthog-js/')) return 'analytics';
  if (id.includes('/node_modules/')) return 'vendor';
  if (id.includes('/src/lib/components/reports/')) return 'reports';

  const locale = id.match(/\/src\/lib\/i18n\/([a-z]+)\.json$/)?.[1];
  if (['en', 'fr', 'es', 'de', 'it', 'pt'].includes(locale)) return 'translations-west';
  if (['nl', 'pl', 'ru', 'tr', 'id'].includes(locale)) return 'translations-europe';
  if (locale) return 'translations-global';

  return undefined;
}

export default defineConfig({
  plugins: [svelte()],
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:8899',
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks,
      },
    },
  },
});
