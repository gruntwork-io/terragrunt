# Terragrunt Docs: Starlight Rewrite

This is the rewrite of the Terragrunt website using [Starlight](https://github.com/withastro/starlight), a documentation website framework for Astro. The goal is to provide a more user-friendly and accessible documentation for Terragrunt users.

## Development

To get started, install the requisite dependencies to run the project locally using [mise](https://mise.jdx.dev/):

```bash
mise install
```

Afterwards, you'll want to install the dependencies for the project:

```bash
bun i
```

You'll also need to install [d2](https://github.com/terrastruct/d2/blob/master/docs/INSTALL.md) to build any diagrams:

You can now start the development server:

```bash
bun dev
```

## Adding shadcn/ui Components

This project includes a limited number of shadcn/ui components. To add new shadcn/ui components:

1. **Install a component**:
   ```bash
   npx shadcn@latest add [component-name]
   ```
   
   Example:
   ```bash
   npx shadcn@latest add card
   npx shadcn@latest add input
   npx shadcn@latest add dialog
   ```

2. **Components will be installed to** `src/components/ui/`

3. **Configuration**: The `components.json` file contains the shadcn/ui CLI configuration

### Usage Example

```tsx
import Button from '@components/ui/Button.tsx';

<Button variant="outline" className="bg-transparent border-white text-white hover:bg-white hover:text-black">
  Learn More
</Button>
```

## WIP

This is still a work in progress. Here are some of the tasks that need to be completed. For the list of tasks, see [TODO.md](TODO.md).
