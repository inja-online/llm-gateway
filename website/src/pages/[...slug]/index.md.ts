import type { APIRoute } from "astro";
import { getCollection } from "astro:content";
import { renderEntryAsMarkdown } from "nimbus-docs";

export const prerender = true;

export async function getStaticPaths() {
  const entries = await getCollection("docs", ({ data }) => {
    if (import.meta.env.PROD && data.draft) return false;
    return true;
  });
  return entries.map((entry) => ({
    params: { slug: entry.id === "index" ? undefined : entry.id },
    props: { entry },
  }));
}

export const GET: APIRoute = async ({ props }) => {
  const { entry } = props as { entry: { body?: string; data: { title: string; description?: string } } };
  const body = renderEntryAsMarkdown(entry);
  const header = [
    `# ${entry.data.title}`,
    entry.data.description ? `\n> ${entry.data.description}\n` : "",
    "",
  ].join("\n");
  return new Response(header + body, {
    headers: {
      "Content-Type": "text/markdown; charset=utf-8",
    },
  });
};
