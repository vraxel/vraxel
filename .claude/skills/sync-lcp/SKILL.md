---
name: sync-lcp
description: Scan vraxel changes and update docs/lcp-refactor.md with new items that LCP should backport. Use when asked to "记录LCP同步项", "sync lcp", "更新回同步清单", or after completing a non-vraxel-specific improvement.
---

# sync-lcp

Update `docs/lcp-refactor.md` with vraxel improvements that LCP should backport.

## Steps

1. **Read the existing doc** -- `docs/lcp-refactor.md`. Note the highest item number and all existing entries to avoid duplicates.

2. **Find new changes** -- Run `git log --oneline` and compare against the entries already in the doc. Look for commits that are NOT vraxel-specific features but general improvements in these categories:
   - **代码规范**: coding conventions, lint rules, naming standards
   - **问题修复**: bug fixes that likely exist in LCP too
   - **UI 美化**: visual polish, layout, design tokens
   - **架构优化**: structural improvements, patterns, tooling
   - **功能增强**: feature improvements applicable to LCP

3. **For each new item**, determine:
   - Does it fix/improve something that LCP likely has the same way? If yes, include it.
   - Is it vraxel-specific (e.g., agent gateway, compute module features)? If yes, skip it.

4. **Append new items** to the appropriate category section in `docs/lcp-refactor.md`. Each item follows this format:

```markdown
### {number}. {one-line title}

**背景**: What was wrong / suboptimal in the original LCP code.

**目的**: Why the change matters (optional if obvious from background).

**改动**:
- Which files / patterns changed and how
```

Keep it concise -- one item should be readable in 30 seconds. Include commit hashes only when they help locate the change.

5. **Do NOT commit** -- `docs/` is gitignored by convention.

6. **Report** what was added (item numbers and titles).
