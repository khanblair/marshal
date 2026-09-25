import { fileURLToPath } from "node:url";
import solid from "vite-plugin-solid";
import { defineConfig } from "vitest/config";

export default defineConfig({
  // Hot reload is for the dev server. Its `file:///@solid-refresh` import cannot be read on Windows.
  plugins: [solid({ hot: false })],
  resolve: {
    conditions: ["development", "browser"],
    alias: { "~": fileURLToPath(new URL("./src", import.meta.url)) },
  },
  test: {
    // A slow or busy machine, such as a CI runner, should not fail a test that is only slow.
    testTimeout: 15_000,
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test-setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
    server: {
      deps: { inline: [/solid-js/, /@solidjs\/testing-library/, /lucide-solid/, /@marshal\/ui/] },
    },
    coverage: {
      provider: "v8",
      include: ["src/**/*.{ts,tsx}"],
      exclude: ["src/**/*.test.{ts,tsx}", "src/test-setup.ts", "src/main.tsx"],
    },
  },
});
