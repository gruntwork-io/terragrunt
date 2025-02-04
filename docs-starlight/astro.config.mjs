// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// https://astro.build/config
export default defineConfig({
	integrations: [
		starlight({
			title: 'Terragrunt',
			logo: {
				src: '/src/assets/logo.svg',
			},
			social: {
				github: 'https://github.com/gruntwork-io/terragrunt',
				discord: 'https://discord.gg/SPu4Degs5f',
			},
			sidebar: [
				{
					label: 'Getting Started',
					autogenerate: { directory: '01-getting-started' },
				},
				{
					label: 'Features',
					autogenerate: { directory: '02-features', collapsed: true },
				},
				{
					label: 'Community',
					autogenerate: { directory: '03-community', collapsed: true },
				},
				{
					label: 'Reference',
					items: [
						{
							label: 'Configuration', slug: 'docs/reference/configuration',
						},
						{
							label: 'CLI', collapsed: true, items: [
								{ label: 'Commands', autogenerate: { directory: '04-reference/cli/commands', collapsed: true } },
							],
						},
					],
				},
				{
					label: 'Troubleshooting',
					autogenerate: { directory: '05-troubleshooting', collapsed: true },
				},
				{
					label: 'Migrate',
					autogenerate: { directory: '06-migrate', collapsed: true },
				},
			],
		}),
	],
});
