# There's a bug that prevents us from using bun for this:
# https://github.com/oven-sh/bun/issues/17022
#
# FROM oven/bun:1.2.2
#
# WORKDIR /app
# COPY package.json bun.lock ./
#
# RUN bun install
#
# COPY . .
#
# EXPOSE 4321
#
# CMD ["bun", "run", "dev"]
#
# NOTE: If we ever switch to bun in Docker because the bug has been squashed, we should make sure
# to address the other differences, like adding the patches and installing d2.

# Used to build diagrams.
FROM terrastruct/d2:v0.7.0 AS d2

# We use node instead of bun in containers because of the bug mentioned above with Astro and Bun.
FROM node:22

WORKDIR /app

COPY package.json package-lock.json ./

# We patch the starlight icons package so that we can use nice Terragrunt/OpenTofu icons.
COPY patches/ ./patches/

# Install d2.
COPY --from=d2 /usr/local/bin/d2 /usr/local/bin/d2

# Required because of issues with transitive dependencies.
RUN npm install --legacy-peer-deps

COPY . .

EXPOSE 4321

CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0"]
