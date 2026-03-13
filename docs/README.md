# Terragrunt Documentation

This is the documentation for Terragrunt (hosted at <https://docs.terragrunt.com>), built using [Starlight](https://github.com/withastro/starlight), a documentation framework for Astro.

## Development

To get started, install the requisite dependencies to run the project locally using [mise](https://mise.jdx.dev/):

```bash
mise install
```

Afterwards, you'll want to install the NPM dependencies for the project:

```bash
bun i
```

You'll also need to install [d2](https://github.com/terrastruct/d2/blob/master/docs/INSTALL.md) to build any diagrams referenced in the documentation:

You can now start the development server:

```bash
bun dev
```

This will start a development server on <http://127.0.0.1:4321> that will be automatically reloaded when you make changes to documentation.

## Building

When the project is ready to deployed, it will be built using the following command:

```bash
bun run build
```

This will generate a `dist` directory with the built documentation.

Running this locally can be useful if you see that the build fails in CI, as additional checks are performed in the build process, like ensuring that all links are valid.

## Hosting

The website is hosted on [Vercel](https://vercel.com/), and is automatically deployed when a new commit is pushed to the `main` branch.

Every pull request will result in a preview deployment of the documentation site. This preview site is only accessible by maintainers of the project to prevent running untrusted code in Vercel builds.

