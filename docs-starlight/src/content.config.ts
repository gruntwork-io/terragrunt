import { defineCollection, z } from 'astro:content';
import { docsLoader } from '@astrojs/starlight/loaders';
import { docsSchema } from '@astrojs/starlight/schema';
import { glob } from 'astro/loaders';

const docs = defineCollection({
	loader: docsLoader(), schema: docsSchema(
		{
			extend: z.object({
				banner: z.object({ content: z.string() }).default({
					content: "👷 The Terragrunt website redesign is Work In Progress! 👷</br>For a list of outstanding TODOs see <a href=\"https://github.com/gruntwork-io/terragrunt/blob/main/docs-starlight/TODO.md\">this</a>.</br>To give feedback, click <a href=\"https://forms.gle/MxfBQ5DebeAHA6oN6\">here</a>.",
				}),
			}),
		},
	)
});

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

const flags = defineCollection({
	loader: glob({ pattern: "**/*.mdx", base: "src/data/flags" }),
	schema: z.object({
		name: z.string(),
		description: z.string(),
		type: z.string(),
		env: z.array(z.string()).optional(),
		aliases: z.array(z.string()).optional(),
	}),
});

export const collections = { docs, commands, flags };
