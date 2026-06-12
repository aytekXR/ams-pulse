import js from "@eslint/js";
import tsPlugin from "@typescript-eslint/eslint-plugin";
import tsParser from "@typescript-eslint/parser";
import reactHooks from "eslint-plugin-react-hooks";

/** @type {import('eslint').Linter.Config[]} */
export default [
  {
    ignores: ["dist/**", "node_modules/**", "src/lib/api/schema.d.ts"],
  },
  js.configs.recommended,
  {
    files: ["src/**/*.{ts,tsx}"],
    languageOptions: {
      parser: tsParser,
      parserOptions: {
        ecmaVersion: 2022,
        sourceType: "module",
        ecmaFeatures: { jsx: true },
      },
      globals: {
        window: "readonly",
        document: "readonly",
        console: "readonly",
        setTimeout: "readonly",
        clearTimeout: "readonly",
        setInterval: "readonly",
        clearInterval: "readonly",
        localStorage: "readonly",
        fetch: "readonly",
        WebSocket: "readonly",
        RequestInit: "readonly",
        URL: "readonly",
        URLSearchParams: "readonly",
        Blob: "readonly",
        HTMLElement: "readonly",
        HTMLDivElement: "readonly",
        HTMLInputElement: "readonly",
        HTMLSelectElement: "readonly",
        HTMLTextAreaElement: "readonly",
        HTMLFormElement: "readonly",
        HTMLButtonElement: "readonly",
        Event: "readonly",
        CustomEvent: "readonly",
        CloseEvent: "readonly",
        MessageEvent: "readonly",
        EventTarget: "readonly",
        AbortController: "readonly",
        FormData: "readonly",
        Response: "readonly",
        Headers: "readonly",
        Request: "readonly",
        performance: "readonly",
        React: "readonly",
        confirm: "readonly",
        prompt: "readonly",
        global: "readonly",
      },
    },
    plugins: {
      "@typescript-eslint": tsPlugin,
      "react-hooks": reactHooks,
    },
    rules: {
      ...tsPlugin.configs.recommended.rules,
      ...reactHooks.configs.recommended.rules,
      "@typescript-eslint/no-unused-vars": [
        "error",
        { argsIgnorePattern: "^_", varsIgnorePattern: "^_" },
      ],
      "@typescript-eslint/no-explicit-any": "warn",
      "no-console": ["warn", { allow: ["warn", "error"] }],
      // useEffect with async data-fetch pattern: void-calling an async fn in
      // an effect is intentional (avoids the useEffect-returns-promise lint
      // error). The "set-state-in-effect" rule fires a false-positive here;
      // the fetch callbacks call setState in async continuations, not
      // synchronously in the effect body.
      "react-hooks/set-state-in-effect": "off",
    },
  },
];
