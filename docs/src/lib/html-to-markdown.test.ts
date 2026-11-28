import { describe, expect, it } from 'bun:test';
import { htmlToMarkdown } from './html-to-markdown';

const line = (code: string) =>
	`<div class="ec-line"><div class="code"><span>${code}</span></div></div>`;

const codeBlock = (language: string, lines: string, title = '') =>
	`<div class="expressive-code"><link rel="stylesheet" href="/ec.css"><figure class="frame is-terminal">` +
	`<figcaption class="header"><span class="title">${title}</span><span class="sr-only">Terminal window</span></figcaption>` +
	`<pre data-language="${language}"><code>${lines}</code></pre>` +
	`<div class="copy"><div aria-live="polite"></div><button title="Copy" data-copied="Copied!" data-code="x"><div></div></button></div>` +
	`</figure></div>`;

const aside = (type: string, text: string) =>
	`<aside class="starlight-aside starlight-aside--${type}">` +
	`<p class="starlight-aside__title">${type}</p><div class="starlight-aside__content"><p>${text}</p></div></aside>`;

describe('htmlToMarkdown', () => {
	it('keeps code fenced with its language and drops the window chrome', async () => {
		const html = codeBlock('bash', line('terragrunt run') + line('\n') + line('  plan'));
		expect(await htmlToMarkdown(html)).toBe('```bash\nterragrunt run\n\n  plan\n```');
	});

	it('writes the title of a code block into its fence', async () => {
		const html = `<p>Set the inputs:</p>${codeBlock('hcl', line('inputs = {}'), 'terragrunt.hcl')}`;
		expect(await htmlToMarkdown(html)).toBe('Set the inputs:\n\n```hcl title="terragrunt.hcl"\ninputs = {}\n```');
		expect(await htmlToMarkdown(html, 'small')).toBe('Set the inputs:\n```hcl title="terragrunt.hcl"\ninputs = {}\n```');
	});

	it('drops heading anchors and HTML comments', async () => {
		const html =
			'<!-- note --><h2 id="usage">Usage<a class="sl-anchor-link" href="#usage">' +
			'<span class="sr-only">Section titled "Usage"</span></a></h2>' +
			'<h3 id="plan"><code>plan</code><a class="anchor-link" href="#plan"><span></span></a></h3>';
		expect(await htmlToMarkdown(html)).toBe('## Usage\n\n### `plan`');
	});

	it('keeps tables as tables', async () => {
		const html =
			'<table><thead><tr><th>Flag</th></tr></thead><tbody><tr><td>--all</td></tr></tbody></table>';
		expect(await htmlToMarkdown(html)).toBe('| Flag  |\n| ----- |\n| --all |');
	});

	it('lists each tab label with its panel', async () => {
		const html =
			'<starlight-tabs><ul role="tablist">' +
			'<li><a role="tab" href="#one"> Linux </a></li><li><a role="tab" href="#two">macOS</a></li></ul>' +
			'<div role="tabpanel"><p>First.</p></div><div role="tabpanel" hidden><p>Second.</p></div>' +
			'</starlight-tabs>';
		expect(await htmlToMarkdown(html)).toBe('* Linux\n\n  First.\n\n* macOS\n\n  Second.');
	});

	it('keeps the text of a code block that Expressive Code did not render', async () => {
		expect(await htmlToMarkdown('<pre><code>plain text\nline2</code></pre>')).toBe(
			'```\nplain text\nline2\n```',
		);
	});

	it('marks inserted and deleted lines and fences them as a diff', async () => {
		const html = codeBlock(
			'hcl',
			line('kept') +
				'<div class="ec-line ins"><div class="code"><span>added</span></div></div>' +
				'<div class="ec-line del"><div class="code"><span>removed</span></div></div>',
		);
		expect(await htmlToMarkdown(html)).toBe('```diff\nkept\n+added\n-removed\n```');
	});

	it('lists a panel without a matching tab label', async () => {
		const html =
			'<starlight-tabs><ul role="tablist"><li><a role="tab">One</a></li></ul>' +
			'<div role="tabpanel"><p>First.</p></div><div role="tabpanel"><p>Second.</p></div>' +
			'</starlight-tabs>';
		expect(await htmlToMarkdown(html)).toBe('* One\n\n  First.\n\n* Second.');
	});

	it('keeps every aside and details element in full output', async () => {
		const html = `${aside('note', 'A note.')}<details><summary>More</summary><p>Hidden.</p></details>`;
		const markdown = await htmlToMarkdown(html);
		expect(markdown).toContain('A note.');
		expect(markdown).toContain('Hidden.');
	});
});

describe('htmlToMarkdown small', () => {
	it('drops notes, tips, and details', async () => {
		const html =
			aside('note', 'A note.') +
			aside('tip', 'A tip.') +
			'<details><summary>More</summary><p>Hidden.</p></details><p>Kept.</p>';
		expect(await htmlToMarkdown(html, 'small')).toBe('Kept.');
	});

	it('keeps caution and danger asides', async () => {
		const html = aside('caution', 'Careful.') + aside('danger', 'Stop.');
		const markdown = await htmlToMarkdown(html, 'small');
		expect(markdown).toContain('Careful.');
		expect(markdown).toContain('Stop.');
	});

	it('removes blank lines between blocks and leaves code untouched', async () => {
		const html = `<h2>Usage</h2><p>Text.</p>${codeBlock('bash', line('one') + line('\n') + line('  two'))}`;
		expect(await htmlToMarkdown(html, 'small')).toBe(
			'## Usage\nText.\n```bash\none\n\n  two\n```',
		);
	});

	it('keeps the blank line that separates a list, quote, or paragraph from the next block', async () => {
		const html =
			'<ul><li>a</li></ul><p>After list.</p>' +
			'<blockquote><p>quoted</p></blockquote><p>After quote.</p><p>Next paragraph.</p>';
		expect(await htmlToMarkdown(html, 'small')).toBe(
			'* a\n\nAfter list.\n\n> quoted\n\nAfter quote.\n\nNext paragraph.',
		);
	});

	it('keeps tables as tables without padding', async () => {
		const html =
			'<table><thead><tr><th>Flag</th></tr></thead><tbody><tr><td>--all</td></tr></tbody></table>';
		expect(await htmlToMarkdown(html, 'small')).toBe('| Flag |\n| - |\n| --all |');
	});
});
