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
	loader: glob({ pattern: "**/*.yml", base: "src/data/commands" }),
	schema: z.object({
		name: z.string(),
		usage: z.string(),
		description: z.string(),
	}),
});

export const collections = { docs, commands };
