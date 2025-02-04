import { defineCollection, z } from 'astro:content';
import { docsLoader } from '@astrojs/starlight/loaders';
import { docsSchema } from '@astrojs/starlight/schema';
import { glob } from 'astro/loaders';

const docs = defineCollection({ loader: docsLoader(), schema: docsSchema() });
const commands = defineCollection({
	loader: glob({ pattern: "**/*.yml", base: "src/data/commands" }),
	schema: z.object({
		name: z.string(),
		usage: z.string(),
		description: z.string(),
	}),
});

export const collections = { docs, commands };
