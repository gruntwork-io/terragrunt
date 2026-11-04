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

/** Returns how a call to a function reads, with optional parameters in brackets, e.g. `get_env(name, [default])`. */
export function functionSignature(data: CollectionEntry<'hclFunctions'>['data']): string {
	const params = data.params.map((param) => (param.optional ? `[${param.name}]` : param.name));
	return `${data.name}(${params.join(', ')})`;
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
 * Returns the Markdown an entry's structured fields render to, split into the
 * part that goes before the entry's body and the part that goes after it.
 */
export function referenceMarkdown(entry: HclEntry): { before: string; after: string } {
	switch (entry.collection) {
		case 'hclBlocks': {
			const attributes = entry.data.attributes ?? [];
			const examples = entry.data.examples ? [examplesMarkdown(entry.data.examples)] : [];
			const sections = attributes.length ? ['## Attributes', ...attributes.map(blockAttributeMarkdown)] : [];
			return { before: '## Usage', after: [...examples, ...sections].join('\n\n') };
		}
		case 'hclAttributes': {
			const lines = [`**Type:** ${code(entry.data.type)}`];
			if (entry.data.default !== undefined) lines.push(`**Default:** ${code(entry.data.default)}`);
			return {
				before: [lines.join('\\\n'), '## Usage'].join('\n\n'),
				after: examplesMarkdown(entry.data.examples),
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
			const attributes = entry.data.attributes ?? [];
			if (attributes.length === 0) return [];
			return [
				{ depth: 2, slug: 'attributes', text: 'Attributes' },
				...attributes.map((attr) => ({ depth: 3, slug: attr.name, text: attr.name })),
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
