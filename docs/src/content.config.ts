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
		status: z.enum(["active", "completed"]),
		since: releaseVersion.optional(),
	}),
});

export const collections = { changelog, commands, compatibility, docs, experiments, faq, flags, patterns, strictControls };
