import path from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'
import {defineConfig} from 'vite'

// Vite 5+ SPA fallback returns index.html for unknown routes, which prevents
// Wails ExternalAssetHandler from falling through to AssetServer.Handler.
// Force /local-file to 404 so Go LocalFileHandler can serve real images.
function localFileAssetFallback() {
  return {
    name: 'wails-local-file-fallback',
    configureServer(server: {middlewares: {use: Function}}) {
      server.middlewares.use((req: {url?: string}, res: {statusCode: number; setHeader: Function; end: Function}, next: Function) => {
        if (req.url?.startsWith('/local-file')) {
          res.statusCode = 404
          res.setHeader('Content-Type', 'text/plain')
          res.end('not found')
          return
        }
        next()
      })
    },
  }
}

export default defineConfig({
  plugins: [vue(), tailwindcss(), localFileAssetFallback()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
})
