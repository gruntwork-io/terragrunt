# Changelog entries

Each file here is one change in the changelog at `/process/changelog`, in a directory named for its release. `src/pages/process/changelog.astro` renders the release index, and `src/pages/process/changelog/[version].astro` renders each release's page.

```
v1.2.3/
  my-new-feature.mdx
  fix-some-bug.mdx
```

Each file has frontmatter, then a `####` heading and the description of the change:

```mdx
---
version: "v1.2.3"
category: "new-features"
order: 1
---

#### Feature title

Description of the change.
```

- `version` is the release, and matches the directory name. An entry for an unreleased version stays out of the build until that release.
- `category` is the section the entry appears under. The slugs and their order are `CHANGELOG_CATEGORIES` in `src/lib/changelog.ts`.
- `order` is optional, and sorts entries within a category, lowest first.
