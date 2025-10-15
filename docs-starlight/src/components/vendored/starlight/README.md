# Vendored Starlight Components

This directory contains components vendored from `@astrojs/starlight` version 0.35.2.

## Why Vendored?

These components were vendored to add custom file tree icons for Terragrunt and OpenTofu file types, eliminating the need to patch the `@astrojs/starlight` dependency.

## Custom Icons

The following custom icons have been added to `file-tree-icons.ts`:

- **`.hcl`** → `custom:terragrunt` (Terragrunt logo)
- **`.tf`, `.tf.json`, `.tfvars`, `.tfvars.json`** → `custom:opentofu` (OpenTofu logo)
- **`.terraform.lock.hcl`, `.tfplan`** → `custom:opentofu` (OpenTofu logo)

## Components

- **`FileTree.astro`** - Main FileTree component for rendering file/directory trees in documentation
- **`rehype-file-tree.ts`** - Rehype plugin that processes FileTree markup
- **`file-tree-icons.ts`** - Icon definitions and mappings for file types (includes custom icons)
- **`Icons.ts`** - Icon registry and SVG path definitions

## Usage

```astro
import FileTree from '@components/vendored/starlight/FileTree.astro';

<FileTree>
- src/
  - main.tf
  - variables.tf
  - terragrunt.hcl
- .terraform.lock.hcl
</FileTree>
```

## Maintenance

When upgrading `@astrojs/starlight`, check if there are changes to these components that should be merged. The original components can be found at:

- `node_modules/@astrojs/starlight/user-components/FileTree.astro`
- `node_modules/@astrojs/starlight/user-components/rehype-file-tree.ts`
- `node_modules/@astrojs/starlight/user-components/file-tree-icons.ts`
- `node_modules/@astrojs/starlight/components/Icons.ts`
