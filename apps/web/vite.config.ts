import { fileURLToPath } from "node:url";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";
import solid from "vite-plugin-solid";
// The extension is required: Vite's native config loader cannot resolve a relative import
// without one, and it warns about it on every start.
import { devTokenPlugin } from "./vite/dev-token.ts";

export default defineConfig({
  plugins: [solid(), tailwindcss(), devTokenPlugin()],
  resolve: { alias: { "~": fileURLToPath(new URL("./src", import.meta.url)) } },
  server: {
    port: 3210,
    strictPort: true,
    // API and event calls go to the dev daemon, so the browser talks to one address.
    proxy: {
      "/v1": { target: `http://127.0.0.1:${process.env.MARSHAL_PORT ?? "47801"}`, ws: true },
    },
  },
  build: { target: "es2022" },
});
