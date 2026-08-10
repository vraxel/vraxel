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
      // The react-hooks v7 compiler-family rules fire ~400 times on the
      // fetchData-in-useEffect idiom shared by every list/detail page.
      // That idiom is removed wholesale by the data-layer migration
      // (docs/frontend-refactor/plan.md, W2/W3); until it lands, keep
      // these visible as warnings so new code still sees them. Ratchet
      // back to error once the migration completes.
      'react-hooks/set-state-in-effect': 'warn',
      'react-hooks/refs': 'warn',
      'react-hooks/purity': 'warn',
      'react-hooks/immutability': 'warn',
      'react-hooks/preserve-manual-memoization': 'warn',
      // Pages currently export dialogs/helpers next to the page component
      // (e.g. HostFormDialog inside hosts/list.tsx); W3/W5 moves them into
      // module components/, then this returns to error.
      'react-refresh/only-export-components': 'warn',
      // Unused imports are auto-removed by unused-imports (AST-safe
      // autofix, used heavily by the W3 migration to strip imports the
      // framework replaced). no-unused-imports subsumes the import half
      // of no-unused-vars, so the ts rule keeps only the non-import half.
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
