import { fileURLToPath } from "node:url";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";
import solid from "vite-plugin-solid";

export default defineConfig({
  plugins: [solid(), tailwindcss()],
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
