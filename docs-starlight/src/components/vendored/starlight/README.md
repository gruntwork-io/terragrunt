# Vendored Starlight Components

This directory contains components vendored from `@astrojs/starlight` version 0.35.2.

## Why Vendored?

These components were vendored to add custom file tree icons for Terragrunt and OpenTofu file types, eliminating the need to patch the `@astrojs/starlight` dependency.

## Custom Icons

The following custom icons have been added to `file-tree-icons.ts` and `Icons.ts`:

- **`.hcl`** → `custom:terragrunt` (Terragrunt logo)
- **`.tf`, `.tf.json`, `.tfvars`, `.tfvars.json`** → `custom:opentofu` (OpenTofu logo)
- **`.terraform.lock.hcl`, `.tfplan`** → `custom:opentofu` (OpenTofu logo)

## Components

### File Tree Components

- **`FileTree.astro`** - Main FileTree component for rendering file/directory trees in documentation
- **`rehype-file-tree.ts`** - Rehype plugin that processes FileTree markup
- **`file-tree-icons.ts`** - Icon definitions and mappings for file types (includes custom icons)

### Card Components

- **`Card.astro`** - Card component for displaying content with optional icons (vendored to support custom icons)
- **`Icon.astro`** - Icon component for rendering SVG icons (vendored to use custom icon definitions)

### Icon Registry

- **`Icons.ts`** - Icon registry and SVG path definitions (includes custom Terragrunt and OpenTofu icons)

## Usage

### FileTree Component

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

### Card Component

```astro
import Card from '@components/vendored/starlight/Card.astro';

<Card title="Terragrunt Configuration" icon="custom:terragrunt">
  Content goes here
</Card>
```

## Maintenance

When upgrading `@astrojs/starlight`, check if there are changes to these components that should be merged. The original components can be found at:

- `node_modules/@astrojs/starlight/user-components/FileTree.astro`
- `node_modules/@astrojs/starlight/user-components/rehype-file-tree.ts`
- `node_modules/@astrojs/starlight/user-components/file-tree-icons.ts`
- `node_modules/@astrojs/starlight/user-components/Card.astro`
- `node_modules/@astrojs/starlight/user-components/Icon.astro`
- `node_modules/@astrojs/starlight/components/Icons.ts`
