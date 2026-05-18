// Copies the three data files into public/data/ so Vite serves them
// alongside the SPA. Runs as a `predev` and `prebuild` step.
//
// Source path is configurable via DATA_DIR; defaults to ../../data
// (the parent repo's data directory, which is where the build scripts
// in scripts/ deposit them).

import { copyFileSync, mkdirSync, existsSync, statSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const projectRoot = resolve(here, '..')
const dstDir = join(projectRoot, 'public', 'data')
const files = [
  'election_results.parquet',
  'local_election_ward_2026.csv',
  'uk_wards.csv',
]

// Find the data dir: explicit DATA_DIR wins, otherwise walk up from here
// looking for a sibling `data/` that contains the canonical parquet.
function findDataDir() {
  if (process.env.DATA_DIR) return resolve(process.env.DATA_DIR)
  let dir = projectRoot
  for (let i = 0; i < 8; i++) {
    const candidate = join(dir, 'data')
    if (existsSync(join(candidate, 'election_results.parquet'))) return candidate
    const parent = dirname(dir)
    if (parent === dir) break
    dir = parent
  }
  return join(projectRoot, '..', 'data') // fallback to original guess
}
const srcDir = findDataDir()

mkdirSync(dstDir, { recursive: true })
let missing = 0
for (const f of files) {
  const from = join(srcDir, f)
  const to = join(dstDir, f)
  if (!existsSync(from)) {
    console.warn(`[copy-data] missing: ${from}`)
    missing++
    continue
  }
  copyFileSync(from, to)
  const kb = (statSync(to).size / 1024).toFixed(1)
  console.log(`[copy-data] ${f} (${kb} KB)`)
}
if (missing > 0) {
  console.warn(`[copy-data] ${missing} file(s) missing — set DATA_DIR or run the build scripts in scripts/ first`)
}
