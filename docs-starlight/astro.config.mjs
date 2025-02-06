// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import starlightLinksValidator from 'starlight-links-validator';

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
						{ label: 'Configuration', autogenerate: { directory: '04-reference/01-configuration', collapsed: true } },
						{
							label: 'CLI', collapsed: true, items: [
								{ label: 'Commands', autogenerate: { directory: '04-reference/02-cli/commands', collapsed: true } },
							],
						},
						{ label: 'Strict Controls', slug: 'docs/reference/strict-controls' },
						{ label: 'Experiments', slug: 'docs/reference/experiments' },
						{ label: 'Supported Versions', slug: 'docs/reference/supported-versions' },
						{ label: 'Lock File Handling', slug: 'docs/reference/lock-file-handling' },
						{ label: 'Terragrunt Cache', slug: 'docs/reference/terragrunt-cache' },
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
			// NOTE: We don't currently check links by default because the CLI
			// Redesign isn't done yet. Once those pages are built out, we'll require
			// links to be checked for all builds.
			plugins: process.env.CHECK_LINKS ? [starlightLinksValidator()] : [],
		}),
	],
});
