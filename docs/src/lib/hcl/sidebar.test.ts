import { describe, expect, test } from 'bun:test';
import { sortHclSidebar, type Route } from './sidebar';

type SidebarEntry = Route['sidebar'][number];
type SidebarLink = Extract<SidebarEntry, { type: 'link' }>;

function link(href: string, isCurrent = false): SidebarLink {
	return { type: 'link', label: href, href, isCurrent, badge: undefined, attrs: {} };
}

function group(label: string, entries: SidebarEntry[]): SidebarEntry {
	return { type: 'group', label, entries, collapsed: true, badge: undefined };
}

function route(sidebar: SidebarEntry[], data: Route['entry']['data'] = {}): Route {
	return { sidebar, pagination: { prev: undefined, next: undefined }, entry: { data } };
}

function hrefs(entries: SidebarEntry[]): string[] {
	return entries.flatMap((entry) => (entry.type === 'link' ? [entry.href] : hrefs(entry.entries)));
}

const order = ['/fn/a/', '/fn/b/', '/fn/z_exp/'];

describe('sortHclSidebar', () => {
	test('reorders item links within a group and leaves other links in place', () => {
		const r = route([
			group('Functions', [link('/fn/'), link('/fn/z_exp/'), link('/fn/a/'), link('/fn/b/')]),
			link('/cli/'),
		]);

		sortHclSidebar(r, order);

		expect(hrefs(r.sidebar)).toEqual(['/fn/', '/fn/a/', '/fn/b/', '/fn/z_exp/', '/cli/']);
	});

	test('matches hrefs with or without a trailing slash', () => {
		const r = route([group('Functions', [link('/fn/z_exp'), link('/fn/a')])]);

		sortHclSidebar(r, order);

		expect(hrefs(r.sidebar)).toEqual(['/fn/a', '/fn/z_exp']);
	});

	test('sets the previous and next links to the new neighbors', () => {
		const r = route([group('Functions', [link('/fn/z_exp/', true), link('/fn/a/'), link('/fn/b/')]), link('/cli/')]);

		sortHclSidebar(r, order);

		expect(r.pagination.prev?.href).toBe('/fn/b/');
		expect(r.pagination.next?.href).toBe('/cli/');
	});

	test('keeps previous and next links the page sets', () => {
		const r = route([group('Functions', [link('/fn/z_exp/', true), link('/fn/a/'), link('/fn/b/')]), link('/cli/')], {
			prev: false,
			next: { link: '/elsewhere/', label: 'Elsewhere' },
		});
		r.pagination.next = link('/elsewhere/');

		sortHclSidebar(r, order);

		expect(r.pagination.prev).toBeUndefined();
		expect(r.pagination.next?.href).toBe('/elsewhere/');
	});
});
