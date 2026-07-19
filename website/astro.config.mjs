import { defineConfig } from "astro/config";
import mdx from "@astrojs/mdx";
import sitemap from "@astrojs/sitemap";

// GitHub Pages project site: https://inja-online.github.io/llm-gateway/
export default defineConfig({
  site: "https://inja-online.github.io",
  base: "/llm-gateway",
  integrations: [mdx(), sitemap()],
  markdown: {
    shikiConfig: {
      theme: "github-dark-dimmed",
      wrap: true,
    },
  },
  build: {
    format: "directory",
  },
});
