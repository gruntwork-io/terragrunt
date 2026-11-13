# Vendored Starlight components

Copies of components from `@astrojs/starlight` 0.42.10, changed where Starlight has no option for what the docs need:

- The file tree shows Terragrunt and OpenTofu icons for their files. Pages import `FileTree` from `@components/vendored/starlight/FileTree.astro` in place of Starlight's.
- Sidebar labels break only after an underscore, and their wrapped lines are indented, so a name such as `get_terraform_commands_that_need_parallelism` doesn't break mid-word. Starlight renders a label as plain text, so it can't hold a `<wbr>`. `astro.config.mjs` registers `Sidebar.astro` as Starlight's `Sidebar`.

| File | Starlight source | Changes |
| --- | --- | --- |
| `FileTree.astro` | `src/user-components/FileTree.astro` | None |
| `file-tree-processor.ts` | `src/user-components/file-tree-processor.ts` | Reads icons from `terragrunt-icons.ts` |
| `file-tree-icons.ts` | `src/user-components/file-tree-icons.ts` | None |
| `Icons.ts` | `src/components-internals/Icons.ts` | Import path |
| `Sidebar.astro` | `src/components/Sidebar.astro` | Import paths |
| `SidebarSublist.astro` | `src/components/SidebarSublist.astro` | Import paths, a local `hasCurrent` in place of Starlight's internal `sidebarGroupHasCurrent`, and label line breaks and indent |

`terragrunt-icons.ts` isn't from Starlight. It adds the `custom:terragrunt` icon for `.hcl` files, and the `custom:opentofu` icon for `.tf`, `.tf.json`, `.tfvars`, `.tfvars.json`, `.tfplan`, and `.terraform.lock.hcl` files.

## Upgrading

Copy each file from `packages/starlight/` at the new release's tag in [withastro/starlight](https://github.com/withastro/starlight), e.g. `@astrojs/starlight@0.42.10`. The npm package has only compiled JavaScript for the `.ts` files. Then reapply the changes in the table and update the version above.
