import { defineCollection, z } from "astro:content";
import { glob } from "astro/loaders";

const docs = defineCollection({
  loader: glob({ pattern: "**/*.{md,mdx}", base: "./src/content/docs" }),
  schema: z.object({
    title: z.string(),
    description: z.string().optional(),
    group: z.enum(["start", "reference", "ops", "project"]).default("reference"),
    order: z.number().default(99),
  }),
});

export const collections = { docs };
