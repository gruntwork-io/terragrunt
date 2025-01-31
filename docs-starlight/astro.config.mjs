// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// https://astro.build/config
export default defineConfig({
	integrations: [
		starlight({
			title: 'Terragrunt',
			social: { github: 'https://github.com/gruntwork-io/terragrunt' },
			sidebar: [
				{
					label: 'Getting Started',
					items: [
						{ label: 'Quick Start', slug: 'getting-started/quick-start' },
						{ label: 'Installation', slug: 'getting-started/install' },
					],
				},
				{
					label: 'Features',
					items: [
						{ label: 'Units', slug: 'features/units' },
						{ label: 'Catalog', slug: 'features/catalog' },
					],
				},
				{
					label: 'Community',
					items: [
						{ label: 'Contributing', slug: 'community/contributing' },
						{ label: 'Support', slug: 'community/support' },
					],
				},
				{
					label: 'Reference',
					items: [
						{
							label: 'Configuration', slug: 'reference/configuration',
						},
						{
							label: 'CLI', items: [
								{ label: 'Commands', autogenerate: { directory: 'reference/cli/commands' } },
							],
						},
					],
				},
				{
					label: 'Troubleshooting',
					items: [
						{ label: 'Debugging', slug: 'troubleshooting/debugging' },
						{ label: 'OpenTelemetry', slug: 'troubleshooting/open-telemetry' },
					],
				},
				{
					label: 'Migrate',
					items: [
						{ label: 'Migrating from root terragrunt.hcl', slug: 'migration-guides/migrating-from-root-terragrunt-hcl' },
						{ label: 'Upgrading to Terragrunt 0.19.x', slug: 'migration-guides/upgrading-to-terragrunt-0-19-x' },
					],
				},
			],
		}),
	],
});
