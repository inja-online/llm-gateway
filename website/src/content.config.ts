import { defineCollection } from "astro:content";
import { docsCollection } from "nimbus-docs/content";

export const collections = {
  docs: defineCollection(docsCollection()),
};
