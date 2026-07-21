import { defineConfig } from "astro/config";
import nimbus, { defineConfig as defineNimbusConfig } from "nimbus-docs";
import tailwindcss from "@tailwindcss/vite";
import icon from "astro-icon";

// GitHub Pages project site: https://inja-online.github.io/llm-gateway/
const site = "https://inja-online.github.io";
const base = "/llm-gateway";

const nimbusConfig = defineNimbusConfig({
  site: `${site}${base}`,
  title: "Inja LLM Gateway",
  description:
    "Open-source multi-dialect LLM API gateway. OpenAI, Anthropic, and Gemini clients — route to any provider with passthrough fidelity, translation, and usage metering.",
  locale: "en",
  homeLabel: "Home",
  github: "https://github.com/inja-online/llm-gateway",
  editPattern:
    "https://github.com/inja-online/llm-gateway/edit/master/website/{path}",
  socialImage: "/logo.jpg",
  socialImageAlt: "Inja LLM Gateway",
  features: {
    sidebar: true,
    tableOfContents: true,
  },
});

export default defineConfig({
  site,
  base,
  integrations: [
    nimbus(nimbusConfig, {
      validateMdx: true,
      admonitions: true,
    }),
    icon(),
  ],
  vite: {
    plugins: [tailwindcss()],
  },
  build: {
    format: "directory",
  },
});
