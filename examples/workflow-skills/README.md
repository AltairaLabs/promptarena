# Workflow + Skills Example

Demonstrates how to combine **workflow states** with **directory-based skill filtering** and the **three-level tool scoping** model.

## What This Shows

| Feature | How it's demonstrated |
|---------|----------------------|
| **Directory-based skills** | Skills organized under `skills/billing/`, `skills/orders/`, `skills/escalation/` |
| **Workflow skill filtering** | Billing state only sees `skills/billing/`, orders state only sees `skills/orders/` |
| **Three-level tool scoping** | Pack declares all tools → prompts select baseline → skills extend on activation |
| **Preloaded skills** | `brand-voice` is always available (no activation needed) |
| **Skill assertions** | Arena scenarios assert `skill_activated` and `skill_not_activated` |

## Directory Structure

```
workflow-skills/
├── config.arena.yaml               # Arena test configuration (declares skills)
├── mock-responses.yaml             # Deterministic mock responses
├── providers/
│   └── mock-provider.yaml
├── scenarios/
│   ├── billing-flow.scenario.yaml  # Billing path with PCI skill activation
│   └── orders-flow.scenario.yaml   # Orders path with troubleshooting skill
└── skills/
    ├── brand-voice/                # Top-level — always available
    │   └── SKILL.md
    ├── billing/                    # Scoped to billing workflow state
    │   ├── pci-compliance/
    │   │   └── SKILL.md
    │   └── refund-processing/
    │       └── SKILL.md
    ├── orders/                     # Scoped to orders workflow state
    │   └── order-troubleshooting/
    │       └── SKILL.md
    └── escalation/                 # Available in intake (all skills)
        └── escalation-policy/
            └── SKILL.md
```

## Workflow

```
intake ──RouteBilling──→ billing ──Resolve──→ closed
   │                                            ↑
   └────RouteOrders────→ orders ───Resolve──────┘
```

- **intake**: All skills available (no `skills` filter)
- **billing**: Only `skills/billing/*` skills (pci-compliance, refund-processing)
- **orders**: Only `skills/orders/*` skills (order-troubleshooting)
- **closed**: No skills (`skills: none`)

## Three-Level Tool Scoping

```
Ceiling (declared tools):  refund
                              │
Prompt baseline:           skill__activate, skill__deactivate,
                           skill__read_resource, workflow__transition
                              │
Skill extension:           + refund  ← only reachable once a skill
                                       naming it in allowed-tools is active
```

Two rules, and the example is built so that both are visible:

1. **A skill can only grant a tool the runtime declares.** The ceiling is the
   union of a compiled pack's tools and the tools this config declares under
   `tools:`. `escalation-policy` asks for `escalate_ticket` and
   `order-troubleshooting` asks for `search_orders`; neither tool is declared,
   so neither is ever granted — asking is not enough.
2. **The grant has to matter.** `refund` is deliberately left out of the prompt
   baselines, so the only route to it is activating `refund-processing` or
   `pci-compliance`. A baseline that already contained `refund` would make the
   grant a no-op and the example would prove nothing.

Verified against a live model: with the grants in place the model activates a
skill and then calls `refund`; with `refund` removed from both skills'
`allowed-tools` and nothing else changed, it activates the same skills and never
calls `refund`, because it was never offered.

Note that the mock provider cannot show this. A mock emits whatever tool calls
its response file names, regardless of the tools array it was handed, so
`promptarena run` against the mock exercises activation but not gating.

## Running

```bash
cd examples/workflow-skills
promptarena run --ci --format markdown
```
