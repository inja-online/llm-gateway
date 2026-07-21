/**
 * MDX global component registry — available in every MDX file without imports.
 * Install more with: npx nimbus-docs add <slug>
 */
import { Aside } from "./components/ui/aside";
import { Badge } from "./components/ui/badge";
import { Callout } from "./components/ui/callout";
import { Card } from "./components/ui/card";
import { CardGrid } from "./components/ui/card-grid";
import { Steps, Step } from "./components/ui/steps";
import { Tabs, TabItem } from "./components/ui/tabs";
import { FileTree } from "./components/ui/file-tree";
import { LinkCard } from "./components/ui/link-card";
import { LinkButton } from "./components/ui/link-button";
import { PackageManagers } from "./components/ui/package-managers";

export const components = {
  Aside,
  Badge,
  Callout,
  Card,
  CardGrid,
  Steps,
  Step,
  Tabs,
  TabItem,
  FileTree,
  LinkCard,
  LinkButton,
  PackageManagers,
};
