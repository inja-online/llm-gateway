/// <reference path="../.astro/types.d.ts" />
/// <reference types="astro/client" />

declare module "virtual:nimbus/config" {
  import type { NimbusConfig } from "nimbus-docs/types";
  export const config: NimbusConfig;
  export const indexedCollections: string[];
  export const versionAlternates: Record<string, unknown>;
}
