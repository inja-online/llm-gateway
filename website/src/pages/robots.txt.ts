import type { APIRoute } from "astro";

export const prerender = true;

export const GET: APIRoute = () => {
  const site = "https://inja-online.github.io/llm-gateway";
  const body = [
    "User-agent: *",
    "Allow: /",
    "",
    `Sitemap: ${site}/sitemap-index.xml`,
    "",
  ].join("\n");
  return new Response(body, {
    headers: { "Content-Type": "text/plain; charset=utf-8" },
  });
};
