import type { StarlightRouteData } from '@astrojs/starlight/route-data';

type SidebarEntry = StarlightRouteData['sidebar'][number];

type SidebarGroup = Extract<SidebarEntry, { type: 'group' }>;

type SidebarLink = Extract<SidebarEntry, { type: 'link' }>;

export type Route = Pick<StarlightRouteData, 'sidebar' | 'pagination'> & {
	entry: { data: Pick<StarlightRouteData['entry']['data'], 'prev' | 'next'> };
};

/**
 * A page an `.astro` route renders from a data collection, which Starlight's
 * sidebar config can't list. `group` is the labels of the configured sidebar
 * group it goes in, outermost first. `subgroup` nests it in a collapsed group
 * of that name, created after the group's other entries.
 */
export interface DataPage {
	group: string[];
	subgroup?: string;
	label: string;
	href: string;
	experimental: boolean;
}

/**
 * Appends a link for each page, in order, to its group in the route's sidebar,
 * and sets the previous and next links to the current page's new neighbors,
 * unless the page sets them in its frontmatter. `currentPath` is the URL path
 * of the page being rendered. Throws when a group isn't in the sidebar.
 */
export function addDataPages(route: Route, pages: DataPage[], currentPath: string): void {
	for (const page of pages) {
		const group = findGroup(route.sidebar, page.group);
		const entries = page.subgroup ? findOrAddSubgroup(group, page.subgroup).entries : group.entries;
		entries.push({
			type: 'link',
			label: page.label,
			href: page.href,
			isCurrent: samePath(page.href, currentPath),
			badge: page.experimental ? { text: 'exp', variant: 'tip' } : undefined,
			attrs: {},
		});
	}

	const links = flatten(route.sidebar);
	const current = links.findIndex((link) => link.isCurrent);
	if (current === -1) return;
	if (route.entry.data.prev === undefined) route.pagination.prev = links[current - 1];
	if (route.entry.data.next === undefined) route.pagination.next = links[current + 1];
}

function findGroup(sidebar: SidebarEntry[], labels: string[]): SidebarGroup {
	const root: SidebarGroup = { type: 'group', label: '', entries: sidebar, collapsed: false, badge: undefined };
	return labels.reduce((group, label) => {
		const child = childGroup(group, label);
		if (!child) throw new Error(`sidebar has no group ${labels.join(' > ')}`);
		return child;
	}, root);
}

function childGroup(group: SidebarGroup, label: string): SidebarGroup | undefined {
	return group.entries.find((entry): entry is SidebarGroup => entry.type === 'group' && entry.label === label);
}

function findOrAddSubgroup(group: SidebarGroup, label: string): SidebarGroup {
	const found = childGroup(group, label);
	if (found) return found;

	const subgroup: SidebarGroup = { type: 'group', label, entries: [], collapsed: true, badge: undefined };
	group.entries.push(subgroup);
	return subgroup;
}

function flatten(entries: SidebarEntry[]): SidebarLink[] {
	return entries.flatMap((entry) => (entry.type === 'link' ? [entry] : flatten(entry.entries)));
}

function samePath(a: string, b: string): boolean {
	return a.replace(/\/$/, '') === b.replace(/\/$/, '');
}
