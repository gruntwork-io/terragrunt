import { themes as prismThemes } from 'prism-react-renderer';
import type { Config } from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

const config: Config = {
  title: 'Terragrunt',
  tagline: 'Flexible orchestration to scale IaC',
  favicon: 'img/favicon.ico',

  // Set the production url of your site here
  url: 'https://terragrunt.gruntwork.io',
  // Set the /<baseUrl>/ pathname under which your site is served
  // For GitHub pages deployment, it is often '/<projectName>/'
  baseUrl: '/',

  // GitHub pages deployment config.
  // If you aren't using GitHub pages, you don't need these.
  organizationName: 'gruntwork-io', // Usually your GitHub org/user name.
  projectName: 'terragrunt', // Usually your repo name.

  onBrokenLinks: 'throw',
  onBrokenMarkdownLinks: 'warn',

  // Even if you don't use internationalization, you can use this field to set
  // useful metadata like html lang. For example, if your site is Chinese, you
  // may want to replace "en" with "zh-Hans".
  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      {
        docs: {
          sidebarPath: './sidebars.ts',
          // Please change this to your repo.
          // Remove this to remove the "edit this page" links.
          editUrl:
            'https://github.com/gruntwork-io/terragrunt/tree/main/',
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    // Replace with your project's social card
    image: 'img/docusaurus-social-card.jpg', // FIXME: Figure out what to do with this.
    navbar: {
      title: 'Terragrunt',
      logo: {
        alt: 'Terragrunt Logo',
        src: 'img/logo.svg',
      },
      items: [ // FIXME: Resolve this.
        {
          type: 'docSidebar',
          sidebarId: 'tutorialSidebar',
          position: 'left',
          label: 'Tutorial',
        },
        { // FIXME: Make these images.
          href: 'https://github.com/gruntwork-io/terragrunt',
          label: 'GitHub',
          position: 'right',
          className: "header--github-link",
          "aria-label": "GitHub repository",
        },
        {
          href: 'https://discord.gg/SPu4Degs5f',
          label: 'Discord',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Docs',
          items: [
            {
              label: 'Tutorial',
              to: '/docs/intro',
            },
          ],
        },
        {
          title: 'Community',
          items: [
            {
              label: 'Discord',
              href: 'https://discord.gg/SPu4Degs5f',
            },
            {
              label: 'GitHub Discussions',
              href: 'https://github.com/gruntwork-io/terragrunt/discussions',
            },
          ],
        },
        {
          title: 'More',
          items: [
            {
              label: 'GitHub',
              href: 'https://github.com/gruntwork-io/terragrunt',
            },
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} Gruntwork Inc. Built with Docusaurus.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
