#!/usr/bin/env node
// Project-local JSX lint rules. ESLint plugin-grade rules need a full
// AST and a published plugin; these are coarse regex-based checks that
// catch the specific repo-wide anti-patterns we keep reintroducing.
// Run via `pnpm lint:jsx`; it's also chained into `pnpm lint`.
//
// Rules:
//   - dialog-overflow: `<DialogContent ... overflow-y-auto>` whitescreen
//     pattern (see ui/CLAUDE.md "大表单 Dialog 标准容器结构").
//   - input-needs-name: `<Input>` JSX without `name=`, `id=`, or any
//     `{...spread}`. The runtime guard in src/components/ui/input.tsx
//     would log a console.error otherwise; this rule keeps the codebase
//     from accumulating new ones.
//   - textarea-needs-name: same rule for `<Textarea>`. The runtime guard
//     in src/components/ui/textarea.tsx *throws* (not just logs), so a
//     bare <Textarea> white-screens the page on mount. Build-time
//     enforcement is the only way to keep this off main.
//
// Adding a rule:
//   - register one entry on RULES with a name + a checker function
//   - checker receives (path, src) and returns array of `path:line` hits
//   - keep helpers shared (walk / stripComments / findJsxProps) so
//     rules don't re-scan the tree
import { readdirSync, readFileSync, statSync } from "node:fs"
import { join } from "node:path"

// ---------- shared helpers ----------

const walk = (dir) =>
  readdirSync(dir).flatMap((f) => {
    const p = join(dir, f)
    return statSync(p).isDirectory() ? walk(p) : [p]
  })

function stripCommentsAndStrings(src) {
  // Whitespace-out comments + string / template literals so docstrings
  // (`<Input>` in a comment) and error messages (`\`<Input> mounted
  // with...\`` inside src/components/ui/input.tsx) don't false-
  // positive. Same-length substitution keeps line numbers aligned.
  //
  // JSX attribute values like className="foo" also get blanked, but
  // they wouldn't carry `<Input>` literal text anyway -- JSX opening
  // tags are not inside strings.
  return src
    .replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(/[^\n]/g, " "))
    .replace(/\/\/[^\n]*/g, (m) => m.replace(/[^\n]/g, " "))
    .replace(/`(?:\\.|\$\{[\s\S]*?\}|[^`\\])*`/g, (m) => m.replace(/[^\n]/g, " "))
    .replace(/"(?:\\.|[^"\\\n])*"/g, (m) => m.replace(/[^\n]/g, " "))
    .replace(/'(?:\\.|[^'\\\n])*'/g, (m) => m.replace(/[^\n]/g, " "))
}

function findJsxProps(src, startIdx) {
  // Walk forward balancing `{ }` so an inner JSX expression like
  // `onChange={(e) => ...}` doesn't terminate parse early. Stops at
  // the first top-level `>` (open or self-close).
  let depth = 0
  for (let i = startIdx; i < src.length; i++) {
    const c = src[i]
    if (c === "{") depth++
    else if (c === "}") depth--
    else if (depth === 0 && c === ">") return src.slice(startIdx, i + 1)
  }
  return ""
}

function lineOf(src, idx) {
  return src.slice(0, idx).split("\n").length
}

// ---------- rules ----------

function checkDialogOverflow(path, src) {
  const out = []
  for (const m of src.matchAll(/<DialogContent[\s\S]*?>/g)) {
    if (/overflow-(y-)?auto/.test(m[0])) {
      out.push(`${path}:${lineOf(src, m.index)}`)
    }
  }
  return out
}

// stripAttributeBraces blanks out every `attr={ ... }` value in propsBlock
// (depth-balanced) so the remaining `{...}` literals are top-level JSX
// children. A naive `/\{\s*\.\.\.[^}]+\}/` regex misfires on callbacks
// like `onChange={(e) => setForm({ ...form, foo: e.target.value })}`,
// where the inner `{ ...form }` is JS object spread, not a JSX prop
// spread. Strip the outer balanced `attr={...}` first and only inspect
// what remains.
//
// The `=` and `{` MUST be adjacent (no intervening whitespace). JSX
// attribute syntax never inserts whitespace between `=` and `{`. The
// stripCommentsAndStrings pass blanks out string attribute values
// (e.g. `name="foo"` → `name=     `), so allowing `\s*` between `=`
// and `{` would treat a following `{...spread}` attribute as the value
// of the previous attribute and over-strip past the spread, hiding
// real JSX spreads from detection.
function stripAttributeBraces(s) {
  let out = s
  while (true) {
    const m = out.match(/[A-Za-z_][\w-]*=\{/)
    if (!m) return out
    const open = m.index + m[0].length - 1
    let depth = 1
    let i = open + 1
    while (i < out.length && depth > 0) {
      if (out[i] === "{") depth++
      else if (out[i] === "}") depth--
      i++
    }
    out = out.slice(0, m.index) + " ".repeat(i - m.index) + out.slice(i)
  }
}

function makeNeedsNameChecker(tagPattern) {
  return (path, src) => {
    const out = []
    for (const m of src.matchAll(tagPattern)) {
      const propsBlock = findJsxProps(src, m.index)
      if (!propsBlock) continue
      if (/\bname\s*=/.test(propsBlock)) continue
      if (/\bid\s*=/.test(propsBlock)) continue
      // Real JSX spread is a top-level `{...expr}` attribute, never an
      // inner JS-object spread inside a callback. Strip attribute values
      // before checking so callback bodies don't false-tolerate.
      if (/\{\s*\.\.\./.test(stripAttributeBraces(propsBlock))) continue
      out.push(`${path}:${lineOf(src, m.index)}`)
    }
    return out
  }
}

const checkInputNeedsName = makeNeedsNameChecker(/<Input\b/g)
const checkTextareaNeedsName = makeNeedsNameChecker(/<Textarea\b/g)

const RULES = [
  {
    name: "dialog-overflow",
    fix: "<DialogContent> must not use overflow-y-auto / overflow-auto",
    check: checkDialogOverflow,
    useStrippedSrc: true,
  },
  {
    name: "input-needs-name",
    fix: "<Input> needs name=, id=, or a {...spread}",
    check: checkInputNeedsName,
    useStrippedSrc: true,
  },
  {
    name: "textarea-needs-name",
    fix: "<Textarea> needs name=, id=, or a {...spread} (runtime guard throws otherwise)",
    check: checkTextareaNeedsName,
    useStrippedSrc: true,
  },
]

// ---------- main ----------

// Pick subset of RULES per --rule=name CLI flag. `pnpm lint` only runs
// the rules that have a clean baseline so it doesn't block unrelated
// work; `pnpm lint:jsx:all` (no flag) runs every rule and is the way
// to surface the still-outstanding violations for chore-time cleanup.
const ruleFilter = process.argv.find((a) => a.startsWith("--rule="))?.slice("--rule=".length)
const activeRules = ruleFilter ? RULES.filter((r) => r.name === ruleFilter) : RULES
if (ruleFilter && activeRules.length === 0) {
  console.error(`lint:jsx: unknown rule ${ruleFilter}; available: ${RULES.map((r) => r.name).join(", ")}`)
  process.exit(2)
}

const files = walk("src").filter((p) => p.endsWith(".tsx"))
const rawByPath = new Map()
const stripByPath = new Map()
for (const p of files) {
  const raw = readFileSync(p, "utf8")
  rawByPath.set(p, raw)
  stripByPath.set(p, stripCommentsAndStrings(raw))
}

let failed = false
for (const rule of activeRules) {
  const hits = []
  const srcMap = rule.useStrippedSrc ? stripByPath : rawByPath
  for (const p of files) hits.push(...rule.check(p, srcMap.get(p)))
  if (hits.length === 0) continue
  failed = true
  console.error(`lint:${rule.name} FAIL — ${rule.fix}:`)
  for (const h of hits) console.error(`  ${h}`)
}
if (failed) process.exit(1)
