// Production web server for Choreo.
//
// Serves the built Vite app from ./dist and proxies /api to the
// backend. The Authorization: Bearer header is forwarded unchanged so the
// backend can validate the Asgardeo token itself.
import http from 'http';
import https from 'https';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const PORT = process.env.PORT || 8080;
const DIST = path.join(__dirname, 'dist');

// Public backend base URL. On Choreo this is the backend component's public
// gateway URL; locally it defaults to the dev backend.
const BACKEND_URL = (process.env.BACKEND_URL || 'http://localhost:8000').replace(/\/$/, '');

const MIME_TYPES = {
  '.html': 'text/html',
  '.js': 'application/javascript',
  '.css': 'text/css',
  '.json': 'application/json',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.svg': 'image/svg+xml',
  '.ico': 'image/x-icon',
  '.woff': 'font/woff',
  '.woff2': 'font/woff2',
};

function proxyToBackend(req, res) {
  const target = new URL(BACKEND_URL + req.url);
  const isHttps = target.protocol === 'https:';
  const lib = isHttps ? https : http;

  // Forward all headers except host/transfer-encoding, keeping Authorization
  // intact so the backend can validate the Asgardeo JWT.
  const forwardHeaders = {};
  for (const [key, value] of Object.entries(req.headers)) {
    const lower = key.toLowerCase();
    if (lower !== 'transfer-encoding' && lower !== 'host') {
      forwardHeaders[key] = value;
    }
  }

  const options = {
    hostname: target.hostname,
    port: target.port || (isHttps ? 443 : 80),
    path: target.pathname + target.search,
    method: req.method,
    headers: forwardHeaders,
  };

  const proxyReq = lib.request(options, (proxyRes) => {
    res.writeHead(proxyRes.statusCode, proxyRes.headers);
    proxyRes.pipe(res);
  });
  proxyReq.on('error', (err) => {
    console.error('Proxy error:', err.code, err.message, '→', target.href);
    if (!res.headersSent) {
      res.writeHead(502);
      res.end('Bad Gateway');
    }
  });
  req.pipe(proxyReq);
}

function serveStatic(req, res) {
  const url = req.url.split('?')[0];
  let filePath = path.join(DIST, url === '/' ? 'index.html' : url);

  // Fall back to index.html so client-side routing works.
  if (!fs.existsSync(filePath) || fs.statSync(filePath).isDirectory()) {
    filePath = path.join(DIST, 'index.html');
  }

  const contentType = MIME_TYPES[path.extname(filePath).toLowerCase()] || 'application/octet-stream';
  fs.readFile(filePath, (err, data) => {
    if (err) {
      res.writeHead(404);
      res.end('Not found');
      return;
    }
    res.writeHead(200, { 'Content-Type': contentType });
    res.end(data);
  });
}

http
  .createServer((req, res) => {
    const url = req.url.split('?')[0];
    if (url.startsWith('/api')) {
      proxyToBackend(req, res);
    } else {
      serveStatic(req, res);
    }
  })
  .listen(PORT, () => console.log(`Frontend on :${PORT} → ${BACKEND_URL}`));
