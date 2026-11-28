# Patterns

Each file here is one pattern on the patterns page at `/guides/patterns`. `src/pages/guides/patterns.astro` renders the card grid, and `src/pages/guides/patterns/[slug].astro` renders each pattern's page.

To add a pattern, create a file such as `my-pattern.mdx`. Its name without the extension is the pattern's slug under `/guides/patterns/`.

```mdx
---
title: "My pattern"
description: "A short description."
author: "Your Name"
order: 3
---

The full pattern, in Markdown.
```

- `title` is the card's title and the heading of the pattern's page.
- `description` appears on the card under the title.
- `author` appears on the card and the pattern's page.
- `order` is optional. Patterns sort by it, lowest first, then alphabetically by title.
