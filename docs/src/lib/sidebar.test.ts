import { describe, expect, test } from 'bun:test';
import { addDataPages, type DataPage, type Route } from './sidebar';

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

function page(href: string, fields: Partial<DataPage> = {}): DataPage {
	return { group: ['Reference', 'Functions'], label: href, href, experimental: false, ...fields };
}

// Renders the sidebar as one string per link, indented by group depth.
function outline(entries: SidebarEntry[], depth = 0): string[] {
	return entries.flatMap((entry) =>
		entry.type === 'link'
			? [`${'  '.repeat(depth)}${entry.href}${entry.isCurrent ? ' *' : ''}${entry.badge ? ` [${entry.badge.text}]` : ''}`]
			: [`${'  '.repeat(depth)}${entry.label}:`, ...outline(entry.entries, depth + 1)],
	);
}

function sidebar(): SidebarEntry[] {
	return [group('Reference', [group('Functions', [link('/fn/')]), link('/cli/')])];
}

describe('addDataPages', () => {
	test('appends pages to their group in order', () => {
		const r = route(sidebar());

		addDataPages(r, [page('/fn/b/'), page('/fn/a/', { experimental: true })], '/elsewhere/');

		expect(outline(r.sidebar)).toEqual([
			'Reference:',
			'  Functions:',
			'    /fn/',
			'    /fn/b/',
			'    /fn/a/ [exp]',
			'  /cli/',
		]);
	});

	test('groups pages that share a subgroup', () => {
		const r = route(sidebar());

		addDataPages(
			r,
			[page('/fn/run/'), page('/fn/stack/run/', { subgroup: 'stack' }), page('/fn/find/'), page('/fn/stack/clean/', { subgroup: 'stack' })],
			'/elsewhere/',
		);

		expect(outline(r.sidebar)).toEqual([
			'Reference:',
			'  Functions:',
			'    /fn/',
			'    /fn/run/',
			'    stack:',
			'      /fn/stack/run/',
			'      /fn/stack/clean/',
			'    /fn/find/',
			'  /cli/',
		]);
	});

	test('marks the current page with or without a trailing slash', () => {
		const r = route(sidebar());

		addDataPages(r, [page('/fn/a/'), page('/fn/b/')], '/fn/b');

		expect(outline(r.sidebar)).toContain('    /fn/b/ *');
	});

	test('sets the previous and next links around the current page', () => {
		const r = route(sidebar());

		addDataPages(r, [page('/fn/a/'), page('/fn/b/')], '/fn/a/');

		expect(r.pagination.prev?.href).toBe('/fn/');
		expect(r.pagination.next?.href).toBe('/fn/b/');
	});

	test('sets the previous and next links of a configured page next to the added pages', () => {
		const r = route([group('Reference', [group('Functions', [link('/fn/', true)]), link('/cli/')])]);

		addDataPages(r, [page('/fn/a/')], '/fn/');

		expect(r.pagination.next?.href).toBe('/fn/a/');
	});

	test('keeps previous and next links the page sets', () => {
		const r = route(sidebar(), { prev: false, next: { link: '/elsewhere/', label: 'Elsewhere' } });
		r.pagination.next = link('/elsewhere/');

		addDataPages(r, [page('/fn/a/')], '/fn/a/');

		expect(r.pagination.prev).toBeUndefined();
		expect(r.pagination.next?.href).toBe('/elsewhere/');
	});

	test('fails when the group is not in the sidebar', () => {
		expect(() => addDataPages(route(sidebar()), [page('/x/', { group: ['Reference', 'Nope'] })], '/x/')).toThrow(
			'sidebar has no group Reference > Nope',
		);
	});
});
