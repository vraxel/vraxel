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
      // The react-hooks v7 compiler-family rules and
      // react-refresh/only-export-components used to be pinned to 'warn' here,
      // carried over from the repo this UI was seeded from, where the
      // fetchData-in-useEffect idiom fired them a few hundred times pending a
      // data-layer migration. That does not describe this tree: every page
      // reads through useApiQuery already, and eslint reports zero hits for
      // all six rules. Holding them at 'warn' bought nothing and quietly
      // licensed new code to reintroduce the idiom, so they stay at the
      // recommended 'error'.
      // Unused imports are auto-removed by unused-imports (AST-safe
      // autofix, used heavily by the W3 migration to strip imports the
      // framework replaced). no-unused-imports subsumes the import half
      // of no-unused-vars, so the ts rule keeps only the non-import half.
      // exhaustive-deps ships as 'warn' in the recommended config. This tree
      // reports zero hits, and the class it catches (a `?? []` view rebuilt
      // every render, defeating the memo or tracker keyed on it) is exactly
      // what breaks the render-phase transition pattern used throughout. Held
      // at error so it stays at zero.
      'react-hooks/exhaustive-deps': 'error',
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
