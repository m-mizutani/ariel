// Minimal flat config. It enforces the IME policy carried over from
// hecatoncheires: an Enter/Space handler must not trigger an action while an
// IME composition is in progress, so raw key detection is forbidden.
import tsParser from '@typescript-eslint/parser'

const eqOp = '[operator=/^={2,3}$/]'

const restrictedKeyboard = [
  {
    selector: `BinaryExpression${eqOp}:matches([right.value='Enter'], [left.value='Enter'])`,
    message: "Direct 'Enter' key detection breaks IME (CJK) input. Guard it against isComposing first.",
  },
  {
    selector: `BinaryExpression${eqOp}:matches([right.value=13], [left.value=13])`,
    message: 'Direct keyCode === 13 detection breaks IME (CJK) input.',
  },
]

export default [
  {
    ignores: ['dist/**', 'node_modules/**', 'coverage/**'],
  },
  {
    files: ['src/**/*.{ts,tsx}'],
    languageOptions: {
      parser: tsParser,
      parserOptions: {
        ecmaVersion: 'latest',
        sourceType: 'module',
        ecmaFeatures: { jsx: true },
      },
    },
    rules: {
      'no-restricted-syntax': ['error', ...restrictedKeyboard],
    },
  },
]
