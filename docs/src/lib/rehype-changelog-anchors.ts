import { visit, SKIP } from "unist-util-visit";
import { toText } from "hast-util-to-text";
import GithubSlugger from "github-slugger";
import type { Root, Element, ElementContent } from "hast";
import type { VFile } from "vfile";

const HEADING_TAGS = new Set(["h1", "h2", "h3", "h4", "h5", "h6"]);

const LINK_ICON_SVG: Element = {
  type: "element",
  tagName: "svg",
  properties: { width: 16, height: 16, viewBox: "0 0 24 24" },
  children: [
    {
      type: "element",
      tagName: "path",
      properties: {
        fill: "currentcolor",
        d: "m12.11 15.39-3.88 3.88a2.52 2.52 0 0 1-3.5 0 2.47 2.47 0 0 1 0-3.5l3.88-3.88a1 1 0 0 0-1.42-1.42l-3.88 3.89a4.48 4.48 0 0 0 6.33 6.33l3.89-3.88a1 1 0 1 0-1.42-1.42Zm8.58-12.08a4.49 4.49 0 0 0-6.33 0l-3.89 3.88a1 1 0 0 0 1.42 1.42l3.88-3.88a2.52 2.52 0 0 1 3.5 0 2.47 2.47 0 0 1 0 3.5l-3.88 3.88a1 1 0 1 0 1.42 1.42l3.88-3.89a4.49 4.49 0 0 0 0-6.33ZM8.83 15.17a1 1 0 0 0 1.1.22 1 1 0 0 0 .32-.22l4.92-4.92a1 1 0 0 0-1.42-1.42l-4.92 4.92a1 1 0 0 0 0 1.42Z",
      },
      children: [],
    },
  ],
};

function buildAnchor(id: string, headingText: string): Element {
  return {
    type: "element",
    tagName: "a",
    properties: {
      href: `#${id}`,
      className: ["sl-anchor-link"],
    },
    children: [
      {
        type: "element",
        tagName: "span",
        properties: { "aria-hidden": "true", className: ["sl-anchor-icon"] },
        children: [structuredClone(LINK_ICON_SVG)],
      },
      {
        type: "element",
        tagName: "span",
        properties: { className: ["sr-only"], "data-pagefind-ignore": "" },
        children: [{ type: "text", value: `Section titled ${headingText}` }],
      },
    ],
  };
}

export function rehypeChangelogAnchors() {
  return (tree: Root, file: VFile) => {
    const path = typeof file.path === "string" ? file.path : "";
    const history = Array.isArray(file.history) ? file.history.join("|") : "";
    const haystack = `${path}|${history}`;
    if (!/[/\\]data[/\\]changelog[/\\]/.test(haystack)) return;

    const slugger = new GithubSlugger();

    visit(tree, "element", (node: Element, index, parent) => {
      if (!HEADING_TAGS.has(node.tagName)) return;
      if (typeof index !== "number" || !parent) return;

      const wrapperClass = Array.isArray(parent.properties?.className)
        ? (parent.properties.className as string[])
        : [];
      if (wrapperClass.includes("sl-heading-wrapper")) return;

      const props = (node.properties ??= {});
      const headingText = toText(node);
      let id = typeof props.id === "string" ? props.id : "";
      if (!id) {
        id = slugger.slug(headingText);
        props.id = id;
      }

      const anchor = buildAnchor(id, headingText);
      const wrapper: Element = {
        type: "element",
        tagName: "div",
        properties: {
          className: ["sl-heading-wrapper", `level-${node.tagName}`],
        },
        children: [node, anchor] as ElementContent[],
      };

      parent.children[index] = wrapper;
      return [SKIP, index + 1];
    });
  };
}
