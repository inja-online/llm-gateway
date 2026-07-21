/**
 * Prefix a site-absolute path with Astro's `base` (e.g. `/llm-gateway`).
 * Leaves external URLs, anchors, and already-prefixed paths alone.
 */
export function withBase(href: string): string {
  if (!href) return href;
  if (
    href.startsWith("http://") ||
    href.startsWith("https://") ||
    href.startsWith("//") ||
    href.startsWith("#") ||
    href.startsWith("?") ||
    href.startsWith("mailto:") ||
    href.startsWith("data:")
  ) {
    return href;
  }

  const base = (import.meta.env.BASE_URL || "/").replace(/\/$/, "") || "";
  if (!base) return href;

  // Relative path — leave for the browser
  if (!href.startsWith("/")) return href;

  // Already under base
  if (href === base || href.startsWith(`${base}/`)) return href;

  return `${base}${href}`;
}

/**
 * Origin + base path for absolute site URLs.
 * Nimbus bridges `nimbusConfig.site` (which includes base) into `Astro.site`,
 * so do not append `BASE_URL` again.
 */
export function siteRoot(site: URL | undefined): string {
  return (site?.href ?? "").replace(/\/$/, "");
}
