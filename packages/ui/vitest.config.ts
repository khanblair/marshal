import solid from "vite-plugin-solid";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [solid()],
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
    },
  },
});
