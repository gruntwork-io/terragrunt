import { defineCollection, z } from 'astro:content';
import { docsLoader } from '@astrojs/starlight/loaders';
import { docsSchema } from '@astrojs/starlight/schema';
import { glob, file } from 'astro/loaders';

const brands = defineCollection({
	loader: file("src/data/brands/brands.json"),
	schema: ({ image }) => z.object({
		id: z.string(),
		name: z.string(),
		logo: image(),
		alt: z.string(),
		order: z.number().optional(),
	}),
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

const docs = defineCollection({
	loader: docsLoader(),
	schema: docsSchema(),
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

const testimonials = defineCollection({
	loader: file("src/data/testimonials/testimonials.json"),
	schema: ({ image }) => z.object({
		id: z.string(),
		order: z.number().optional(),
		author: z.string(),
    title: z.string().optional(),
    company: z.string().optional(),
		logo: image().optional(),
		alt: z.string().optional(),
    content: z.string(),
    link: z.string().optional(),
	}),
});

function compatibilityLoader() {
	return {
		name: "compatibility-loader",
		load: async ({ store, parseData }: { store: any; parseData: any }) => {
			const content = await import("./data/compatibility/compatibility.json");
			const items = content.default as Array<{
				tool: string;
				version: string;
				terragrunt_min: string;
				terragrunt_max: string | null;
				order: number;
			}>;
			store.clear();
			for (const item of items) {
				const id = `${item.tool}-${item.version}`;
				const data = await parseData({ id, data: item });
				store.set({ id, data });
			}
		},
	};
}

const compatibility = defineCollection({
	loader: compatibilityLoader(),
	schema: z.object({
		tool: z.enum(["opentofu", "terraform"]),
		version: z.string(),
		terragrunt_min: z.string(),
		terragrunt_max: z.string().nullable(),
		order: z.number(),
	}),
});

export const collections = { brands, commands, compatibility, docs, flags, testimonials };
