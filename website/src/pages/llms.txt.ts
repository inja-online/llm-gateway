import type { APIRoute } from "astro";
import { getCollection } from "astro:content";

export const prerender = true;

export const GET: APIRoute = async () => {
  const entries = await getCollection("docs", ({ data }) => {
    if (import.meta.env.PROD && data.draft) return false;
    return data.searchable !== false && !data.noindex;
  });

  const site = "https://inja-online.github.io/llm-gateway";
  const lines = [
    "# Inja LLM Gateway",
    "",
    "> Open-source multi-dialect LLM API gateway. OpenAI, Anthropic, and Gemini clients route to any provider with passthrough fidelity, translation, and usage metering.",
    "",
    `Full corpus: ${site}/llms-full.txt`,
    "",
    "## Pages",
    "",
  ];

  const sorted = [...entries].sort((a, b) => a.id.localeCompare(b.id));
  for (const e of sorted) {
    const path = e.id === "index" ? "" : `${e.id}/`;
    const md = `${site}/${path}index.md`;
    const desc = e.data.description ? ` — ${e.data.description}` : "";
    lines.push(`- [${e.data.title}](${md})${desc}`);
  }
  lines.push("");

  return new Response(lines.join("\n"), {
    headers: { "Content-Type": "text/plain; charset=utf-8" },
  });
};
