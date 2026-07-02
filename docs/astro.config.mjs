// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import starlightThemeGalaxy from 'starlight-theme-galaxy';
import d2 from 'astro-d2';

// Archived doc versions are built at a subpath (e.g. /v1-5/) via BASE_PATH.
// The current version is always built at the site root ("/").
const basePath = process.env.BASE_PATH || '/';

// https://astro.build/config
export default defineConfig({
  site: 'https://promptarena.altairalabs.ai',
  base: basePath,
  integrations: [
    // astro-d2 0.8+ requires an explicit `output` — the Zod default is no
    // longer applied when no userConfig is passed, and the build crashes on
    // path.join(undefined) otherwise.
    d2({ output: 'd2' }),
    starlight({
      title: 'PromptArena',
      logo: {
        src: './public/logo.svg',
        alt: 'PromptArena Logo',
      },
      plugins: [starlightThemeGalaxy()],
      customCss: ['./src/styles/custom.css'],
      social: [
        { icon: 'github', label: 'GitHub', href: 'https://github.com/AltairaLabs/promptarena' },
      ],
      // Version switcher (header) + "not latest" banner (page title).
      components: {
        Header: './src/components/Header.astro',
        PageTitle: './src/components/PageTitle.astro',
      },
      sidebar: [
        // Starlight v0.39+ requires `autogenerate` to live inside an `items`
        // array on a labeled group, not as a sibling of `label`/`collapsed`.
        {
          label: 'Arena',
          collapsed: false,
          items: [
            { label: 'Tutorials', collapsed: true, items: [{ autogenerate: { directory: 'arena/tutorials' } }] },
            { label: 'How-To Guides', collapsed: true, items: [{ autogenerate: { directory: 'arena/how-to' } }] },
            { label: 'Reference', collapsed: true, items: [{ autogenerate: { directory: 'arena/reference' } }] },
            { label: 'Explanation', collapsed: true, items: [{ autogenerate: { directory: 'arena/explanation' } }] },
          ],
        },
        {
          label: 'PackC',
          collapsed: true,
          items: [
            { label: 'Tutorials', collapsed: true, items: [{ autogenerate: { directory: 'packc/tutorials' } }] },
            { label: 'How-To Guides', collapsed: true, items: [{ autogenerate: { directory: 'packc/how-to' } }] },
            { label: 'Reference', collapsed: true, items: [{ autogenerate: { directory: 'packc/reference' } }] },
            { label: 'Explanation', collapsed: true, items: [{ autogenerate: { directory: 'packc/explanation' } }] },
          ],
        },
      ],
      head: [
        {
          tag: 'script',
          attrs: {
            type: 'module',
            src: '/mermaid-init.js',
          },
        },
      ],
    }),
  ],
});
