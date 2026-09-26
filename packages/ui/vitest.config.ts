import solid from "vite-plugin-solid";
import { defineConfig } from "vitest/config";

export default defineConfig({
  // Hot reload is for the dev server. Its `file:///@solid-refresh` import cannot be read on Windows.
  plugins: [solid({ hot: false })],
  resolve: { conditions: ["development", "browser"] },
  test: {
    // A slow or busy machine, such as a CI runner, should not fail a test that is only slow.
    testTimeout: 15_000,
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test-setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
    server: { deps: { inline: [/solid-js/, /@solidjs\/testing-library/, /lucide-solid/] } },
    coverage: {
      provider: "v8",
      include: ["src/**/*.{ts,tsx}"],
      exclude: ["src/**/*.test.{ts,tsx}", "src/test-setup.ts"],
      reporter: ["text-summary"],
      // The floor for UI packages in docs/code-standards.md section 6. `vitest run --coverage` fails below it.
      thresholds: { lines: 60, statements: 60, functions: 60, branches: 60 },
    },
  },
});
