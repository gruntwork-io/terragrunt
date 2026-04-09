import { defineCollection, z } from 'astro:content';
import { docsLoader } from '@astrojs/starlight/loaders';
import { docsSchema } from '@astrojs/starlight/schema';
import { glob, file } from 'astro/loaders';

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
		})),
		flags: z.array(z.string()).optional(),
		experiment: z.object({
			control: z.string(),
			name: z.string(),
		}).optional(),
	}),
});

const docs = defineCollection({
	loader: docsLoader(),
	schema: docsSchema({
		extend: z.object({
			banner: z.object({ content: z.string() }).default({
				content: '🎉 <strong>Terragrunt v1.0 is here!</strong> Read the <a href="https://www.gruntwork.io/blog/terragrunt-1-0-released">announcement</a> to learn more.',
			}),
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
	}),
});

const changelog = defineCollection({
	loader: glob({ pattern: "**/*.mdx", base: "src/data/changelog" }),
	schema: z.object({
		version: z.string(),
	}),
});

const compatibility = defineCollection({
	loader: file("src/data/compatibility/compatibility.json"),
	schema: z.object({
		id: z.string(),
		tool: z.enum(["opentofu", "terraform"]),
		version: z.string(),
		terragrunt_min: z.string(),
		terragrunt_max: z.string().nullable(),
		order: z.number(),
	}),
});

const experiments = defineCollection({
	loader: glob({ pattern: "**/*.mdx", base: "src/data/experiments" }),
	schema: z.object({
		name: z.string(),
		status: z.enum(["active", "completed"]),
		since: z.string().optional(),
	}),
});

const strictControls = defineCollection({
	loader: glob({ pattern: "**/*.mdx", base: "src/data/strict-controls" }),
	schema: z.object({
		name: z.string(),
		status: z.enum(["active", "completed"]),
		since: z.string().optional(),
	}),
});

export const collections = { changelog, commands, compatibility, docs, experiments, flags, strictControls };
