# Docs site (Nimbus + Astro)

Documentation for [Inja LLM Gateway](https://github.com/inja-online/llm-gateway), built with [Nimbus](https://nimbus-docs.com) on Astro 7.

**Live:** [inja-online.github.io/llm-gateway](https://inja-online.github.io/llm-gateway/)

## Develop

```bash
cd website
npm install
npm run dev
```

Open [http://localhost:4321/llm-gateway/](http://localhost:4321/llm-gateway/) (`base: /llm-gateway` for GitHub Pages).

## Build

```bash
npm run build   # → dist/ (+ Pagefind index when search enabled)
npm run preview
```

## Content

Markdown/MDX under `src/content/docs/`. The filesystem **is** the sidebar and URL tree (Nimbus convention).

Frontmatter (Nimbus schema):

```yaml
---
title: Page title
description: Optional blurb
sidebar:
  order: 10
  label: Short nav label
  group:
    label: Start
---
```

Landing page: `src/pages/index.astro` (custom marketing home).

## Components

UI ships from the Nimbus registry into `src/components/ui/` (you own the files):

```bash
npx nimbus-docs list
npx nimbus-docs add card --yes
```

Register MDX globals in `src/components.ts`.

## Agent surfaces

- `/llms.txt` — page index for agents  
- Every docs page has `/…/index.md` Markdown alternate  
- `public/.nojekyll` for GitHub Pages  

## Deploy

Push to `master` (paths under `website/**` or `CHANGELOG.md`) runs [`.github/workflows/docs.yml`](../.github/workflows/docs.yml), builds this site, and deploys `dist/` to GitHub Pages.
