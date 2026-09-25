import fs from 'node:fs';
import path from 'node:path';
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { VitePWA } from 'vite-plugin-pwa';
import tailwindcss from 'tailwindcss';
import autoprefixer from 'autoprefixer';

export default defineConfig({
  css: {
    postcss: {
      plugins: [
        tailwindcss(),
        autoprefixer(),
      ],
    },
  },
  plugins: [
    react(),
    {
      name: 'cfasync-disabler',
      enforce: 'post',
      transformIndexHtml(html: string) {
        return html.replaceAll('<script', '<script data-cfasync="false"');
      },
      closeBundle() {
        const indexPath = path.resolve(__dirname, 'dist/index.html');
        if (fs.existsSync(indexPath)) {
          let html = fs.readFileSync(indexPath, 'utf-8');
          html = html.replace(/<script\b(?![^>]*data-cfasync)/gi, '<script data-cfasync="false"');
          fs.writeFileSync(indexPath, html);
        }
      },
    },
    VitePWA({
      registerType: 'autoUpdate',
      includeAssets: ['favicon.ico', 'apple-touch-icon.png', 'masked-icon.svg'],
      manifest: {
        name: 'Simple-Trader AI Terminal',
        short_name: 'SimpleTrader',
        description: 'Autonomous Pure-Go Global Trading & Quantitative Intelligence PWA',
        theme_color: '#070b14',
        background_color: '#020617',
        display: 'standalone',
        orientation: 'portrait',
        icons: [
          {
            src: '/pwa-192x192.png',
            sizes: '192x192',
            type: 'image/png',
          },
          {
            src: '/pwa-512x512.png',
            sizes: '512x512',
            type: 'image/png',
          },
        ],
      },
      workbox: {
        globPatterns: ['**/*.{js,css,ico,png,svg}'],
        // index.html must NEVER be precached: a stale precached HTML references
        // asset hashes deleted by the last deploy, and installed clients
        // blank-screen on the 404 module before the SW wakes up. Network-first
        // with a cache fallback keeps installs working offline and fresh on load.
        navigateFallbackDenylist: [],
        runtimeCaching: [
          {
            urlPattern: ({ request }) => request.mode === 'navigate',
            handler: 'NetworkFirst',
            options: {
              cacheName: 'html-shell',
              networkTimeoutSeconds: 3,
              expiration: { maxEntries: 5, maxAgeSeconds: 60 * 60 * 24 },
            },
          },
        ],
      },
    }),
  ],
  server: {
    port: 3000,
    proxy: {
      '/api': 'http://localhost:8080',
      '/health': 'http://localhost:8080',
    },
  },
});
