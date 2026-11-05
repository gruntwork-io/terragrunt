import { describe, expect, it } from 'bun:test';
import { hasRenderableBody } from './docs-collection';

describe('hasRenderableBody', () => {
	it('rejects a missing or blank body', () => {
		expect(hasRenderableBody(undefined)).toBe(false);
		expect(hasRenderableBody(' \n')).toBe(false);
	});

	it('rejects a body of only HTML comments', () => {
		expect(hasRenderableBody('<!-- one -->\n<!-- two\nlines -->\n')).toBe(false);
	});

	it('rejects a body of only MDX comments', () => {
		expect(hasRenderableBody('{/* one\ntwo */}\n')).toBe(false);
	});

	it('accepts content that follows a comment', () => {
		expect(hasRenderableBody('{/* note */}\n\n## Heading\n')).toBe(true);
	});
});
