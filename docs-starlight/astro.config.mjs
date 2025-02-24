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
					{ label: 'Global Flags', slug: 'docs/reference/cli/global-flags' },
					{ label: 'Rules', slug: 'docs/reference/cli/rules' },
				],
			},
			{ label: 'Strict Controls', slug: 'docs/reference/strict-controls' },
			{ label: 'Experiments', slug: 'docs/reference/experiments' },
			{ label: 'Supported Versions', slug: 'docs/reference/supported-versions' },
			{ label: 'Lock Files', slug: 'docs/reference/lock-files' },
			{ label: 'Logging', autogenerate: { directory: '04-reference/07-logging', collapsed: true } },
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
	site: 'https://terragrunt-v1.gruntwork.io',
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

					// Unfortunately, these have to be ignored, as they're
					// referencing links that exist outside the file.
					'/docs/reference/cli/commands/run#*',
				],
			})],
		}),
	],
	redirects: {
		// Pages that have been rehomed.
		"/docs/features/debugging/": "/docs/troubleshooting/debugging/",
		"/docs/upgrade/upgrading_to_terragrunt_0.19.x/": "/docs/migrate/upgrading_to_terragrunt_0.19.x/",

		// Redirects to external sites.
		"/contact/": "https://gruntwork.io/contact",
		"/commercial-support/": "https://gruntwork.io/support",
		"/cookie-policy/": "https://gruntwork.io/legal/cookie-policy/",

		// Restructured docs
		"/docs/reference/configuration/": "/docs/reference/hcl/",
		"/docs/reference/cli-options/": "/docs/reference/cli/",
		"/docs/reference/built-in-functions/": "/docs/reference/hcl/functions/",
		"/docs/reference/config-blocks-and-attributes/": "/docs/reference/hcl/blocks/",
		"/docs/reference/strict-mode/": "/docs/reference/strict-controls/",
		"/docs/reference/log-formatting/": "/docs/reference/logging/formatting/",
		"/docs/features/aws-authentication/": "/docs/features/authentication/",

		// Support old doc structure paths
		"/docs/": "/docs/getting-started/quick-start/",
		"/docs/getting-started/": "/docs/getting-started/quick-start/",
		"/docs/features/": "/docs/features/units/",
		"/docs/reference/": "/docs/reference/hcl/",
		"/docs/troubleshooting/": "/docs/troubleshooting/debugging/",
		"/docs/migrate/": "/docs/migrate/migrating-from-root-terragrunt-hcl/",

		// Support old community paths
		"/docs/community/": "/docs/community/contributing/",
		"/support/": "/docs/community/support/",

		// Support old feature paths
		"/docs/features/inputs/": "/docs/features/units/",
		"/docs/features/locals/": "/docs/features/units/",
		"/docs/features/keep-your-terraform-code-dry/": "/docs/features/units/",
		"/docs/features/execute-terraform-commands-on-multiple-units-at-once/": "/docs/features/stacks/",
		"/docs/features/keep-your-terragrunt-architecture-dry/": "/docs/features/includes/",
		"/docs/features/keep-your-remote-state-configuration-dry/": "/docs/features/state-backend/",
		"/docs/features/keep-your-cli-flags-dry/": "/docs/features/extra-arguments/",
		"/docs/features/aws-auth/": "/docs/features/aws-authentication/",
		"/docs/features/work-with-multiple-aws-accounts/": "/docs/features/aws-authentication/",
		"/docs/features/auto-retry/": "/docs/features/runtime-control/",
		"/docs/features/provider-cache/": "/docs/features/provider-cache-server/",
		"/docs/features/provider-caching/": "/docs/features/provider-cache-server/",
	},
});
