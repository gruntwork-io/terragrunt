# TODO

The Starlight rewrite of the Terragrunt website is a work in progress.

Here are some of the tasks that need to be completed:

## Infrastructure

- [x] **Docker compose local dev setup**
- [x] **Vercel deployment**
  - [x] **Vercel preview deployments**
  - [x] **Custom domain setup** - [terragrunt-v1.gruntwork.io](https://terragrunt-v1.gruntwork.io)
- [ ] **Remove the no-index robots.txt**
- [ ] **Add a sitemap.xml**

## Content

- [x] **Content parity with current docs site**
  - [x] **Parity for all docs except CLI reference**
  - [x] **Parity for CLI reference**
- [x] **Redesign reference to use cards**
- [ ] **Setup redirects for all old URLs**
- [x] **Automate keeping versions updated**
  - [x] **Automate Terragrunt version lookup in docs**
  - [x] **Automate IaC Engine version lookup in docs**

## User Experience

- [x] **User feedback collection**
- [x] **Broken link checking**
  - [x] **Automate broken link checking**
  - [x] **Fix broken links**
    - [x] **Fix broken links in CLI reference**
    - [x] **Fix broken links in all other docs**
  - [x] **Require link checking**
- [ ] **Jekyll site banner indicating new site**
