import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import fsp from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

// ── Хранилище фоновых картинок кастомной темы (dev-режим) ───────────────
// В проде /api/bg/* и /bg/<файл> обслуживает Go-хендлер internal/bgstore
// (Wails assetserver Handler, папка <exe>/bg). В dev (wails dev / vite)
// то же самое эмулирует эта мидлварь: файлы лежат в public/bg.
const BG_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), 'public', 'bg');
const BG_EXT = /\.(jpe?g|png|webp|gif)$/i;

const bgName = (raw) => {
  const base = path.basename(String(raw || ''));
  return BG_EXT.test(base) ? base : null;
};

const bgPath = (name) => '/bg/' + encodeURIComponent(name);

function bgStorage() {
  return {
    name: 'zpui-bg-storage',
    configureServer(server) {
      server.middlewares.use('/api/bg', (req, res) => {
        const done = (code, obj) => {
          res.statusCode = code;
          res.setHeader('Content-Type', 'application/json; charset=utf-8');
          res.end(JSON.stringify(obj));
        };
        const route = (req.url || '/').split('?')[0].replace(/\/+$/, '') || '/';
        const readBody = async () => {
          const chunks = [];
          for await (const ch of req) chunks.push(ch);
          return Buffer.concat(chunks);
        };
        (async () => {
          try {
            await fsp.mkdir(BG_DIR, { recursive: true });

            if (req.method === 'GET' && (route === '/' || route === '/list')) {
              const names = (await fsp.readdir(BG_DIR)).filter((n) => BG_EXT.test(n)).sort();
              return done(200, { files: names.map((n) => ({ name: n, path: bgPath(n) })) });
            }

            if (req.method === 'POST' && route === '/upload') {
              const buf = await readBody();
              const name = decodeURIComponent(String(req.headers['x-file-name'] || req.headers['x-filename'] || ''));
              const safe = bgName(name);
              if (!safe || !buf || !buf.length) return done(400, { error: 'bad_file' });
              await fsp.writeFile(path.join(BG_DIR, safe), buf);
              return done(200, { ok: true, name: safe, path: bgPath(safe) });
            }

            if (req.method === 'POST' && route === '/delete') {
              let body = {};
              try { body = JSON.parse((await readBody()).toString('utf8') || '{}'); } catch {}
              const safe = bgName(body.name);
              if (!safe) return done(400, { error: 'bad_name' });
              const target = path.resolve(BG_DIR, safe);
              if (path.dirname(target) !== path.resolve(BG_DIR)) return done(400, { error: 'bad_name' });
              await fsp.rm(target, { force: true });
              return done(200, { ok: true });
            }

            return done(404, { error: 'not_found' });
          } catch (e) {
            return done(500, { error: String((e && e.message) || e) });
          }
        })();
      });
    },
  };
}

export default defineConfig({
  plugins: [react(), bgStorage()],
  root: process.cwd(),
  resolve: {
    // Один экземпляр React на весь бандл: дубликат ломает хуки.
    dedupe: ['react', 'react-dom'],
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    host: '0.0.0.0',
    port: 3000,
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./vitest.setup.js'],
    css: false,
  },
});
