# Docs site (Astro)

Custom documentation site for [Inja LLM Gateway](https://github.com/inja-online/llm-gateway).

**Live:** [inja-online.github.io/llm-gateway](https://inja-online.github.io/llm-gateway/)

## Develop

```bash
cd website
npm install
npm run dev
```

Open [http://localhost:4321/llm-gateway/](http://localhost:4321/llm-gateway/) (the site is built with `base: /llm-gateway` for GitHub Pages).

## Build

```bash
npm run build   # → dist/
npm run preview
```

## Content

Markdown pages live in `src/content/docs/` with frontmatter:

```yaml
---
title: Page title
description: Optional blurb
group: start | reference | ops | project
order: 10
---
```

Homepage and layout live under `src/pages/` and `src/layouts/`.

## Deploy

Push to `master` (paths under `website/**` or `CHANGELOG.md`) runs [`.github/workflows/docs.yml`](../.github/workflows/docs.yml), which builds this site and deploys `dist/` to GitHub Pages.
