# FAQ entries

Each file here is one question on the FAQ at `/process/faq`. `src/pages/process/faq.astro` renders the index table, and `src/pages/process/faq/[slug].astro` renders each question's page.

To add a question, create a file such as `how-do-i-do-x.mdx`. Its name without the extension is the question's slug under `/process/faq/`.

```mdx
---
question: "How do I do X?"
description: "A short, 1-2 line answer."
order: 3
---

The full answer, in Markdown.
```

- `question` is the question's link in the index table and the heading of its page.
- `description` is the answer preview in the table, cut to two lines.
- `order` is optional. Questions sort by it, lowest first, then alphabetically.
