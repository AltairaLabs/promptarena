---
name: docs-review
description: Review and rewrite PromptArena docs pages against docs/STYLE.md and the code — Diátaxis quadrant, verified facts, voice, diagrams — and record what each page was verified against. Use when asked to review, fix, tidy or rewrite docs pages or example READMEs, to run a docs batch, before a release, or to refresh stale pages (--stale).
---

# docs-review

Brings published pages into line with `docs/STYLE.md` and with the code.
Prose review finds typos; this finds **claims that are false**: options that
were renamed, flags that never existed, limitations fixed months ago, and
internal notes published to strangers.

Invoked as `/docs-review <page-or-directory>`, or `/docs-review --stale [path]`
for upkeep.

Read `docs/STYLE.md` in full before starting. It is the authority; this file
is the procedure.

This skill is shared across the PromptKit family of repositories (PromptKit,
PromptArena; Omnia runs the original). Everything outside "This repository" is
identical in each copy, as are the `scripts/docs-*`, `scripts/lib/` and
`docs/check-mermaid.mjs` files; a diff between copies is drift.

## This repository

- **Product:** PromptArena, the `promptarena` and `packc` CLIs. Build with
  `make build-go` (binaries in `bin/`); `--help` on any subcommand is the
  authority on flags.
- **Quadrants:** `arena/<quadrant>/` and `packc/<quadrant>/`, where
  `<quadrant>` is `tutorials`, `how-to`, `reference` or `explanation`.
  `arena/index.md` and `arena/why-arena.md` are explanation.
- **Verify sources (`{sources}`):** `arena/` (`arena/cmd/promptarena/` for
  commands and flags, `arena/arenaconfig/` for config fields,
  `arena/assertions/` for assertion types, `arena/engine/` for run
  behaviour), `packc/`, and the schemas in `schemas/v1alpha1/` (generated
  from the Go config types by `make schemas`; never hand-edited). For
  PromptKit runtime behaviour, read the module cache at the version pinned
  in `go.mod` (`go list -m -f '{{.Dir}}' github.com/AltairaLabs/PromptKit/runtime`),
  not a sibling checkout.
- **Every `examples/<name>/README.md` is published verbatim** as a public
  docs page by `scripts/prepare-examples-docs.sh`, with no review step in
  between. This is where internal working notes leak out; see AGENTS.md.
  Review and edit the README. `docs/src/content/docs/arena/examples/` is
  generated from them and gitignored; never edit it.
- `docs/src/content/docs/arena/*/deploy/<adapter>/` is **fetched from the
  adapter repositories** by `docs/scripts/fetch-adapter-docs.mjs`. Fix those
  in `promptarena-deploy-<adapter>`.
- Assertion and eval types have a registry behind them; regenerate the
  catalog with `promptarena gen-reference` and compare against it.
- **Commits:** conventional (`docs:`), signed off: `git commit -s`. Hold new
  code to `golangci-lint`, but do not churn the inherited baseline.

## Setup

1. Work in a worktree on a branch `docs/review-<dir>`, never on `main` in the
   main checkout.
2. Establish the publication surface for the target. Every page is one of:
   - **Authored**: edit directly.
   - **Generated from repo sources**: edit the *source*, not the output.
   - **Fetched from another repository**: cannot be fixed here. An edit
     survives until the next build, then silently reverts. Fix it in the
     owning repo.

   Confirm before editing:

   ```bash
   git check-ignore -v <path>   # ignored ⇒ generated or fetched, not authored
   git ls-files <dir> | wc -l   # 0 tracked files ⇒ same conclusion
   ```
3. List the target pages (`*.md`, `*.mdx`). Record each page's word count and
   its quadrant (from its directory; see "This repository").
4. Stage 0, mechanical findings, kept per page:
   - `bash scripts/check-docs-voice.sh`
   - `npm --prefix docs run check:mermaid`
   - every CLI command and flag the pages use, against the binary's `--help`
     (see "Checking commands" below).

## Per-page pipeline

Run pages through stages 1–4 independently: start page B's stage 1 as soon as
it is dispatched, without waiting for page A's stage 4. Dispatch each stage as
an Agent call with the `model` shown, passing the prompt file's contents (with
`{product}`, `{repo}`, `{quadrant}` and `{sources}` filled in from "This
repository") plus the inputs listed. Write each stage's JSON output to
`<scratchpad>/docs-review/<slug>/<stage>.json`.

| Stage | Model | Prompt | Inputs |
|---|---|---|---|
| 1 classify | haiku | `prompts/classify.md` | page path, page text, the page's quadrant, STYLE.md "Quadrants" and "Diagrams" sections |
| 2 extract | haiku | `prompts/extract.md` | page path, page text |
| 3 verify | sonnet | `prompts/verify.md` | stage 2 JSON, repo path, the verify sources |
| 4 rewrite | sonnet | `prompts/rewrite.md` | page path, STYLE.md in full, stage 0 findings, stage 1 JSON, stage 3 JSON |

Stages 1 and 2 can run in parallel for the same page. Stage 3 needs stage 2,
and stage 4 needs stages 1 and 3.

Rules you enforce between stages:
- Read every stage 1 action other than `keep` before stage 4 runs. Override
  any that strips a how-to of the config its steps need, moves a block to a
  page that does not exist, or contradicts the quadrant rules; record the
  override in the stage 1 JSON's `why`.
- If a move targets another page in the batch, run that page's stage 4
  first, then the mover's: two agents must not edit one file at once.
- Spot-check every `dead` verdict in the code before stage 4 removes the
  field from the docs, and report the field as a bug if it looks like a
  wiring gap rather than a field to delete.
- Stage 4 must not change a fact that stage 3 marked `true`. It must use
  stage 3's `correct` value verbatim for `false` claims, and it must leave
  `unverified` claims as written.
- A block that stage 1 moves to another page is applied by stage 4 to both
  pages. If the target page does not exist, stage 4 creates it in the right
  quadrant directory, with frontmatter `title` and `description`.
- A generated page is rewritten at its source (an example README, a Go doc
  comment), then regenerated. Never edit the output.

## Batch close

1. Run `bash scripts/check-docs-voice.sh`, `npm --prefix docs run check:mermaid`
   and `npm --prefix docs run build`. Fix failures by re-running stage 4 for
   the page concerned, with the failure appended to its inputs.
2. Stage 5: dispatch one Agent with `model: opus` and `prompts/review.md`,
   passing `git diff main...HEAD -- docs/` (plus any example READMEs changed)
   and every page's stage 3 JSON. It returns `approve` or a list of
   `{page, reason}`. Re-run stage 4 for each returned page with the reason
   appended. If Opus returns the same page a second time, run that page's
   third stage 4 with `model: opus`, and record the escalation. A reviewer's
   factual claim is a claim like any other: verify it in the code before
   acting on it.
3. Record provenance for every page that went through stage 3:
   `python3 scripts/docs-provenance.py <page> <slug>/stage2.json <slug>/stage3.json <commit>`,
   where `<commit>` is `origin/main` as it was when the batch started (the code
   the claims were checked against). This writes the page's `verified`
   frontmatter and its sidecar in `docs/provenance/`. Commit both. Pages
   generated from another source (example READMEs) get no record: the
   frontmatter would be overwritten at the next build.
4. Add the batch's directory to `scripts/docs-voice-enforced.txt`, and confirm
   `bash scripts/check-docs-voice.sh` exits 0.
5. Build and link-check:

   ```bash
   npm --prefix docs run build
   (cd docs && CHECK_LINKS_PORT=4455 npm run check-links)   # slow: checks external URLs
   ```

   If a preview server is already running on the default port, the checker
   binds elsewhere and fails to start; hence the explicit port.
   `npx astro preview stop` clears a stale daemon.
6. Write the batch report to the PR body:
   - fact corrections (old → new, file:line);
   - blocks moved (from → to);
   - pages created;
   - diagrams added or converted;
   - dead options, as one list for a single removal issue;
   - unverified claims;
   - code bugs found (not fixed);
   - model escalations;
   - words before → after per page, and the total.
7. Commit (see "This repository" for the commit convention), push, open the
   PR. File one issue for the dead-option list if it is non-empty.

## Stale mode (`/docs-review --stale [path]`)

Use this for upkeep. A full review costs 100–250k tokens per page; most of
it re-verifies claims that are still true. Stale mode re-checks only what the
code changed.

1. `bash scripts/docs-stale.sh` lists pages whose `verified.sources` changed
   (`stale`, `missing`, `unknown-commit`). `bash scripts/docs-coverage.sh`
   lists public-surface files no page cites (`uncovered`). Restrict to
   `[path]` if given. The weekly `docs-freshness` issue has the same lists.
2. For each stale page, get the changes:
   `git diff <verified.commit> -- <changed sources>`. A `missing` source was
   deleted or renamed: find its successor with
   `git log --follow --diff-filter=R` or the page's claims, and treat every
   claim citing it as selected. An `unknown-commit` page has no usable
   baseline, so give it the full pipeline.
3. From the page's sidecar (`docs/provenance/<page>.json`), select the claims
   whose evidence the stale report names: `cited lines N` selects the claims
   citing those lines; `cited without a line`, `file changed` and
   `surface added` select every claim citing the file. Only those are
   re-verified; the rest stay trusted.
4. One Sonnet agent per group of up to 4 stale pages. For each page it runs
   `prompts/verify.md` on the selected claims only, and
   `prompts/surface-diff.md` on the diff hunks plus the page sections that
   document those files. No driver agents, and no Haiku classify/extract
   unless the page text changed outside a review.
5. The same agent rewrites only the affected sections: false claims take
   `correct` verbatim, dead ones are removed, and added surface is written in
   the page's existing structure. No whole-page voice pass (CI runs the voice
   check).
6. Re-record provenance with
   `python3 scripts/docs-provenance.py --merge <page> <stage2.json> <stage3.json> <sha of origin/main>`.
   The stage 3 input holds only the re-verified claims, using the sidecar's
   claim ids. `--merge` replaces those and keeps every other claim, so no
   watched source is lost.
7. `uncovered` surface: document it on the page that owns the topic, or
   create a page from the quadrant skeleton in `docs/STYLE.md`, and give that
   page the full pipeline (it has nothing to trust yet). A file that
   deliberately has no page goes in `scripts/docs-coverage-ignore.txt`.
8. Opus reviews only new pages and pages with 5 or more fact changes.
9. Run the batch-close checks, then open one PR titled
   `docs: refresh stale pages (<date>)`.

Expected cost: a page whose source gained one field is a diff hunk, a few
claims and one section rewrite. That is a few thousand tokens.

## Checking commands

The highest-yield stage 0 check, and entirely mechanical. Docs claim flags
exist; the binary is the authority.

```bash
# Real flags
<cli> <subcommand> --help | grep -oE '\-\-[a-z-]+' | sort -u > /tmp/real.txt

# Flags the docs use
grep -rhoE '<cli> <subcommand> [^|]*' docs/src/content/docs examples/*/README.md 2>/dev/null \
  | grep -oE '\-\-[a-z-]+' | sort -u > /tmp/doc.txt

comm -13 /tmp/real.txt /tmp/doc.txt     # documented but non-existent
```

Confirm each hit really fails, rather than being a root-level or renamed flag:
`<cli> <subcommand> --suspect-flag 2>&1 | grep -oE "unknown flag.*"`.

Check the **binary name itself**. Docs outlive renames: quickstarts once told
readers to run `arena`, which no binary provides (the CLI is `promptarena`),
so the first command of two quickstarts could not be followed.

Do the same for any identifier with a registry behind it: assertion types,
eval types, provider names. Parse the YAML rather than grepping `type:`,
which also matches provider and schema fields.

## Issue references

No issue links in documentation at all, open ones included (STYLE.md
"History"). When removing one, check its state first
(`gh issue view <n> --repo <owner/repo> --json state,stateReason,title`) and
**verify the limitation before restating it**: closed-as-completed does not
mean shipped. The acoustic-echo issues were closed months before AEC reached
the released runtime, so both "it's coming" and "it works" would have been
wrong.

## Traps this review actually hit

Each produced a wrong conclusion before being caught:

- **`find -maxdepth 2`** missed files nested three deep, leading to "these
  don't exist", and three real files were overwritten before the mistake
  surfaced. Check `git status` after any bulk write: `M` where you expected
  `A` means you clobbered something.
- **`grep -h`** strips filenames, so a following `grep -v <path>` filters
  nothing. Stale generated pages then read as live source hits.
- **Assuming a missing file is a bug.** A skill directory absent from the repo
  turned out to be written by `init` into *new* projects; verified by running
  it and listing the output. The page was accurate and needed no change.
- **Double-counting logs.** The PromptKit runtime and PromptArena both emit
  `workflow state transition` with identical fields, so raw log counts are
  twice the real number. Count behaviour by running it, and know which layer
  logs what.
- **Fixing before reproducing.** Every correction is confirmed first: by
  running the command, checking the issue state, or counting real output.

## Reporting

Separate what you verified from what you inferred, and say which claims you
could not check. A finding is "this command errors, here is the output", not
"this looks outdated".
