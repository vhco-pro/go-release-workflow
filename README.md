# go-release-workflow

Reusable workflow that publishes a Go repository's signed GoReleaser release
for an exact tag. It checks out the tag, runs
[`michielvha/go-ci-action`](https://github.com/michielvha/go-ci-action) on it,
and only if that passes runs
[`michielvha/goreleaser-action`](https://github.com/michielvha/goreleaser-action)
with the Go version the checks resolved.

The checks run here as well as before the tag was created. Whoever or whatever
pushed the tag, nothing is published from source that failed them. A repository
that only wants CI calls `go-ci-action` directly; there is no CI-only workflow.

Versions and tags come from
[vhco-pro/version-workflow](https://github.com/vhco-pro/version-workflow): the
default branch tags after its checks pass, and a branch tags a prerelease while
its open PR has the `release:preview` label.

## Adopt it

Copy one example folder into the repository's `.github/workflows/` and replace
`FULL_COMMIT_SHA` with a released commit of this repository. Use one arrangement
per repository; enabling both releases the same tag twice.

| Example | Use when |
| --- | --- |
| [examples/separate-workflows](examples/separate-workflows) | A tag push, including one pushed by hand, starts the release. Needs a GitHub App to push the tag |
| [examples/same-workflow](examples/same-workflow) | No GitHub App. `GITHUB_TOKEN` pushes the tag and a dependent job in the same run releases it |

In the same-workflow arrangement, retry a failed release with **Re-run failed
jobs**. **Re-run all jobs** finds the tag already pushed and releases nothing,
and a tag pushed by hand publishes nothing.

## Contract

Inputs are defined in [go-release.yml](.github/workflows/go-release.yml):
`go-version-file`, `golangci-lint-config` and `tag` (only for the same-workflow
arrangement), with the `pgp-private-key` secret. The caller job grants
`contents: write`.

GoReleaser owns release creation, upload and publication. Keep asset and draft
replacement disabled in the caller's configuration, and inspect a partial
publication before retrying.

## This repository

[release.yml](.github/workflows/release.yml) runs [test.yml](.github/workflows/test.yml)
first, then releases this repository with
[vhco-pro/gh-release-workflow](https://github.com/vhco-pro/gh-release-workflow).
The tests lint the workflow and the examples with actionlint and check the
contract with `go test ./...`: action order, the Go version handoff, the checked
out commit, the signing secret's scope, pins and serialization, and that every
example passes only inputs and secrets the workflow declares. Locally:
`go tool actionlint .github/workflows/*.yml examples/*/*.yml` and `go test ./...`.
