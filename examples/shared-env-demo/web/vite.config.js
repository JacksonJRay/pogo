import { sveltekit } from "@sveltejs/kit/vite";
import { defineConfig } from "vite";

const port = Number(process.env.WEB_PORT) || 43122;

export default defineConfig({
  plugins: [sveltekit()],
  server: {
    host: "127.0.0.1",
    port,
    strictPort: true,
  },
});
