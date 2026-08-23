import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

const config: Config = {
  title: 'hiveD',
  tagline: 'A control plane for defining, scheduling, governing and observing AI agents',
  favicon: 'img/favicon.png',

  // GitHub Pages project site. This works with no DNS setup.
  //
  // To move to a custom domain later: set url to the domain, baseUrl to '/',
  // add a CNAME file to website/static/, and point the DNS at GitHub Pages.
  // Nothing else changes.
  url: 'https://vibed-project.github.io',
  baseUrl: '/hiveD/',

  organizationName: 'vibed-project',
  projectName: 'hiveD',

  // A broken internal link should fail the build. This site publishes
  // automatically from main, so a warning would go unread.
  onBrokenLinks: 'throw',
  markdown: {
    hooks: {
      onBrokenMarkdownLinks: 'warn',
    },
  },

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
          routeBasePath: '/',
          editUrl: 'https://github.com/vibed-project/hiveD/tree/main/website/',
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],

  plugins: [
    [
      '@docusaurus/plugin-content-docs',
      {
        // The ADRs stay in docs/adr/ as the single source of truth. Pointing
        // the plugin at them keeps the site in sync with no copying, so an
        // ADR added in a normal PR shows up here automatically.
        id: 'adr',
        path: '../docs/adr',
        routeBasePath: 'adr',
        sidebarPath: './sidebarsAdr.ts',
        editUrl: 'https://github.com/vibed-project/hiveD/tree/main/',
      },
    ],
  ],

  themeConfig: {
    image: 'img/social-card.png',
    colorMode: {
      defaultMode: 'light',
      respectPrefersColorScheme: true,
    },
    navbar: {
      title: 'hiveD',
      logo: {
        alt: 'hiveD logo',
        src: 'img/hived-logo.png',
      },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'main',
          position: 'left',
          label: 'Docs',
        },
        {
          href: 'https://github.com/vibed-project/hiveD',
          label: 'GitHub',
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
            {label: 'Overview', to: '/'},
            {label: 'Quickstart', to: '/quickstart'},
            {label: 'Architecture', to: '/architecture'},
            {label: 'Roadmap', to: '/roadmap'},
          ],
        },
        {
          title: 'Reference',
          items: [
            {label: 'Resources', to: '/resources'},
            {label: 'CLI', to: '/cli'},
            {label: 'Decision records', to: '/adr/'},
          ],
        },
        {
          title: 'The stack',
          items: [
            {label: 'mindD (memory)', href: 'https://vibed-project.github.io/mindD/'},
            {label: 'routeD (model routing)', href: 'https://vibed-project.github.io/routeD/'},
            {label: 'vibeD (sandbox)', href: 'https://vibed.run/'},
            {label: 'GitHub', href: 'https://github.com/vibed-project/hiveD'},
          ],
        },
      ],
      copyright: 'Apache 2.0 · hiveD contributors',
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ['bash', 'yaml', 'protobuf', 'go', 'json', 'sql'],
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
