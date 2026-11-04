import { describe, expect, test } from 'bun:test';
import type { CollectionEntry } from 'astro:content';
import { functionSignature, hclItemPath, itemHeadings, referenceMarkdown } from '.';

type FunctionData = CollectionEntry<'hclFunctions'>['data'];
type BlockData = CollectionEntry<'hclBlocks'>['data'];
type AttributeData = CollectionEntry<'hclAttributes'>['data'];

const examples: FunctionData['examples'] = [{ code: 'x = 1\n' }];

function functionEntry(data: Partial<FunctionData>): CollectionEntry<'hclFunctions'> {
	return {
		id: 'fn',
		collection: 'hclFunctions',
		data: {
			name: 'fn',
			description: 'Does a thing.',
			configs: ['unit'],
			examples,
			params: [],
			returns: 'string',
			...data,
		},
	} as CollectionEntry<'hclFunctions'>;
}

function blockEntry(data: Partial<BlockData>): CollectionEntry<'hclBlocks'> {
	return {
		id: 'blk',
		collection: 'hclBlocks',
		data: { name: 'blk', description: 'A block.', configs: ['unit'], examples, ...data },
	} as CollectionEntry<'hclBlocks'>;
}

function attributeEntry(data: Partial<AttributeData>): CollectionEntry<'hclAttributes'> {
	return {
		id: 'attr',
		collection: 'hclAttributes',
		data: { name: 'attr', description: 'An attribute.', configs: ['unit'], examples, type: 'bool', ...data },
	} as CollectionEntry<'hclAttributes'>;
}

describe('hclItemPath', () => {
	test('keeps underscores in the name', () => {
		expect(hclItemPath('functions', 'get_env')).toBe('/reference/hcl/functions/get_env/');
	});
});

describe('functionSignature', () => {
	test('renders a function with no parameters', () => {
		expect(functionSignature(functionEntry({ name: 'get_platform' }).data)).toBe('get_platform() --> string');
	});

	test('brackets optional parameters', () => {
		const data = functionEntry({
			name: 'get_env',
			params: [
				{ name: 'name', type: 'string', optional: false, description: '' },
				{ name: 'default', type: 'string', optional: true, description: '' },
			],
		}).data;
		expect(functionSignature(data)).toBe('get_env(name: string, [default: string]) --> string');
	});
});

describe('itemHeadings', () => {
	const slugs = (entry: Parameters<typeof itemHeadings>[0]) => itemHeadings(entry, []).map((heading) => heading.slug);

	test('puts body headings between Usage and Examples, and drops deep ones', () => {
		const body = [
			{ depth: 2, slug: 'caching', text: 'Caching' },
			{ depth: 3, slug: 'keys', text: 'Keys' },
			{ depth: 4, slug: 'hashing', text: 'Hashing' },
		];
		expect(itemHeadings(attributeEntry({}), body).map((heading) => heading.slug)).toEqual([
			'usage',
			'caching',
			'keys',
			'examples',
		]);
	});

	test('omits Attributes for a block with none', () => {
		expect(slugs(blockEntry({}))).toEqual(['usage', 'examples']);
	});

	test('nests each block attribute under Attributes', () => {
		const entry = blockEntry({
			attributes: [{ name: 'urls', type: 'list(string)', required: true, description: 'URLs.', example: '' }],
		});
		expect(itemHeadings(entry, []).slice(-2)).toEqual([
			{ depth: 2, slug: 'attributes', text: 'Attributes' },
			{ depth: 3, slug: 'urls', text: 'urls' },
		]);
	});

	test('omits Examples for a block with none', () => {
		const entry = blockEntry({
			examples: undefined,
			attributes: [{ name: 'urls', type: 'list(string)', required: true, description: 'URLs.', example: '' }],
		});
		expect(slugs(entry)).toEqual(['usage', 'attributes', 'urls']);
	});

	test('omits Parameters for a function with none', () => {
		expect(slugs(functionEntry({}))).toEqual(['usage', 'examples']);
	});

	test('puts Parameters before Examples for a function with some', () => {
		const entry = functionEntry({
			params: [{ name: 'name', type: 'string', optional: false, description: 'Variable name.' }],
		});
		expect(slugs(entry)).toEqual(['usage', 'parameters', 'examples']);
	});

	test('adds nothing past Examples for an attribute', () => {
		expect(slugs(attributeEntry({}))).toEqual(['usage', 'examples']);
	});
});

describe('referenceMarkdown', () => {
	test('escapes pipes in table cells', () => {
		const entry = functionEntry({
			params: [{ name: 'mode', type: 'string', optional: false, description: 'Either `a|b` or `c`.' }],
		});
		expect(referenceMarkdown(entry).after).toContain('Either `a\\|b` or `c`.');
	});

	test('renders examples after the body, with descriptions when set', () => {
		const entry = blockEntry({
			examples: [{ description: 'Sets `x`.', code: 'x = 1\n' }, { code: 'y = 2\n' }],
		});
		expect(referenceMarkdown(entry)).toEqual({
			before: '## Usage',
			after: ['## Examples', 'Sets `x`.', '```hcl\nx = 1\n```', '```hcl\ny = 2\n```'].join('\n\n'),
		});
	});

	test('renders no Examples section for a block without examples', () => {
		const entry = blockEntry({
			examples: undefined,
			attributes: [{ name: 'urls', type: 'bool', required: true, description: 'URLs.', example: 'urls = true' }],
		});
		expect(referenceMarkdown(entry).after).not.toContain('## Examples');
		expect(referenceMarkdown(entry).after).toContain('## Attributes');
	});

	test('renders each block attribute as a section with its metadata', () => {
		const entry = blockEntry({
			attributes: [
				{
					name: 'urls',
					type: 'list(string)',
					required: true,
					description: 'Catalog URLs.',
					example: 'catalog {\n  urls = ["a"]\n}\n',
				},
				{
					name: 'no_shell',
					type: 'bool',
					required: false,
					default: 'false',
					description: 'Disables shell.',
					example: 'no_shell = true',
				},
			],
		});
		expect(referenceMarkdown(entry).after).toBe(
			[
				'## Examples',
				'```hcl\nx = 1\n```',
				'## Attributes',
				'### `urls`',
				'Catalog URLs.',
				'- **Type:** `list(string)`\n- **Required:** Yes',
				'```hcl\ncatalog {\n  urls = ["a"]\n}\n```',
				'### `no_shell`',
				'Disables shell.',
				'- **Type:** `bool`\n- **Required:** No\n- **Default:** `false`',
				'```hcl\nno_shell = true\n```',
			].join('\n\n'),
		);
	});

	test('shows an attribute default only when one is set', () => {
		expect(referenceMarkdown(attributeEntry({ default: 'false' })).before).toContain('**Default:** `false`');
		expect(referenceMarkdown(attributeEntry({})).before).not.toContain('Default');
	});

	test('puts the return type and signature first and the parameters before the examples', () => {
		const { before, after } = referenceMarkdown(
			functionEntry({
				name: 'get_env',
				configs: ['unit', 'stack'],
				params: [{ name: 'name', type: 'string', optional: false, description: 'Variable name.' }],
			}),
		);
		expect(before).toBe('**Returns:** `string`\n\n## Usage\n\n```hcl\nget_env(name: string) --> string\n```');
		expect(after).toBe(
			[
				'## Parameters',
				'| Name | Type | Required | Description |\n| --- | --- | --- | --- |\n| `name` | `string` | Yes | Variable name. |',
				'## Examples',
				'```hcl\nx = 1\n```',
			].join('\n\n'),
		);
	});
});
