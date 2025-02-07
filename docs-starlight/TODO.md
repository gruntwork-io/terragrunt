# TODO

The Starlight rewrite of the Terragrunt website is a work in progress.

Here are some of the tasks that need to be completed:

## Infrastructure
- [x] **Docker compose local dev setup**
- [x] **Vercel deployment**
  - [x] **Vercel preview deployments**
  - [x] **Custom domain setup** - [terragrunt-v1.gruntwork.io](https://terragrunt-v1.gruntwork.io)

## Content
- [ ] **Content parity with current docs site**
  - [x] **Parity for all docs except CLI reference**
  - [ ] **Parity for CLI reference**
- [ ] **Redesign reference to use cards**
- [ ] **Setup redirects for all old URLs**
- [ ] **Automate keeping versions updated**
  - [x] **Automate Terragrunt version lookup in docs**
  - [ ] **Automate IaC Engine version lookup in docs**

## User Experience
- [x] **User feedback collection**
- [ ] **Broken link checking**
  - [x] **Automate broken link checking**
  - [ ] **Fix broken links**
    - [ ] **Fix broken links in CLI reference**
    - [x] **Fix broken links in all other docs**
  - [x] **Require link checking**
- [ ] **Jekyll site banner indicating new site**
