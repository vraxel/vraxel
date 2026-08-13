import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import unusedImports from 'eslint-plugin-unused-imports'
import tseslint from 'typescript-eslint'
import { defineConfig, globalIgnores } from 'eslint/config'

export default defineConfig([
  globalIgnores(['dist', 'src/shared/ui', 'src/generated']),
  {
    files: ['**/*.{ts,tsx}'],
    plugins: { 'unused-imports': unusedImports },
    extends: [
      js.configs.recommended,
      tseslint.configs.recommended,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
    },
    rules: {
      // --- shared decisions -------------------------------------------------
      // MIRROR of `sharedRules` in lcp's eslint.shared.js. That file is the
      // source of truth; there is no private npm registry to share a package
      // through, so this copy is deliberate and must be kept identical.
      //
      // The standing rule: a rule is either at 'error' or it is not enabled.
      // Nothing lives at 'warn'. `eslint . --max-warnings 0` makes the two
      // identical to CI anyway, so a downgrade buys nothing except invisible
      // debt -- which is exactly how ~340 violations accumulated in the repo
      // this UI was seeded from, behind a green CI. Exceptions go on the
      // offending line as `// eslint-disable-next-line <rule>` with the reason
      // above it: greppable, visible in the diff, and dies with the code it
      // excuses.
      //
      // exhaustive-deps ships as 'warn' in the recommended config. The class it
      // catches (a `?? []` view rebuilt every render, defeating the memo or
      // tracker keyed on it) is exactly what breaks the render-phase transition
      // pattern used throughout this tree.
      'react-hooks/exhaustive-deps': 'error',
      // --- local ------------------------------------------------------------
      // The react-hooks v7 compiler-family rules and
      // react-refresh/only-export-components were briefly pinned to 'warn'
      // here, carried over from the seed repo where the fetchData-in-useEffect
      // idiom fired them a few hundred times. That never described this tree --
      // every page reads through useApiQuery and eslint reports zero hits -- so
      // they sit at the recommended 'error'.
      // Unused imports are auto-removed by unused-imports (AST-safe
      // autofix). no-unused-imports subsumes the import half of
      // no-unused-vars, so the ts rule keeps only the non-import half.
      'unused-imports/no-unused-imports': 'error',
      // `const { kind: _kind, ...rest } = obj` omit idiom: rest siblings
      // and _-prefixed bindings are intentional discards.
      '@typescript-eslint/no-unused-vars': ['error', {
        argsIgnorePattern: '^_',
        varsIgnorePattern: '^_',
        caughtErrorsIgnorePattern: '^_',
        ignoreRestSiblings: true,
      }],
    },
  },
  {
    // Playwright suite: not React code. The fixtures' `use` callback
    // parameter false-positives react-hooks/rules-of-hooks.
    files: ['e2e/**'],
    rules: {
      'react-hooks/rules-of-hooks': 'off',
    },
  },
])
