---
name: docs-review
description: Review published documentation for accuracy, stale claims, internal content and competitive positioning. Use before a release, when docs have drifted, or when auditing what a repo publishes. Verifies commands and identifiers against the real binary rather than reading prose.
---

# Documentation review

Prose review finds typos. This finds **claims that are false** — commands that
error, flags that were renamed, limitations that were fixed months ago, and
internal notes published to strangers.

Work in this order. Steps 1 and 2 change what the rest of the review even looks
at, and skipping them is how you review the wrong files.

## 1. Establish the publication surface

Before reading anything, find out what actually gets published and from where.
Three categories, and they need different handling:

```bash
# What's generated at build time (edits here are overwritten)
cat .gitignore | grep -A20 -iE "generated|fetched|build time"

# What generates it
ls scripts/*docs* docs/scripts/* 2>/dev/null
grep -n "prebuild\|predocs" docs/package.json 2>/dev/null
```

- **Authored** — edit directly.
- **Generated from repo sources** — edit the *source*. In this repo every
  `examples/<name>/README.md` is copied verbatim to a public docs page by
  `scripts/prepare-examples-docs.sh`, with no review step in between. That is
  where internal working notes leak out.
- **Fetched from other repositories** — cannot be fixed here at all. Here that
  is `docs/.../deploy/<adapter>/`, pulled from the adapter repos by
  `fetch-adapter-docs.mjs`. An edit survives until the next build, then
  silently reverts. Fix it in the owning repo and open a PR there.

Confirm before editing:

```bash
git check-ignore -v <path>   # ignored ⇒ generated or fetched, not authored
git ls-files <dir> | wc -l   # 0 tracked files ⇒ same conclusion
```

## 2. Verify every command against the binary

The highest-yield check, and entirely mechanical. Docs claim flags exist; the
binary is the authority.

```bash
# Real flags
<cli> <subcommand> --help | grep -oE '\-\-[a-z-]+' | sort -u > /tmp/real.txt

# Flags the docs use
grep -rhoE '<cli> <subcommand> [^|]*' docs/ examples/*/README.md \
  | grep -oE '\-\-[a-z-]+' | sort -u > /tmp/doc.txt

comm -13 /tmp/real.txt /tmp/doc.txt     # documented but non-existent
```

Then confirm each hit really fails, rather than being a root-level or renamed flag:

```bash
<cli> <subcommand> --suspect-flag 2>&1 | grep -oE "unknown flag.*"
```

Also check the **binary name itself**. Docs outlive renames: this repo's
quickstarts told readers to run `arena`, which no binary provides — the CLI is
`promptarena`. The first command of two quickstarts could not be followed.

Do the same for any identifier with a registry behind it — assertion types, eval
types, provider names. Parse the YAML rather than grepping `type:`, which also
matches provider and schema fields:

```python
# collect assertion types from scenario YAML, compare against the generated catalog
```

## 3. Check every issue reference

**In this repo the rule is: no issue links in documentation at all.** Not open
ones either. A page cannot know when a ticket closes, and nothing in the docs
build notices — four voice caveats here pointed at issues closed two months
earlier, still telling readers that shipped features were missing.

```bash
grep -rEo "github.com/[^)]+/issues/[0-9]+" docs/ examples/*/README.md
```

For each, check state before deciding what to write:

```bash
gh issue view <n> --repo <owner/repo> --json state,stateReason,title
```

Replace with the limitation itself. **Verify the limitation before restating
it** — closed-as-completed does not mean shipped. Here the AEC issues were
closed months ago, but AEC is still absent from the released runtime, so both
"it's coming" and "it works" would have been wrong.

The counterpart belongs on the ticket: if closing an issue would make a
documented statement untrue, updating that page goes in its acceptance
criteria. Links run ticket → docs, never the reverse.

## 4. Scan for internal content

Anything a reader cannot follow, or should not see:

```bash
grep -rniE "local-backlog|roadmap|Day [0-9]|spike|proposal §|deferred to|v1 ?/ ?v2|\
Issue #[0-9]+|TODO|FIXME|not yet implemented|planned for|future release" \
  docs/ examples/*/README.md | grep -viE "v1alpha1|alphanumeric"
```

That last exclusion matters — `v1alpha1` matches `alpha` and buries the real
hits in false positives.

Distinguish two things that look alike:

- *"`graph` and `composite` are accepted by the schema but not yet implemented
  (they behave as `keyword`)"* — **keep**. An accurate limitation with no
  dependency on a ticket.
- *"Issue #216: Recording adapter system (future)"* — **remove**. Internal
  tracking and a promise.

## 5. Scan for positioning

```bash
grep -rniE "best.in.class|industry.leading|world.class|revolutionary|\
unlike other|competitors?|vs\.? [A-Z]|seamless|effortless|just works" docs/ examples/
```

Naming a competitor to say it is worse ages badly, invites argument, and is
often wrong on the details — the Langchain comparison here claimed
Python-specific and single-templates-only, neither true. Naming one as a
*vocabulary* reference ("if you already work in X's terminology") is helpful and
should stay. So should an honest "what to use something else for" section.

## 6. Verify documented behaviour by running it

Prose describing behaviour drifts silently. Run the thing and count.

```bash
cd examples/<name> && <cli> run --ci 2>&1 | grep "<the behaviour>"
```

Here the agent-loops page claimed three self-transitions and a fourth redirected
attempt; it is two and a third. Beware double-counting: the runtime and this
repo both log `workflow state transition` with identical fields, so raw log
counts are twice the real number.

## 7. Build and link-check

```bash
cd docs && npm run build
CHECK_LINKS_PORT=4455 npm run check-links   # slow: checks external URLs
```

If a preview server is already running on the default port, the checker binds
elsewhere and fails to start — hence the explicit port. `npx astro preview stop`
clears a stale daemon.

## Traps this review actually hit

Each of these produced a wrong conclusion before being caught:

- **`find -maxdepth 2`** missed skills nested three deep, leading to
  "these files don't exist" — and three real files were overwritten before the
  mistake surfaced. Check `git status` after any bulk write: `M` where you
  expected `A` means you clobbered something.
- **`grep -h`** strips filenames, so a following `grep -v <path>` filters
  nothing. Stale generated pages then look like live source hits.
- **Assuming a missing file is a bug.** `.claude/skills/promptarena-authoring/`
  is absent from this repo because `promptarena init` writes it into *new*
  projects. Verified by running `init --quick` and listing the output — the page
  was accurate and needed no change.
- **Fixing before reproducing.** Every correction above was confirmed by running
  the command, checking the issue state, or counting real output first.

## Reporting

Separate what you verified from what you inferred, and say which claims you
could not check. A finding is "this command errors, here is the output" — not
"this looks outdated".
