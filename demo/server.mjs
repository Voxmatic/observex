import { createReadStream, existsSync, statSync } from 'node:fs'
import { createServer } from 'node:http'
import { extname, join, normalize } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = fileURLToPath(new URL('.', import.meta.url))
const port = Number(process.env.PORT || 4177)

const types = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'application/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.svg': 'image/svg+xml',
}

function resolvePath(url) {
  const clean = decodeURIComponent(url.split('?')[0])
  const requested = clean === '/' ? '/observex-agent-saas-demo.html' : clean
  const candidate = normalize(join(root, requested))
  if (!candidate.startsWith(root)) return null
  return candidate
}

createServer((req, res) => {
  const path = resolvePath(req.url || '/')
  if (!path || !existsSync(path) || !statSync(path).isFile()) {
    res.writeHead(404, { 'content-type': 'text/plain; charset=utf-8' })
    res.end('Not found')
    return
  }
  res.writeHead(200, {
    'content-type': types[extname(path)] || 'application/octet-stream',
    'cache-control': 'no-store',
  })
  createReadStream(path).pipe(res)
}).listen(port, () => {
  console.log(`ObserveX Agent SaaS demo running at http://localhost:${port}`)
})
