// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import d2 from 'astro-d2';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

// Archived doc versions are built at a subpath (e.g. /v1-5/) via BASE_PATH.
// The current version is always built at the site root ("/").
const basePath = process.env.BASE_PATH || '/';

// Resolve the content dir so generated/fetched sections (arena/examples) can be
// wired only when present — a skipped/offline build shouldn't fail on a missing
// autogenerate directory.
const CONTENT = fileURLToPath(new URL('./src/content/docs/', import.meta.url));
const has = (dir) => existsSync(CONTENT + dir);
const auto = (dir) => ({ autogenerate: { directory: dir } });
const grp = (label, dir) => ({ label, items: [auto(dir)] });

// Redirects for How-To pages that moved into themed subdirectories (see
// docs/local-backlog/2026-07-05-arena-docs-taxonomy-design.md). The two CI
// pages were consolidated into interfaces/run-in-ci.
const howToRedirects = {
  '/arena/how-to/installation/': '/arena/how-to/setup/installation/',
  '/arena/how-to/shell-completions/': '/arena/how-to/setup/shell-completions/',
  '/arena/how-to/use-project-templates/': '/arena/how-to/setup/use-project-templates/',
  '/arena/how-to/develop-templates/': '/arena/how-to/setup/develop-templates/',
  '/arena/how-to/install-skills/': '/arena/how-to/setup/install-skills/',
  '/arena/how-to/use-as-go-library/': '/arena/how-to/setup/use-as-go-library/',
  '/arena/how-to/write-scenarios/': '/arena/how-to/scenarios/write-scenarios/',
  '/arena/how-to/manage-context/': '/arena/how-to/scenarios/manage-context/',
  '/arena/how-to/validate-outputs/': '/arena/how-to/scenarios/validate-outputs/',
  '/arena/how-to/guardrails-as-signals/': '/arena/how-to/scenarios/guardrails-as-signals/',
  '/arena/how-to/generate-mock-responses-from-arena/': '/arena/how-to/scenarios/generate-mock-responses-from-arena/',
  '/arena/how-to/session-recording/': '/arena/how-to/scenarios/session-recording/',
  '/arena/how-to/workflow-regression/': '/arena/how-to/scenarios/workflow-regression/',
  '/arena/how-to/configure-providers/': '/arena/how-to/providers/configure-providers/',
  '/arena/how-to/use-mock-providers/': '/arena/how-to/providers/use-mock-providers/',
  '/arena/how-to/model-migration/': '/arena/how-to/providers/model-migration/',
  '/arena/how-to/build-with-an-ai-agent/': '/arena/how-to/agents/build-with-an-ai-agent/',
  '/arena/how-to/a2a-multi-agent/': '/arena/how-to/agents/a2a-multi-agent/',
  '/arena/how-to/test-a2a-agents/': '/arena/how-to/agents/test-a2a-agents/',
  '/arena/how-to/test-mcp-tools/': '/arena/how-to/agents/test-mcp-tools/',
  '/arena/how-to/provision-mcp-sandbox/': '/arena/how-to/agents/provision-mcp-sandbox/',
  '/arena/how-to/rag-agent/': '/arena/how-to/agents/rag-agent/',
  '/arena/how-to/text-negotiation/': '/arena/how-to/agents/text-negotiation/',
  '/arena/how-to/setup-voice-testing/': '/arena/how-to/voice/setup-voice-testing/',
  '/arena/how-to/voice-bake-off/': '/arena/how-to/voice/voice-bake-off/',
  '/arena/how-to/voice-characterization/': '/arena/how-to/voice/voice-characterization/',
  '/arena/how-to/voice-console/': '/arena/how-to/voice/voice-console/',
  '/arena/how-to/voice-customer-support/': '/arena/how-to/voice/voice-customer-support/',
  '/arena/how-to/voice-guardrails/': '/arena/how-to/voice/voice-guardrails/',
  '/arena/how-to/voice-ivr/': '/arena/how-to/voice/voice-ivr/',
  '/arena/how-to/voice-latency-budget/': '/arena/how-to/voice/voice-latency-budget/',
  '/arena/how-to/voice-red-team/': '/arena/how-to/voice/voice-red-team/',
  '/arena/how-to/voice-tool-calls/': '/arena/how-to/voice/voice-tool-calls/',
  // Consolidated CI pages
  '/arena/how-to/arena-ci-quality-gate/': '/arena/how-to/interfaces/run-in-ci/',
  '/arena/how-to/integrate-ci-cd/': '/arena/how-to/interfaces/run-in-ci/',
};

// https://astro.build/config
export default defineConfig({
  site: 'https://promptarena.altairalabs.ai',
  base: basePath,
  redirects: howToRedirects,
  integrations: [
    // astro-d2 0.8+ requires an explicit `output` — the Zod default is no
    // longer applied when no userConfig is passed, and the build crashes on
    // path.join(undefined) otherwise.
    d2({ output: 'd2' }),
    starlight({
      title: 'PromptArena',
      logo: {
        src: './public/atlas/logo-promptarena.svg',
        alt: 'PromptArena',
      },
      customCss: ['./src/styles/custom.css'],
      // Atlas-themed code blocks: a distinct ink-void surface, mono code font,
      // hairline frame with a soft shadow, and a cyan active-tab indicator —
      // so docs code blocks read as first-class, not the plain default.
      expressiveCode: {
        // Vibrant, legible syntax themes (dark = tokyo-night's blue/violet/cyan
        // fits the night-sky brand; light = github-light). Starlight swaps them
        // with data-theme.
        themes: ['tokyo-night', 'github-light'],
        styleOverrides: {
          borderColor: 'var(--hairline)',
          borderRadius: 'var(--radius-code)',
          codeBackground: 'var(--ink-void)',
          codeFontFamily: 'var(--font-mono)',
          codeFontSize: '13.5px',
          uiFontFamily: 'var(--font-sans)',
          frames: {
            editorActiveTabBackground: 'var(--ink-void)',
            editorTabBarBackground: 'var(--ink-surface)',
            editorActiveTabIndicatorBottomColor: 'var(--ion-cyan)',
            editorTabBarBorderBottomColor: 'var(--hairline)',
            terminalBackground: 'var(--ink-void)',
            terminalTitlebarBackground: 'var(--ink-surface)',
            terminalTitlebarBorderBottomColor: 'var(--hairline)',
            frameBoxShadowCssValue: '0 12px 32px -18px rgba(0, 0, 0, 0.6)',
          },
        },
      },
      social: [
        { icon: 'github', label: 'GitHub', href: 'https://github.com/AltairaLabs/promptarena' },
      ],
      // Version switcher (header) + "not latest" banner (page title).
      components: {
        Header: './src/components/Header.astro',
        SocialIcons: './src/components/SocialIcons.astro',
        PageTitle: './src/components/PageTitle.astro',
      },
      // --- Diátaxis-first navigation ---
      // Top level is the four Diátaxis quadrants, with product (Arena / PackC)
      // as the second axis and Arena's bloated How-To grouped by theme subdir.
      // Starlight requires `autogenerate` to live inside an `items` array on a
      // labeled group, not as a sibling of `label`.
      sidebar: [
        {
          label: 'Overview',
          items: [
            { label: 'What is Arena', slug: 'arena' },
            { label: 'Why Arena', slug: 'arena/why-arena' },
            { label: 'PackC', slug: 'packc' },
          ],
        },
        {
          label: 'Tutorials',
          collapsed: true,
          items: [
            {
              label: 'Arena',
              items: [
                auto('arena/tutorials'),
                ...(has('arena/examples')
                  ? [{ label: 'Examples', collapsed: true, items: [auto('arena/examples')] }]
                  : []),
              ],
            },
            grp('PackC', 'packc/tutorials'),
          ],
        },
        {
          label: 'How-To Guides',
          collapsed: true,
          items: [
            {
              label: 'Arena',
              items: [
                { label: 'Overview', slug: 'arena/how-to' },
                grp('Getting set up', 'arena/how-to/setup'),
                grp('Interfaces', 'arena/how-to/interfaces'),
                grp('Writing tests', 'arena/how-to/scenarios'),
                grp('Providers', 'arena/how-to/providers'),
                grp('Agents, MCP & RAG', 'arena/how-to/agents'),
                grp('Voice', 'arena/how-to/voice'),
                grp('Deploy', 'arena/how-to/deploy'),
              ],
            },
            grp('PackC', 'packc/how-to'),
          ],
        },
        {
          label: 'Reference',
          collapsed: true,
          items: [
            grp('Arena', 'arena/reference'),
            grp('PackC', 'packc/reference'),
          ],
        },
        {
          label: 'Explanation',
          collapsed: true,
          items: [
            grp('Arena', 'arena/explanation'),
            grp('PackC', 'packc/explanation'),
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
