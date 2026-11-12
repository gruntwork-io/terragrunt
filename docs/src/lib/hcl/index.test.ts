import { describe, expect, test } from 'bun:test';
import type { CollectionEntry } from 'astro:content';
import { anchorRedirects, functionSignature, hclItemPath, itemAnchors, itemHeadings, referenceMarkdown } from '.';

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
			{ depth: 5, slug: 'salt', text: 'Salt' },
		];
		expect(itemHeadings(attributeEntry({}), body).map((heading) => heading.slug)).toEqual([
			'usage',
			'caching',
			'keys',
			'hashing',
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

	test('puts Labels before Attributes for a block with both', () => {
		const entry = blockEntry({
			labels: [{ name: 'name', description: 'Name.' }],
			attributes: [{ name: 'path', type: 'string', required: true, description: 'Path.', example: '' }],
		});
		expect(slugs(entry)).toEqual(['usage', 'examples', 'labels', 'attributes', 'path']);
	});

	test('puts Blocks after Attributes, with an entry per nested block', () => {
		const entry = blockEntry({
			attributes: [{ name: 'source', type: 'string', required: true, description: 'Source.', example: '' }],
			blocks: [{ name: 'before_hook', description: 'Runs a command first.', labels: [{ name: 'name', description: 'Names the hook.' }], attributes: [{ name: 'commands', type: 'list(string)', required: true, description: 'Commands to run before.' }], example: 'before_hook "fmt" {}' }],
		});
		expect(slugs(entry)).toEqual(['usage', 'examples', 'attributes', 'source', 'blocks', 'before_hook']);
	});

	test('puts each key group after Attributes, with an entry per group', () => {
		const entry = blockEntry({ attributes: [{ name: 'config', type: 'map(any)', required: true, description: 'Config.', example: '', keyGroups: { slug: 'backend-config-keys', title: 'Backend config keys', selector: 'backend', description: 'Keys Terragrunt reads.', groups: [{ name: 's3', title: 'S3', keys: [{ name: 'bucket', type: 'string', required: true, provisioning: false, deprecated: false, description: 'The bucket.' }, { name: 'endpoints', type: 'object', required: false, provisioning: false, deprecated: false, description: 'Endpoints.', keys: [{ name: 's3', type: 'string', required: false, provisioning: false, deprecated: false, description: 'S3 endpoint.' }] }, { name: 's3_bucket_tags', type: 'map(string)', required: false, provisioning: true, deprecated: false, description: 'Tags.' }] }] } }] });
		expect(slugs(entry)).toEqual(['usage', 'examples', 'attributes', 'config', 'backend-config-keys', 's3']);
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

describe('itemAnchors', () => {
	const urls = { name: 'urls', type: 'list(string)', required: true, description: 'URLs.', example: '' };

	test('returns body heading slugs at every depth', () => {
		const body = [
			{ depth: 2, slug: 'caching', text: 'Caching' },
			{ depth: 4, slug: 'hashing', text: 'Hashing' },
		];
		expect(itemAnchors(functionEntry({}), body)).toEqual(['caching', 'hashing']);
	});

	test('adds attribute names for a block', () => {
		const body = [{ depth: 2, slug: 'merge-behavior', text: 'Merge behavior' }];
		expect(itemAnchors(blockEntry({ attributes: [urls] }), body)).toEqual(['merge-behavior', 'urls']);
	});

	test('adds key group anchors for a block', () => {
		expect(itemAnchors(blockEntry({ attributes: [{ name: 'config', type: 'map(any)', required: true, description: 'Config.', example: '', keyGroups: { slug: 'backend-config-keys', title: 'Backend config keys', selector: 'backend', description: 'Keys Terragrunt reads.', groups: [{ name: 's3', title: 'S3', keys: [{ name: 'bucket', type: 'string', required: true, provisioning: false, deprecated: false, description: 'The bucket.' }, { name: 'endpoints', type: 'object', required: false, provisioning: false, deprecated: false, description: 'Endpoints.', keys: [{ name: 's3', type: 'string', required: false, provisioning: false, deprecated: false, description: 'S3 endpoint.' }] }, { name: 's3_bucket_tags', type: 'map(string)', required: false, provisioning: true, deprecated: false, description: 'Tags.' }] }] } }] }), [])).toEqual(['config', 'backend-config-keys', 's3']);
	});

	test('adds nested block names for a block', () => {
		expect(itemAnchors(blockEntry({ attributes: [urls], blocks: [{ name: 'before_hook', description: 'Runs a command first.', labels: [{ name: 'name', description: 'Names the hook.' }], attributes: [{ name: 'commands', type: 'list(string)', required: true, description: 'Commands to run before.' }], example: 'before_hook "fmt" {}' }] }), [])).toEqual(['urls', 'before_hook']);
	});

	test('returns nothing for an attribute without body headings', () => {
		expect(itemAnchors(attributeEntry({}), [])).toEqual([]);
	});
});

describe('anchorRedirects', () => {
	test('sends names to the item page and anchors to a fragment on it', () => {
		expect(anchorRedirects('blocks', [{ name: 'remote_state', anchors: ['backend', 'encryption'] }])).toEqual({
			remote_state: '/reference/hcl/blocks/remote_state/',
			backend: '/reference/hcl/blocks/remote_state/#backend',
			encryption: '/reference/hcl/blocks/remote_state/#encryption',
		});
	});

	test('leaves out an anchor two items define', () => {
		const redirects = anchorRedirects('blocks', [
			{ name: 'unit', anchors: ['source'] },
			{ name: 'stack', anchors: ['source'] },
		]);
		expect(redirects).toEqual({ unit: '/reference/hcl/blocks/unit/', stack: '/reference/hcl/blocks/stack/' });
	});

	test("prefers an item name over another item's anchor of the same name", () => {
		const redirects = anchorRedirects('blocks', [
			{ name: 'generate', anchors: [] },
			{ name: 'remote_state', anchors: ['generate'] },
		]);
		expect(redirects.generate).toBe('/reference/hcl/blocks/generate/');
	});

	test('keeps an anchor one item defines twice', () => {
		const redirects = anchorRedirects('blocks', [{ name: 'remote_state', anchors: ['backend', 'backend'] }]);
		expect(redirects.backend).toBe('/reference/hcl/blocks/remote_state/#backend');
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

	test('renders an attribute note after its examples', () => {
		const entry = attributeEntry({ note: { title: 'Engines', body: 'Skipped with an [engine](/engine/).' } });
		expect(referenceMarkdown(entry).after).toBe(
			['## Examples', '```hcl\nx = 1\n```', '> **Engines:** Skipped with an [engine](/engine/).'].join('\n\n'),
		);
	});

	test('lists block labels after the examples', () => {
		const entry = blockEntry({ labels: [{ name: 'name', description: 'Names the unit.' }] });
		expect(referenceMarkdown(entry).after).toBe(
			['## Examples', '```hcl\nx = 1\n```', '## Labels', '- `name`: Names the unit.'].join('\n\n'),
		);
	});

	test('lists a nested block\'s labels and attributes before its example', () => {
		const entry = blockEntry({ blocks: [{ name: 'before_hook', description: 'Runs a command first.', labels: [{ name: 'name', description: 'Names the hook.' }], attributes: [{ name: 'commands', type: 'list(string)', required: true, description: 'Commands to run before.' }], example: 'before_hook "fmt" {}' }] });
		expect(referenceMarkdown(entry).after).toContain(
			[
				'## Blocks',
				'### `before_hook`',
				'Runs a command first.',
				'Labels:',
				'- `name`: Names the hook.',
				'Attributes:',
				'- `commands` (`list(string)`, required): Commands to run before.',
				'```hcl\nbefore_hook "fmt" {}\n```',
			].join('\n\n'),
		);
	});

	test('lists each key group\'s keys with nested keys indented and provisioning keys marked', () => {
		const entry = blockEntry({ attributes: [{ name: 'config', type: 'map(any)', required: true, description: 'Config.', example: '', keyGroups: { slug: 'backend-config-keys', title: 'Backend config keys', selector: 'backend', description: 'Keys Terragrunt reads.', groups: [{ name: 's3', title: 'S3', keys: [{ name: 'bucket', type: 'string', required: true, provisioning: false, deprecated: false, description: 'The bucket.' }, { name: 'endpoints', type: 'object', required: false, provisioning: false, deprecated: false, description: 'Endpoints.', keys: [{ name: 's3', type: 'string', required: false, provisioning: false, deprecated: false, description: 'S3 endpoint.' }] }, { name: 's3_bucket_tags', type: 'map(string)', required: false, provisioning: true, deprecated: false, description: 'Tags.' }] }] } }] });
		expect(referenceMarkdown(entry).after).toContain(
			[
				'## Backend config keys',
				'Keys Terragrunt reads.',
				'### S3',
				[
					'- `bucket` (`string`, required): The bucket.',
					'- `endpoints` (`object`, optional): Endpoints.',
					'  - `s3` (`string`, optional): S3 endpoint.',
					'- `s3_bucket_tags` (`map(string)`, optional, used by Terragrunt for backend provisioning): Tags.',
				].join('\n'),
			].join('\n\n'),
		);
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
