import { defineCollection } from 'astro:content';
import { z } from 'astro/zod';
import { docsLoader } from '@astrojs/starlight/loaders';
import { docsSchema } from '@astrojs/starlight/schema';
import { glob, file } from 'astro/loaders';
import { CHANGELOG_CATEGORY_SLUGS } from './lib/changelog';
import { publishedOnly } from './lib/release';
import { RELEASE_VERSION } from './lib/versions';

const releaseVersion = z.string().regex(RELEASE_VERSION, 'must have the form vX.Y.Z, e.g. v1.2.0');

const compatibilityVersion = z.string().regex(/^\d+\.\d+\.\d+$/, 'must have the form X.Y.Z, e.g. 1.2.0');

const commands = defineCollection({
	loader: glob({ pattern: "**/*.mdx", base: "src/data/commands" }),
	schema: z.object({
		name: z.string(),
		description: z.string(),
		path: z.string().regex(/^[a-z0-9-/]+$/),
		category: z.enum([
			"main",
			"backend",
			"stack",
			"catalog",
			"discovery",
			"configuration",
			"shortcuts",
		]),
		sidebar: z.object({
			parent: z.string().optional(),
			order: z.number(),
		}),
		usage: z.string(),
		examples: z.array(z.object({
			code: z.string(),
			description: z.string().optional(),
			lang: z.string().optional(),
		})),
		flags: z.array(z.string()).optional(),
		experiment: z.object({
			control: z.string(),
			name: z.string(),
		}).optional(),
		since: releaseVersion.optional(),
	}),
});

const docs = defineCollection({
	loader: publishedOnly(docsLoader()),
	schema: docsSchema({
		extend: z.object({
			since: releaseVersion.optional(),
		}),
	}),
});

const flags = defineCollection({
	loader: glob({ pattern: "**/*.mdx", base: "src/data/flags" }),
	schema: z.object({
		name: z.string(),
		description: z.string(),
		defaultVal: z.string().optional(),
		type: z.string(),
		env: z.array(z.string()).optional(),
		aliases: z.array(z.string()).optional(),
		since: releaseVersion.optional(),
	}),
});

// The configuration files an HCL block, attribute, or function is valid in:
// `unit` for `terragrunt.hcl` and the files it includes, `stack` for
// `terragrunt.stack.hcl`.
const hclConfigs = z.array(z.enum(["unit", "stack"])).nonempty();

// `code` is HCL, and a leading `# terragrunt.hcl` comment becomes the code
// block's title. `description` is Markdown. It appears above the code, so it
// states how the item behaves and doesn't name values from `code`.
const hclExamples = z.array(z.object({
	description: z.string().optional(),
	code: z.string(),
})).nonempty();

// Shared by every HCL reference entry. `name` is the identifier as written in
// HCL and names the page at `/reference/hcl/<kind>/<name>/`. `description` is
// one plain-text sentence, used as the page's meta description and in index
// tables.
const hclItem = {
	name: z.string().regex(/^[a-z][a-z0-9_]*$/),
	description: z.string(),
	configs: hclConfigs,
	since: releaseVersion.optional(),
	// The experiment that gates the item, e.g. `deep-merge`. While it's active,
	// index tables and the sidebar list the item after the rest.
	experiment: z.string().optional(),
	examples: hclExamples,
};

// An attribute set inside a block. `type` uses HCL type syntax, e.g.
// `list(string)`. `description` is Markdown. An attribute with `since` is left
// out of its block until that release.
const hclAttributeFields = {
	name: z.string(),
	type: z.string(),
	required: z.boolean().default(false),
	default: z.string().optional(),
	description: z.string(),
	since: releaseVersion.optional(),
};

const requiredHasNoDefault = [
	(attribute: { required: boolean; default?: string }) => !attribute.required || attribute.default === undefined,
	{ message: "A required attribute can't have a default" },
] as const;

// Required attributes come first, so the first example on the page is the
// simplest valid block.
function requiredFirst<T extends z.ZodType<{ required: boolean }>>(attribute: T) {
	return z.array(attribute).refine(
		(attributes) => {
			const requiredCount = attributes.filter((attr) => attr.required).length;
			return attributes.slice(0, requiredCount).every((attr) => attr.required);
		},
		{ message: 'Required attributes must come before optional ones' },
	);
}

// A key of a map attribute, e.g. `bucket` in `remote_state`'s `config`.
// `provisioning` marks a key Terragrunt uses only to provision the backend,
// and doesn't pass on to OpenTofu/Terraform. `keys` lists the keys of a key
// that is itself a map.
const hclKeyFields = {
	...hclAttributeFields,
	provisioning: z.boolean().default(false),
	deprecated: z.boolean().default(false),
};

const hclMapKey = z
	.object({
		...hclKeyFields,
		keys: requiredFirst(z.object(hclKeyFields).refine(...requiredHasNoDefault)).optional(),
	})
	.refine(...requiredHasNoDefault);

// The keys a map attribute accepts, in groups chosen by another attribute of
// the block, `selector`, e.g. one group per `remote_state` `backend`. Each
// group's `name` is the `selector` value that chooses it, and the page anchor
// of its card. `description` and `note` are Markdown, before and after the keys.
const hclKeyGroups = z.object({
	slug: z.string().regex(/^[a-z][a-z0-9-]*$/),
	title: z.string(),
	selector: z.string(),
	description: z.string(),
	groups: z.array(
		z.object({
			name: z.string().regex(/^[a-z][a-z0-9_]*$/),
			title: z.string(),
			description: z.string().optional(),
			keys: requiredFirst(hclMapKey),
			note: z.string().optional(),
		}),
	),
});

// `example` is HCL showing the attribute set inside its block.
const hclBlockAttribute = z
	.object({ ...hclAttributeFields, example: z.string(), keyGroups: hclKeyGroups.optional() })
	.refine(...requiredHasNoDefault);

// In the order they follow the block name. `description` is Markdown.
const hclBlockLabels = z.array(z.object({ name: z.string(), description: z.string() }));

// A block that only appears inside another block, e.g. `before_hook` inside
// `terraform`. Its attributes share one `example`, HCL showing the block inside
// its parent. `description` is Markdown.
const hclNestedBlock = z.object({
	name: z.string().regex(/^[a-z][a-z0-9_]*$/),
	description: z.string(),
	labels: hclBlockLabels.optional(),
	attributes: requiredFirst(z.object(hclAttributeFields).refine(...requiredHasNoDefault)),
	example: z.string(),
});

const hclBlocks = defineCollection({
	loader: glob({ pattern: "**/*.mdx", base: "src/data/hcl/blocks" }),
	schema: z
		.object({
			...hclItem,
			// Each attribute has its own example, so a block only needs these to
			// show something the attribute examples can't.
			examples: hclExamples.optional(),
			labels: hclBlockLabels.optional(),
			attributes: requiredFirst(hclBlockAttribute).optional(),
			blocks: z.array(hclNestedBlock).optional(),
		})
		.refine((block) => block.examples !== undefined || (block.attributes ?? []).length > 0, {
			message: 'A block needs attributes or examples',
		}),
});

const hclAttributes = defineCollection({
	loader: glob({ pattern: "**/*.mdx", base: "src/data/hcl/attributes" }),
	schema: z.object({
		...hclItem,
		type: z.string(),
		default: z.string().optional(),
		// An Aside after the examples, which the body can't reach. `body` is Markdown.
		note: z.object({ title: z.string(), body: z.string() }).optional(),
	}),
});

const hclFunctions = defineCollection({
	loader: glob({ pattern: "**/*.mdx", base: "src/data/hcl/functions" }),
	schema: z.object({
		...hclItem,
		// In call order. `description` is Markdown.
		params: z.array(z.object({
			name: z.string(),
			type: z.string(),
			optional: z.boolean().default(false),
			variadic: z.boolean().optional(),
			description: z.string(),
		})),
		returns: z.string(),
	}),
});

const faq = defineCollection({
	loader: glob({ pattern: "**/*.{md,mdx}", base: "src/data/faq" }),
	schema: z.object({
		// The question, shown as the row title on the index and as the page
		// heading on the question's own page.
		question: z.string(),
		// A short, ~1-2 line answer shown (truncated to two lines) in the
		// "Answer" column of the index table.
		description: z.string(),
		// Optional: controls ordering on the index. Lower numbers sort first;
		// entries without an order fall back to alphabetical by question.
		order: z.number().optional(),
	}),
});

const patterns = defineCollection({
	loader: glob({ pattern: "**/*.{md,mdx}", base: "src/data/patterns" }),
	schema: z.object({
		// The pattern's title, shown on its card and as the page heading.
		title: z.string(),
		// A short description shown on the card beneath the title.
		description: z.string(),
		// The author's name, shown on the card and the pattern's page.
		author: z.string(),
		// Optional: controls ordering on the index. Lower numbers sort first;
		// entries without an order fall back to alphabetical by title.
		order: z.number().optional(),
	}),
});

const changelog = defineCollection({
	loader: glob({ pattern: "**/*.{md,mdx}", base: "src/data/changelog" }),
	schema: z.object({
		version: releaseVersion,
		category: z.enum(CHANGELOG_CATEGORY_SLUGS),
		order: z.number().optional(),
	}),
});

const compatibility = defineCollection({
	loader: file("src/data/compatibility/compatibility.json"),
	schema: z.object({
		id: z.string(),
		tool: z.enum(["opentofu", "terraform"]),
		version: z.string(),
		terragrunt_min: compatibilityVersion,
		terragrunt_max: compatibilityVersion.nullable(),
		order: z.number(),
	}),
});

const experiments = defineCollection({
	loader: glob({ pattern: "**/*.mdx", base: "src/data/experiments" }),
	schema: z.object({
		name: z.string(),
		// `since` is the release an experiment became available for opt-in.
		// `completedSince` is the release in which the experiment concluded,
		// whether it graduated to a default feature or was retired; once that
		// release ships, the experiment is treated as completed. An
		// experiment's active/completed status is derived from these versions
		// rather than a separate `status` field.
		since: releaseVersion.optional(),
		completedSince: releaseVersion.optional(),
	}),
});

const strictControls = defineCollection({
	loader: glob({ pattern: "**/*.mdx", base: "src/data/strict-controls" }),
	schema: z.object({
		name: z.string(),
		// `since` is the release a strict control became available.
		// `completedSince` is the release in which the behavior it enforces
		// became the default; once that release ships, the control is treated
		// as completed.
		since: releaseVersion.optional(),
		completedSince: releaseVersion.optional(),
	}),
});

export const collections = {
	changelog,
	commands,
	compatibility,
	docs,
	experiments,
	faq,
	flags,
	hclAttributes,
	hclBlocks,
	hclFunctions,
	patterns,
	strictControls,
};
