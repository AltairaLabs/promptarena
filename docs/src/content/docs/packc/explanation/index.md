---
title: Explanation
description: Background on the PromptPack format and on how packc compiles and validates packs.
sidebar:
  order: 0
verified:
  commit: 8c7a948d1eb921fd8db28cf9e76beccf7079cbd6
  sources:
    - packc/main_interactive.go
---
These pages explain what a pack contains and what `packc` does to build and check one. For the steps, see the [how-to guides](/packc/how-to/). For each command's flags, see the [reference](/packc/reference/).

- [Pack format](/packc/explanation/pack-format/): the parts of a `.pack.json` file, and why prompts ship as one compiled file.
- [Compilation](/packc/explanation/compilation/): how `packc compile` turns an arena config into a pack, and what it fills in.
- [Validation](/packc/explanation/validation/): what `packc compile`, `packc validate` and `promptarena validate` check, and which findings block.

## Related

- [Reference](/packc/reference/)
- [How-to guides](/packc/how-to/)
- [Tutorials](/packc/tutorials/)
