import type { StarlightRouteData } from '@astrojs/starlight/route-data';

type SidebarEntry = StarlightRouteData['sidebar'][number];

type SidebarLink = Extract<SidebarEntry, { type: 'link' }>;

export type Route = Pick<StarlightRouteData, 'sidebar' | 'pagination'> & {
	entry: { data: Pick<StarlightRouteData['entry']['data'], 'prev' | 'next'> };
};

/**
 * Reorders the links to HCL item pages in each sidebar group to follow
 * `hrefs`, and moves the page's previous and next links to match. Every other
 * entry keeps its place, as does a previous or next link set by the page.
 */
export function sortHclSidebar(route: Route, hrefs: string[]): void {
	const rank = new Map(hrefs.map((href, i) => [withoutTrailingSlash(href), i]));
	sortEntries(route.sidebar, rank);

	const { prev, next } = neighbors(route.sidebar);
	if (route.entry.data.prev === undefined) route.pagination.prev = prev;
	if (route.entry.data.next === undefined) route.pagination.next = next;
}

function sortEntries(entries: SidebarEntry[], rank: Map<string, number>): void {
	const rankOf = (entry: SidebarEntry) =>
		entry.type === 'link' ? rank.get(withoutTrailingSlash(entry.href)) : undefined;

	const slots = entries.flatMap((entry, i) => (rankOf(entry) === undefined ? [] : [i]));
	const sorted = slots.map((i) => entries[i]).sort((a, b) => rankOf(a)! - rankOf(b)!);
	slots.forEach((slot, i) => {
		entries[slot] = sorted[i];
	});

	for (const entry of entries) {
		if (entry.type === 'group') sortEntries(entry.entries, rank);
	}
}

function neighbors(sidebar: SidebarEntry[]): { prev?: SidebarLink; next?: SidebarLink } {
	const links = flatten(sidebar);
	const current = links.findIndex((link) => link.isCurrent);
	if (current === -1) return {};
	return { prev: links[current - 1], next: links[current + 1] };
}

function flatten(entries: SidebarEntry[]): SidebarLink[] {
	return entries.flatMap((entry) => (entry.type === 'link' ? [entry] : flatten(entry.entries)));
}

function withoutTrailingSlash(href: string): string {
	return href.replace(/\/$/, '');
}
