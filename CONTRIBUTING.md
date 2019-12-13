# Contribution Guidelines

Contributions to this repo are very welcome! We follow a fairly standard [pull request 
process](https://help.github.com/articles/about-pull-requests/) for contributions, subject to the following guidelines:
 
1. [File a GitHub issue or write an RFC](#file-a-github-issue-or-write-an-rfc)
1. [Update the documentation](#update-the-documentation)
1. [Update the tests](#update-the-tests)
1. [Update the code](#update-the-code)
1. [Create a pull request](#create-a-pull-request)
1. [Merge and release](#merge-and-release)

## File a GitHub issue or write an RFC

Before starting any work, we recommend filing a GitHub issue in this repo. This is your chance to ask questions and
get feedback from the maintainers and the community before you sink a lot of time into writing (possibly the wrong) 
code. If there is anything you're unsure about, just ask!

Sometimes, the scope of the feature proposal is large enough that it requires major updates to the code base to
implement. In these situations, a maintainer may suggest writing up an RFC that describes the feature in more details
than what can be reasonably captured in a Github Issue. RFCs are written in markdown and live in the directory
`_docs/rfc`.

To write an RFC:

- Clone the repository
- Create a new branch
- Copy the template (`_docs/rfc/TEMPLATE.md`) to a new file in the same directory.
- Fill out the template
- Open a PR for comments, prefixing the title with the term `[RFC]` to indicate that it is an RFC PR.

## Update the documentation

We recommend updating the documentation *before* updating any code (see [Readme Driven 
Development](http://tom.preston-werner.com/2010/08/23/readme-driven-development.html)). This ensures the documentation 
stays up to date and allows you to think through the problem at a high level before you get lost in the weeds of 
coding.

The documentation is built with Jekyll and hosted on the Github Pages from `docs` folder on `master` branch. Check out
[Terragrunt website](docs/README.md) to learn more about working with the documentation.

## Update the tests

We also recommend updating the automated tests *before* updating any code (see [Test Driven 
Development](https://en.wikipedia.org/wiki/Test-driven_development)). That means you add or update a test case, 
verify that it's failing with a clear error message, and *then* make the code changes to get that test to pass. This 
ensures the tests stay up to date and verify all the functionality in this Module, including whatever new 
functionality you're adding in your contribution. Check out [Developing Terragrunt](README.md#developing-terragrunt) 
for instructions on running the automated tests. 

## Update the code

At this point, make your code changes and use your new test case to verify that everything is working. Check out 
[Developing Terragrunt](README.md#developing-terragrunt) for instructions on how to build and run Terragrunt locally.
 
## Create a pull request

[Create a pull request](https://help.github.com/articles/creating-a-pull-request/) with your changes. Please make sure
to include the following:

1. A description of the change, including a link to your GitHub issue.
1. The output of your automated test run, preferably in a [GitHub Gist](https://gist.github.com/). We cannot run 
   automated tests for pull requests automatically due to [security 
   concerns](https://circleci.com/docs/fork-pr-builds/#security-implications), so we need you to manually provide this 
   test output so we can verify that everything is working.
1. Any notes on backwards incompatibility.

## Merge and release   

The maintainers for this repo will review your code and provide feedback. If everything looks good, they will merge the
code and release a new version, which you'll be able to find in the [releases page](../../releases).
