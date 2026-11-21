const { readFileSync } = require('node:fs')

const [profile, ...packages] = process.argv.slice(2)
if (!profile || packages.length === 0) {
  throw new Error('Usage: node check-go-coverage.cjs <profile> <package> ...')
}

const totals = new Map(packages.map(name => [name, { statements: 0, covered: 0 }]))
const blocks = new Map()
for (const line of readFileSync(profile, 'utf8').split(/\r?\n/).slice(1)) {
  if (!line) continue
  const [location, statements, executions] = line.split(/\s+/)
  const name = location.match(/\/internal\/([^/]+)\/[^/]+$/)?.[1]
  if (!totals.has(name)) continue
  const block = blocks.get(location) ?? { name, statements: Number(statements), covered: false }
  block.covered ||= Number(executions) > 0
  blocks.set(location, block)
}
for (const { name, statements, covered } of blocks.values()) {
  const total = totals.get(name)
  total.statements += statements
  if (covered) total.covered += statements
}

for (const [name, { covered, statements }] of totals) {
  const percent = statements > 0 ? (covered / statements) * 100 : 0
  console.log(`${name}: ${percent.toFixed(1)}% (${covered}/${statements})`)
  if (percent < 70) process.exitCode = 1
}
