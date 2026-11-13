import type { CollectionEntry } from 'astro:content';

export const HCL_KINDS = ['blocks', 'attributes', 'functions'] as const;

export type HclKind = (typeof HCL_KINDS)[number];

export const HCL_COLLECTIONS = {
	blocks: 'hclBlocks',
	attributes: 'hclAttributes',
	functions: 'hclFunctions',
} as const satisfies Record<HclKind, string>;

export type HclEntry =
	| CollectionEntry<'hclBlocks'>
	| CollectionEntry<'hclAttributes'>
	| CollectionEntry<'hclFunctions'>;

export type BlockAttribute = NonNullable<CollectionEntry<'hclBlocks'>['data']['attributes']>[number];

export type BlockLabel = NonNullable<CollectionEntry<'hclBlocks'>['data']['labels']>[number];

export type NestedBlock = NonNullable<CollectionEntry<'hclBlocks'>['data']['blocks']>[number];

export type KeyGroups = NonNullable<BlockAttribute['keyGroups']>;

export type MapKey = KeyGroups['groups'][number]['keys'][number];

export type HclConfig = HclEntry['data']['configs'][number];

type Heading = { depth: number; slug: string; text: string };

/** Returns the docs slug of an item's page, e.g. `reference/hcl/functions/get_env`. */
export function hclItemSlug(kind: HclKind, name: string): string {
	return `reference/hcl/${kind}/${name}`;
}

/** Returns the URL path of an item's page, e.g. `/reference/hcl/functions/get_env/`. */
export function hclItemPath(kind: HclKind, name: string): string {
	return `/${hclItemSlug(kind, name)}/`;
}

/**
 * Returns how a call to a function reads, with each parameter's type after its
 * name, variadic parameters after `...`, optional parameters in brackets, and
 * the return type after an arrow, e.g. `get_env(name: string, [default: string]) --> string`.
 */
export function functionSignature(data: CollectionEntry<'hclFunctions'>['data']): string {
	const params = data.params.map((param) => {
		const typed = `${param.variadic ? '...' : ''}${param.name}: ${param.type}`;
		return param.optional ? `[${typed}]` : typed;
	});
	return `${data.name}(${params.join(', ')}) --> ${data.returns}`;
}

/** Returns an item page's headings in page order, given the headings of the entry's body. */
export function itemHeadings(entry: HclEntry, bodyHeadings: Heading[]): Heading[] {
	const usage = [
		{ depth: 2, slug: 'usage', text: 'Usage' },
		...bodyHeadings.filter((heading) => heading.depth <= 3),
	];
	const examples = entry.data.examples ? [{ depth: 2, slug: 'examples', text: 'Examples' }] : [];

	if (entry.collection === 'hclFunctions') return [...usage, ...referenceHeadings(entry), ...examples];
	return [...usage, ...examples, ...referenceHeadings(entry)];
}

/**
 * Returns the fragments an item's page defines besides its fixed sections: the
 * slugs of the body's headings and, for a block, its attribute names, key
 * group sections, and nested block names.
 */
export function itemAnchors(entry: HclEntry, bodyHeadings: Heading[]): string[] {
	if (entry.collection !== 'hclBlocks') return bodyHeadings.map((heading) => heading.slug);
	const attributes = entry.data.attributes ?? [];
	const keyGroups = attributeKeyGroups(attributes).flatMap((keys) => [keys.slug, ...keys.groups.map((group) => group.name)]);
	const blocks = (entry.data.blocks ?? []).map((block) => block.name);
	return [
		...bodyHeadings.map((heading) => heading.slug),
		...attributes.map((attr) => attr.name),
		...keyGroups,
		...blocks,
	];
}

/**
 * Maps each fragment of a kind's index page to the item page it is on now, for
 * `AnchorRedirects`. An item's name maps to its page, and each of its anchors
 * to that fragment on its page. An anchor that more than one item defines, or
 * that is also an item's name, is left out.
 */
export function anchorRedirects(kind: HclKind, items: { name: string; anchors: string[] }[]): Record<string, string> {
	const names = new Set(items.map((item) => item.name));
	const owners = new Map<string, string[]>();
	for (const item of items) {
		for (const anchor of new Set(item.anchors)) {
			owners.set(anchor, [...(owners.get(anchor) ?? []), item.name]);
		}
	}

	const redirects = Object.fromEntries(items.map((item) => [item.name, hclItemPath(kind, item.name)]));
	for (const [anchor, [owner, ...others]] of owners) {
		if (names.has(anchor) || others.length > 0) continue;
		redirects[anchor] = `${hclItemPath(kind, owner)}#${anchor}`;
	}
	return redirects;
}

/**
 * Returns the Markdown an entry's structured fields render to, split into the
 * part that goes before the entry's body and the part that goes after it.
 */
export function referenceMarkdown(entry: HclEntry): { before: string; after: string } {
	switch (entry.collection) {
		case 'hclBlocks': {
			const labels = entry.data.labels ?? [];
			const attributes = entry.data.attributes ?? [];
			const examples = entry.data.examples ? [examplesMarkdown(entry.data.examples)] : [];
			const blocks = entry.data.blocks ?? [];
			const labelSections = labels.length ? ['## Labels', labels.map(blockLabelMarkdown).join('\n')] : [];
			const sections = attributes.length ? ['## Attributes', ...attributes.map(blockAttributeMarkdown)] : [];
			const keySections = attributeKeyGroups(attributes).map(keyGroupsMarkdown);
			const blockSections = blocks.length ? ['## Blocks', ...blocks.map(nestedBlockMarkdown)] : [];
			return {
				before: '## Usage',
				after: [...examples, ...labelSections, ...sections, ...keySections, ...blockSections].join('\n\n'),
			};
		}
		case 'hclAttributes': {
			const lines = [`**Type:** ${code(entry.data.type)}`];
			if (entry.data.default !== undefined) lines.push(`**Default:** ${code(entry.data.default)}`);
			const { note: attributeNote } = entry.data;
			const note = attributeNote ? [`> **${attributeNote.title}:** ${attributeNote.body}`] : [];
			return {
				before: [lines.join('\\\n'), '## Usage'].join('\n\n'),
				after: [examplesMarkdown(entry.data.examples), ...note].join('\n\n'),
			};
		}
		case 'hclFunctions': {
			const sections = entry.data.params.length
				? [
						'## Parameters',
						markdownTable(
							['Name', 'Type', 'Required', 'Description'],
							entry.data.params.map((param) => [
								code(param.name),
								code(param.type),
								param.optional ? 'No' : 'Yes',
								param.description,
							]),
						),
					]
				: [];
			return {
				before: [
					`**Returns:** ${code(entry.data.returns)}`,
					'## Usage',
					hclFence(functionSignature(entry.data)),
				].join('\n\n'),
				after: [...sections, examplesMarkdown(entry.data.examples)].join('\n\n'),
			};
		}
	}
}

function referenceHeadings(entry: HclEntry): Heading[] {
	switch (entry.collection) {
		case 'hclBlocks': {
			const labels = entry.data.labels?.length ? [{ depth: 2, slug: 'labels', text: 'Labels' }] : [];
			const attributes = entry.data.attributes ?? [];
			const blocks = entry.data.blocks ?? [];
			return [
				...labels,
				...namedSection('attributes', 'Attributes', attributes),
				...attributeKeyGroups(attributes).flatMap((keys) => [
					{ depth: 2, slug: keys.slug, text: keys.title },
					...keys.groups.map((group) => ({ depth: 3, slug: group.name, text: group.title })),
				]),
				...namedSection('blocks', 'Blocks', blocks),
			];
		}
		case 'hclAttributes':
			return [];
		case 'hclFunctions':
			return entry.data.params.length ? [{ depth: 2, slug: 'parameters', text: 'Parameters' }] : [];
	}
}

function examplesMarkdown(examples: NonNullable<HclEntry['data']['examples']>): string {
	const lines = examples.flatMap((example) => [
		...(example.description ? [example.description] : []),
		hclFence(example.code),
	]);
	return ['## Examples', ...lines].join('\n\n');
}

function namedSection(slug: string, text: string, items: { name: string }[]): Heading[] {
	if (items.length === 0) return [];
	return [{ depth: 2, slug, text }, ...items.map((item) => ({ depth: 3, slug: item.name, text: item.name }))];
}

function attributeKeyGroups(attributes: BlockAttribute[]): KeyGroups[] {
	return attributes.flatMap((attr) => (attr.keyGroups ? [attr.keyGroups] : []));
}

function keyGroupsMarkdown(keys: KeyGroups): string {
	const groups = keys.groups.map((group) =>
		[
			`### ${group.title}`,
			...(group.description ? [group.description] : []),
			group.keys.map((key) => mapKeyMarkdown(key, '')).join('\n'),
			...(group.note ? [group.note] : []),
		].join('\n\n'),
	);
	return [`## ${keys.title}`, keys.description, ...groups].join('\n\n');
}

function mapKeyMarkdown(key: MapKey, indent: string): string {
	const meta = [code(key.type), key.required ? 'required' : 'optional'];
	if (key.default !== undefined) meta.push(`default ${code(key.default)}`);
	if (key.provisioning) meta.push('used by Terragrunt for backend provisioning');
	if (key.deprecated) meta.push('deprecated');
	const line = `${indent}- ${code(key.name)} (${meta.join(', ')}): ${key.description}`;
	const children = (key.keys ?? []).map((child) => mapKeyMarkdown(child, `${indent}  `));
	return [line, ...children].join('\n');
}

function nestedBlockMarkdown(block: NestedBlock): string {
	const labels = (block.labels ?? []).map(blockLabelMarkdown);
	const attributes = block.attributes.map((attr) => {
		const meta = [code(attr.type), attr.required ? 'required' : 'optional'];
		if (attr.default !== undefined) meta.push(`default ${code(attr.default)}`);
		return `- ${code(attr.name)} (${meta.join(', ')}): ${attr.description}`;
	});
	const lists = [
		...(labels.length ? ['Labels:', labels.join('\n')] : []),
		...(attributes.length ? ['Attributes:', attributes.join('\n')] : []),
	];
	return [`### ${code(block.name)}`, block.description, ...lists, hclFence(block.example)].join('\n\n');
}

function blockLabelMarkdown(label: BlockLabel): string {
	return `- ${code(label.name)}: ${label.description}`;
}

function blockAttributeMarkdown(attr: BlockAttribute): string {
	const meta = [`- **Type:** ${code(attr.type)}`, `- **Required:** ${attr.required ? 'Yes' : 'No'}`];
	if (attr.default !== undefined) meta.push(`- **Default:** ${code(attr.default)}`);
	return [`### ${code(attr.name)}`, attr.description, meta.join('\n'), hclFence(attr.example)].join('\n\n');
}

function code(text: string): string {
	return '`' + text + '`';
}

function hclFence(source: string): string {
	return '```hcl\n' + source.trim() + '\n```';
}

function markdownTable(headers: string[], rows: string[][]): string {
	const line = (cells: string[]) => `| ${cells.map((cell) => cell.replaceAll('|', '\\|').replaceAll('\n', ' ')).join(' | ')} |`;
	return [line(headers), line(headers.map(() => '---')), ...rows.map(line)].join('\n');
}
