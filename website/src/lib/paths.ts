/** Prefix paths with Astro `base` (e.g. `/llm-gateway`). */
export function withBase(path = "/"): string {
  const base = (import.meta.env.BASE_URL || "/").replace(/\/$/, "");
  if (!path || path === "/") return `${base}/`;
  const p = path.startsWith("/") ? path : `/${path}`;
  return `${base}${p}`;
}
