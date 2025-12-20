import starlightLlmsTxt from "starlight-llms-txt";

type SidebarItem = {
  label: string;
  slug?: string;
  autogenerate?: {
    directory: string;
    collapsed?: boolean;
  };
  collapsed?: boolean;
  items?: SidebarItem[];
};

type SidebarSection = {
  label: string;
  autogenerate?: {
    directory: string;
    collapsed?: boolean;
  };
  collapsed?: boolean;
  items?: SidebarItem[];
};

type Sidebar = SidebarSection[];

const stripNumberPrefixes = (path: string): string =>
  path
    .split("/")
    .map(segment => segment.replace(/^\d+-/, ""))
    .join("/");

const collectPaths = (items: SidebarItem[] | undefined, acc: string[]): void => {
  for (const item of items ?? []) {
    if (item.slug) {
      acc.push(item.slug);
    }

    if (item.autogenerate?.directory) {
      acc.push(`docs/${stripNumberPrefixes(item.autogenerate.directory)}/**`);
    }

    if (item.items) {
      collectPaths(item.items, acc);
    }
  }
};

export function sidebarToCustomSets(sidebar: Sidebar): Array<{ label: string; paths: string[]; description?: string }> {
  return sidebar.map(section => {
    const paths: string[] = [];

    if (section.autogenerate?.directory) {
      paths.push(`docs/${stripNumberPrefixes(section.autogenerate.directory)}/**`);
    }

    collectPaths(section.items, paths);

    return {
      label: section.label,
      description: `${section.label} documentation`,
      paths: Array.from(new Set(paths)), // dedupe
    };
  });
}

export default function (sidebar: Sidebar) {
export default function (sidebar: Sidebar) {
  return starlightLlmsTxt({

  return starlightLlmsTxt({
    customSets: sidebarToCustomSets(sidebar),
    rawContent: true,
  });
}