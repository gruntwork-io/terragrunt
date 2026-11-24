import { describe, expect, test } from 'bun:test';
import { defaultFitsBadge, typeBadgeLabel, typeBadgeVariant, type TypeBadgeVariant } from './type-badge';

describe('typeBadgeVariant', () => {
	test.each<[string, TypeBadgeVariant]>([
		['bool', 'tip'],
		['string', 'success'],
		['list(string)', 'note'],
		['integer', 'default'],
	])('colors %s as %s', (type, variant) => {
		expect(typeBadgeVariant(type)).toBe(variant);
	});
});

describe('defaultFitsBadge', () => {
	test('counts the quotes around a string', () => {
		expect(defaultFitsBadge('string', '1234567890')).toBe(true);
		expect(defaultFitsBadge('string', '12345678901')).toBe(false);
	});

	test('counts the value alone for other types', () => {
		expect(defaultFitsBadge('integer', '123456789012')).toBe(true);
		expect(defaultFitsBadge('integer', '1234567890123')).toBe(false);
	});
});

describe('typeBadgeLabel', () => {
	test('leaves a non-optional type bare', () => {
		expect(typeBadgeLabel('list(string)', false, undefined)).toBe('list(string)');
	});

	test('wraps an optional type', () => {
		expect(typeBadgeLabel('bool', true, undefined)).toBe('optional(bool)');
	});

	test('includes a short default', () => {
		expect(typeBadgeLabel('bool', true, 'false')).toBe('optional(bool, false)');
		expect(typeBadgeLabel('integer', true, '1')).toBe('optional(integer, 1)');
	});

	test('quotes a string default', () => {
		expect(typeBadgeLabel('string', true, 'a')).toBe('optional(string, "a")');
	});

	test('replaces a long default with an ellipsis', () => {
		expect(typeBadgeLabel('string', true, '.terragrunt-excludes')).toBe('optional(string, "…")');
		expect(typeBadgeLabel('integer', true, '1234567890123')).toBe('optional(integer, …)');
	});
});
