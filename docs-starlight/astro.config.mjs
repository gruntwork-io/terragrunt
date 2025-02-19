// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import starlightLinksValidator from 'starlight-links-validator';
import vercel from '@astrojs/vercel';

export const sidebar = [
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
			{ label: 'HCL', autogenerate: { directory: '04-reference/01-hcl', collapsed: true } },
			{
				label: 'CLI', collapsed: true, items: [
					{ label: 'Overview', slug: 'docs/reference/cli' },
					{ label: 'Commands', autogenerate: { directory: '04-reference/02-cli/02-commands', collapsed: true } },
					{ label: 'Rules', slug: 'docs/reference/cli/rules' },
				],
			},
			{ label: 'Strict Controls', slug: 'docs/reference/strict-controls' },
			{ label: 'Experiments', slug: 'docs/reference/experiments' },
			{ label: 'Supported Versions', slug: 'docs/reference/supported-versions' },
			{ label: 'Lock Files', slug: 'docs/reference/lock-files' },
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
];

// https://astro.build/config
export default defineConfig({
	output: 'server',
	adapter: vercel({
		isr: {
			expiration: 60 * 60 * 24, // 24 hours
		},
	}),
	integrations: [
		starlight({
			title: 'Terragrunt',
			logo: {
				dark: '/src/assets/logo-light.svg',
				light: '/src/assets/logo-dark.svg',
			},
			social: {
				github: 'https://github.com/gruntwork-io/terragrunt',
				discord: 'https://discord.gg/SPu4Degs5f',
			},
			sidebar: sidebar,
			// NOTE: We don't currently check links by default because the CLI
			// Redesign isn't done yet. Once those pages are built out, we'll require
			// links to be checked for all builds.
			plugins: [starlightLinksValidator({
				exclude: [
					// Used in the docs for OpenTelemetry
					'http://localhost:16686/',
					'http://localhost:9090/',

					// TODO: Remove these once the CLI redesign is done
					'/docs/reference/cli**/*',
					'/docs/reference/cli*',
				],
			})],
		}),
	],
});
