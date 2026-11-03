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

export type HclConfig = HclEntry['data']['configs'][number];

const CONFIG_LABELS: Record<HclConfig, string> = {
	unit: 'Unit',
	stack: 'Stack',
};

type Heading = { depth: number; slug: string; text: string };

/** Returns the docs slug of an item's page, e.g. `reference/hcl/functions/get_env`. */
export function hclItemSlug(kind: HclKind, name: string): string {
	return `reference/hcl/${kind}/${name}`;
}

/** Returns the URL path of an item's page, e.g. `/reference/hcl/functions/get_env/`. */
export function hclItemPath(kind: HclKind, name: string): string {
	return `/${hclItemSlug(kind, name)}/`;
}

/** Returns the label shown for a configuration kind, e.g. `Unit`. */
export function configLabel(config: HclConfig): string {
	return CONFIG_LABELS[config];
}

/** Returns how a call to a function reads, with optional parameters in brackets, e.g. `get_env(name, [default])`. */
export function functionSignature(data: CollectionEntry<'hclFunctions'>['data']): string {
	const params = data.params.map((param) => (param.optional ? `[${param.name}]` : param.name));
	return `${data.name}(${params.join(', ')})`;
}

/** Returns the headings of the sections rendered from an entry's structured fields, in page order. */
export function referenceHeadings(entry: HclEntry): Heading[] {
	switch (entry.collection) {
		case 'hclBlocks':
			return entry.data.attributes?.length ? [{ depth: 2, slug: 'attributes', text: 'Attributes' }] : [];
		case 'hclAttributes':
			return [];
		case 'hclFunctions':
			return [
				...(entry.data.params.length ? [{ depth: 2, slug: 'parameters', text: 'Parameters' }] : []),
				{ depth: 2, slug: 'returns', text: 'Returns' },
			];
	}
}

/**
 * Returns the Markdown an entry's structured fields render to, split into the
 * part that goes before the entry's body and the part that goes after it.
 */
export function referenceMarkdown(entry: HclEntry): { before: string; after: string } {
	const usedIn = `**Used in:** ${entry.data.configs.map(configLabel).join(', ')}`;

	switch (entry.collection) {
		case 'hclBlocks': {
			const attributes = entry.data.attributes ?? [];
			const after = attributes.length
				? [
						'## Attributes',
						markdownTable(
							['Name', 'Type', 'Required', 'Default', 'Description'],
							attributes.map((attr) => [
								code(attr.name),
								code(attr.type),
								attr.required ? 'Yes' : 'No',
								attr.default === undefined ? '' : code(attr.default),
								attr.description,
							]),
						),
					].join('\n\n')
				: '';
			return { before: usedIn, after };
		}
		case 'hclAttributes': {
			const lines = [usedIn, `**Type:** ${code(entry.data.type)}`];
			if (entry.data.default !== undefined) lines.push(`**Default:** ${code(entry.data.default)}`);
			return { before: lines.join('\\\n'), after: '' };
		}
		case 'hclFunctions': {
			const sections: string[] = [];
			if (entry.data.params.length) {
				sections.push(
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
				);
			}
			sections.push('## Returns', `${code(entry.data.returns.type)}: ${entry.data.returns.description}`);
			return {
				before: [usedIn, '```hcl\n' + functionSignature(entry.data) + '\n```'].join('\n\n'),
				after: sections.join('\n\n'),
			};
		}
	}
}

function code(text: string): string {
	return '`' + text + '`';
}

function markdownTable(headers: string[], rows: string[][]): string {
	const line = (cells: string[]) => `| ${cells.map((cell) => cell.replaceAll('|', '\\|').replaceAll('\n', ' ')).join(' | ')} |`;
	return [line(headers), line(headers.map(() => '---')), ...rows.map(line)].join('\n');
}
