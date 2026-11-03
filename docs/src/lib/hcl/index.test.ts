import { describe, expect, test } from 'bun:test';
import type { CollectionEntry } from 'astro:content';
import { functionSignature, hclItemPath, referenceHeadings, referenceMarkdown } from '.';

type FunctionData = CollectionEntry<'hclFunctions'>['data'];
type BlockData = CollectionEntry<'hclBlocks'>['data'];
type AttributeData = CollectionEntry<'hclAttributes'>['data'];

function functionEntry(data: Partial<FunctionData>): CollectionEntry<'hclFunctions'> {
	return {
		id: 'fn',
		collection: 'hclFunctions',
		data: {
			name: 'fn',
			description: 'Does a thing.',
			configs: ['unit'],
			params: [],
			returns: { type: 'string', description: 'The result.' },
			...data,
		},
	} as CollectionEntry<'hclFunctions'>;
}

function blockEntry(data: Partial<BlockData>): CollectionEntry<'hclBlocks'> {
	return {
		id: 'blk',
		collection: 'hclBlocks',
		data: { name: 'blk', description: 'A block.', configs: ['unit'], ...data },
	} as CollectionEntry<'hclBlocks'>;
}

function attributeEntry(data: Partial<AttributeData>): CollectionEntry<'hclAttributes'> {
	return {
		id: 'attr',
		collection: 'hclAttributes',
		data: { name: 'attr', description: 'An attribute.', configs: ['unit'], type: 'bool', ...data },
	} as CollectionEntry<'hclAttributes'>;
}

describe('hclItemPath', () => {
	test('keeps underscores in the name', () => {
		expect(hclItemPath('functions', 'get_env')).toBe('/reference/hcl/functions/get_env/');
	});
});

describe('functionSignature', () => {
	test('renders a function with no parameters', () => {
		expect(functionSignature(functionEntry({ name: 'get_platform' }).data)).toBe('get_platform()');
	});

	test('brackets optional parameters', () => {
		const data = functionEntry({
			name: 'get_env',
			params: [
				{ name: 'name', type: 'string', optional: false, description: '' },
				{ name: 'default', type: 'string', optional: true, description: '' },
			],
		}).data;
		expect(functionSignature(data)).toBe('get_env(name, [default])');
	});
});

describe('referenceHeadings', () => {
	test('omits Attributes for a block with none', () => {
		expect(referenceHeadings(blockEntry({}))).toEqual([]);
	});

	test('lists Attributes for a block with some', () => {
		const entry = blockEntry({
			attributes: [{ name: 'urls', type: 'list(string)', required: true, description: 'URLs.' }],
		});
		expect(referenceHeadings(entry).map((heading) => heading.slug)).toEqual(['attributes']);
	});

	test('omits Parameters for a function with none', () => {
		expect(referenceHeadings(functionEntry({})).map((heading) => heading.slug)).toEqual(['returns']);
	});

	test('lists nothing for an attribute', () => {
		expect(referenceHeadings(attributeEntry({}))).toEqual([]);
	});
});

describe('referenceMarkdown', () => {
	test('escapes pipes in table cells', () => {
		const entry = blockEntry({
			attributes: [{ name: 'mode', type: 'string', required: false, description: 'Either `a|b` or `c`.' }],
		});
		expect(referenceMarkdown(entry).after).toContain('Either `a\\|b` or `c`.');
	});

	test('shows an attribute default only when one is set', () => {
		expect(referenceMarkdown(attributeEntry({ default: 'false' })).before).toContain('**Default:** `false`');
		expect(referenceMarkdown(attributeEntry({})).before).not.toContain('Default');
	});

	test('puts the function signature before the body and the parameters after it', () => {
		const { before, after } = referenceMarkdown(
			functionEntry({
				name: 'get_env',
				configs: ['unit', 'stack'],
				params: [{ name: 'name', type: 'string', optional: false, description: 'Variable name.' }],
			}),
		);
		expect(before).toBe('**Used in:** Unit, Stack\n\n```hcl\nget_env(name)\n```');
		expect(after).toBe(
			[
				'## Parameters',
				'| Name | Type | Required | Description |\n| --- | --- | --- | --- |\n| `name` | `string` | Yes | Variable name. |',
				'## Returns',
				'`string`: The result.',
			].join('\n\n'),
		);
	});
});
