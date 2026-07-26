import { defineConfig } from 'tsup';
import { readFileSync } from 'fs';

const { version } = JSON.parse(readFileSync('./package.json', 'utf8')) as { version: string };

export default defineConfig({
  entry: ['src/index.ts'],
  format: ['esm', 'cjs', 'iife'],
  dts: true,
  minify: true,
  define: {
    __SDK_VERSION__: JSON.stringify(version),
  },
});
