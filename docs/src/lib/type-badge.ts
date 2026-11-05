export type TypeBadgeVariant = 'tip' | 'success' | 'note' | 'default';

// A longer default wraps the badge onto two lines on a 390px wide phone.
const MAX_BADGE_DEFAULT_LENGTH = 12;

/** Returns the Badge variant that colors a flag or attribute type. */
export function typeBadgeVariant(type: string): TypeBadgeVariant {
	if (type === 'bool') return 'tip';
	if (type === 'string') return 'success';
	if (type.startsWith('list(')) return 'note';
	return 'default';
}

/** Returns true when `value` is short enough to show in the type badge. */
export function defaultFitsBadge(type: string, value: string): boolean {
	return defaultLiteral(type, value).length <= MAX_BADGE_DEFAULT_LENGTH;
}

/**
 * Returns the text of a type badge in OpenTofu's `optional()` syntax, e.g.
 * `optional(bool, false)`. A default too long for the badge appears as an
 * ellipsis, and `DefaultValue.astro` gives its full value.
 */
export function typeBadgeLabel(type: string, optional: boolean, defaultVal: string | undefined): string {
	if (!optional) return type;
	if (defaultVal === undefined) return `optional(${type})`;
	if (defaultFitsBadge(type, defaultVal)) return `optional(${type}, ${defaultLiteral(type, defaultVal)})`;
	return `optional(${type}, ${defaultLiteral(type, '…')})`;
}

function defaultLiteral(type: string, value: string): string {
	return type === 'string' ? `"${value}"` : value;
}
