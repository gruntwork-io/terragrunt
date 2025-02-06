// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// https://astro.build/config
export default defineConfig({
	integrations: [
		starlight({
			title: 'Terragrunt',
			social: {
				github: 'https://github.com/gruntwork-io/terragrunt',
				discord: 'https://discord.gg/SPu4Degs5f',
			},
			sidebar: [
				{
					label: 'Getting Started',
					autogenerate: { directory: 'getting-started' },
				},
				{
					label: 'Features',
					autogenerate: { directory: 'features' },
				},
				{
					label: 'Community',
					autogenerate: { directory: 'community' },
				},
				{
					label: 'Reference',
					items: [
						{
							label: 'Configuration', slug: 'docs/reference/configuration',
						},
						{
							label: 'CLI', collapsed: true, items: [
								{ label: 'Commands', autogenerate: { directory: 'reference/cli/commands', collapsed: true } },
							],
						},
					],
				},
				{
					label: 'Troubleshooting',
					autogenerate: { directory: 'troubleshooting' },
				},
				{
					label: 'Migrate',
					autogenerate: { directory: 'migrate' },
				},
			],
		}),
	],
	redirects: {
		'/getting-started': '/quick-start',
	},
});
