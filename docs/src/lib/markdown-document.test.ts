import { describe, expect, it } from 'bun:test';
import { SITE_DESCRIPTION, SITE_TITLE } from '../data/site';
import { llmsDocument, markdownDocument } from './markdown-document';

describe('markdownDocument', () => {
	it('frames the title, description, and body', () => {
		expect(markdownDocument('run', 'Run a command.', 'Body.')).toBe(
			'# run\n\n> Run a command.\n\nBody.\n',
		);
	});

	it('omits a missing description and an empty body', () => {
		expect(markdownDocument('run', undefined, '  ')).toBe('# run\n');
	});
});

describe('llmsDocument', () => {
	it('opens with the site title and description, then each page in order', () => {
		const document = llmsDocument('full', [
			{ title: 'run', description: 'Run.', body: 'Body.' },
			{ title: 'exec', body: 'Other.' },
		]);
		expect(document).toBe(
			`# ${SITE_TITLE}\n\n> ${SITE_DESCRIPTION}\n\nThis is the full documentation.\n\n` +
				'# run\n\n> Run.\n\nBody.\n\n# exec\n\nOther.\n',
		);
	});

	it('says what the small document omits', () => {
		expect(llmsDocument('small', [])).toContain('It omits notes, tips, and collapsible details.');
	});

	it('skips a page with an empty body', () => {
		const document = llmsDocument('full', [
			{ title: 'Completed Experiments', description: 'None yet.', body: ' \n' },
		]);
		expect(document).not.toContain('Completed Experiments');
	});
});
