# GitHub Migration Validator

A GitHub CLI extension for validating GitHub organization and repository migrations by comparing key metrics between source and target repositories. Supports GitHub-to-GitHub, Bitbucket Server (Data Center) to GitHub, and Azure DevOps (Server or Services/cloud) to GitHub migrations.

## Overview

The GitHub Migration Validator helps ensure that your migration to GitHub has been completed successfully. It compares various repository metrics (issues, pull requests, tags, releases, commits) between source and target repositories and provides a detailed validation report.

Supported sources:

- **GitHub** (organization/repository) — full validation including migration archives
- **Bitbucket Server / Data Center** — API-based validation
- **Azure DevOps** (Server on-prem or Services/cloud) — API-based validation (single repo or whole team project)

## Documentation

- **[Export & Validate-from-Export](docs/export.md)** - Point-in-time export and validation workflow
- **[Validate Organization](docs/validate-org.md)** - Org-level migration validation (GitHub → GitHub)
- **[Bitbucket Validation](docs/bitbucket.md)** - Bitbucket Server / Data Center to GitHub
- **[Azure DevOps Validation](docs/ado.md)** - Azure DevOps (Server or Services/cloud) to GitHub
- **[Migration Archive Support](docs/migration-archive.md)** - Enhanced validation using GitHub migration archives

## Install

```bash
gh extension install mona-actions/gh-migration-validator
```

## Usage

### Basic Usage

```bash
gh migration-validator \
  --github-source-org "source-org" \
  --github-target-org "target-org" \
  --source-repo "my-repo" \
  --target-repo "my-repo" \
  --github-source-pat "ghp_xxx" \
  --github-target-pat "ghp_yyy"
```

### With Markdown Output

```bash
gh migration-validator \
  --github-source-org "source-org" \
  --github-target-org "target-org" \
  --source-repo "my-repo" \
  --target-repo "my-repo" \
  --github-source-pat "ghp_xxx" \
  --github-target-pat "ghp_yyy" \
  --markdown-table \
  --markdown-file "validation-report.md"
```

### Skipping LFS Validation

If you want to skip LFS object validation (useful for large repositories or when LFS is not used), use the `--no-lfs` flag:

```bash
gh migration-validator \
  --github-source-org "source-org" \
  --github-target-org "target-org" \
  --source-repo "my-repo" \
  --target-repo "my-repo" \
  --github-source-pat "ghp_xxx" \
  --github-target-pat "ghp_yyy" \
  --no-lfs
```

### Environment Variables

You can use environment variables instead of flags. All environment variables use the `GHMV_` prefix.

#### Source (GitHub-to-GitHub)

```bash
export GHMV_SOURCE_ORGANIZATION="source-org"
export GHMV_SOURCE_TOKEN="ghp_xxx"
export GHMV_SOURCE_REPO="my-repo"
export GHMV_SOURCE_HOSTNAME="https://github.example.com"  # Optional: GitHub Enterprise Server
```

#### Target (shared across all subcommands)

```bash
export GHMV_TARGET_ORGANIZATION="target-org"
export GHMV_TARGET_TOKEN="ghp_yyy"
export GHMV_TARGET_REPO="my-repo"
export GHMV_TARGET_HOSTNAME="https://github.example.com"  # Optional: GitHub Enterprise Server
```

#### Output & Behavior

```bash
export GHMV_MARKDOWN_TABLE="true"                  # Output as markdown table
export GHMV_MARKDOWN_FILE="validation-report.md"   # Write markdown to file
export GHMV_NO_LFS="true"                          # Skip LFS validation
export GHMV_STRICT_EXIT="true"                     # Exit code 2 on validation failures
export GHMV_RATE_LIMIT_THRESHOLD="100"             # GitHub API rate limit warning threshold (default: 50, 0 to disable)
```

#### Basic Run

```bash
gh migration-validator
```

### Strict Exit Mode

Use strict exit mode when you need shell pipelines to detect validation failures. Enable it with the `--strict-exit` flag or set `GHMV_STRICT_EXIT=true` to return exit code 2 whenever any validation fails. Without this option the command exits 0 while still reporting failures in the output.

```bash
gh migration-validator \
  --github-source-org "source-org" \
  --github-target-org "target-org" \
  --source-repo "my-repo" \
  --target-repo "my-repo" \
  --github-source-pat "ghp_xxx" \
  --github-target-pat "ghp_yyy" \
  --strict-exit
```

### GitHub App Authentication

For GitHub App authentication, use environment variables:

```bash
# Source GitHub App
export GHMV_SOURCE_APP_ID="123456"
export GHMV_SOURCE_PRIVATE_KEY="-----BEGIN RSA PRIVATE KEY-----\n..."
export GHMV_SOURCE_INSTALLATION_ID="987654"

# Target GitHub App
export GHMV_TARGET_APP_ID="123457"
export GHMV_TARGET_PRIVATE_KEY="-----BEGIN RSA PRIVATE KEY-----\n..."
export GHMV_TARGET_INSTALLATION_ID="987655"
```

### Enterprise Server Support

For GitHub Enterprise Server:

```bash
export GHMV_SOURCE_HOSTNAME="https://github.example.com"
```

## Export and Validation Workflow

Export source repository data at a specific point in time, then validate the target against that snapshot. Useful when the source continues to receive changes during migration.

For full usage, options, and workflow details, see **[Export & Validate-from-Export](docs/export.md)**.

## Validate Organization

Validate all repositories migrated from a source GitHub organization to a target organization in a single run, with an optional CSV mapping file for different source/target names.

For full usage, options, and output details, see **[Validate Organization](docs/validate-org.md)**.

## Bitbucket Validation

Validate migrations from Bitbucket Server / Data Center to GitHub. Compares PRs, tags, commits, branch permissions, and webhooks.

For full usage, options, and validation details, see **[Bitbucket Validation](docs/bitbucket.md)**.

## Azure DevOps Validation

Validate migrations from Azure DevOps (Server or Services/cloud) to GitHub. Supports single-repo and project-level validation with optional CSV mapping. Compares PRs, tags, commits, LFS objects, branch policies, and service hooks.

For full usage, options, and validation details, see **[Azure DevOps Validation](docs/ado.md)**.

## What Gets Validated

The tool compares the following metrics between source and target repositories:

- **Issues**: Total count (expects +1 in target for migration log issue)
- **Pull Requests**: Total, Open, Merged, and Closed counts
- **Tags**: Total count of Git tags
- **Releases**: Total count of GitHub releases
- **Commits**: Total commit count on default branch
- **Branch Protection Rules**: Total count of branch protection rules configured for the repository
- **Webhooks**: Total count of active repository webhooks
- **LFS Objects**: Total count of Git LFS (Large File Storage) objects referenced in the repository (can be skipped with `--no-lfs` flag)
- **Latest Commit SHA**: Ensures both repositories have the same latest commit in default branch

## Validation Results

- ✅ **PASS**: Metrics match expected values
- ❌ **FAIL**: Target is missing data from source
- ⚠️ **WARN**: Target has more data than source (usually acceptable)
- ℹ️ **INFO**: Advisory comparison only (e.g., Bitbucket branch permissions vs GitHub branch protection)

## Output Formats

### Console Output

The tool provides a formatted table with colored status indicators and a summary.

Example:

```
📊 Migration Validation Report

🔄 Source vs Target Validation

Metric                                 | Status  | Source Value                             | Target Value                             | Difference
Issues (expected +1 for migration log) | ⚠️ WARN  | 2 (expected target: 3)                   | 7                                        | Extra: 4
Pull Requests (Total)                  | ✅ PASS | 29                                       | 29                                       | Perfect match
Pull Requests (Open)                   | ✅ PASS | 0                                        | 0                                        | Perfect match
Pull Requests (Merged)                 | ✅ PASS | 27                                       | 27                                       | Perfect match
Tags                                   | ✅ PASS | 25                                       | 25                                       | Perfect match
Releases                               | ✅ PASS | 25                                       | 25                                       | Perfect match
Commits                                | ✅ PASS | 64                                       | 64                                       | Perfect match
Branch Protection Rules                | ✅ PASS | 1                                        | 1                                        | Perfect match
Webhooks                               | ✅ PASS | 0                                        | 0                                        | Perfect match
LFS Objects                            | ✅ PASS | 15                                       | 15                                       | Perfect match
Latest Commit SHA                      | ✅ PASS | d11552345ad4ffea894b59d9a4145a5119d77dba | d11552345ad4ffea894b59d9a4145a5119d77dba | N/A

📦 Migration Archive vs Source Validation

Metric                           | Status  | Source API Value | Archive Value | Difference
Archive vs Source Issues         | ❌ FAIL | 2                | 6             | Missing: 4
Archive vs Source Pull Requests  | ✅ PASS | 29               | 29            | Perfect match
Archive vs Source Protected Branches | ✅ PASS | 1            | 1             | Perfect match
Archive vs Source Releases       | ✅ PASS | 25               | 25            | Perfect match

🎯 Migration Archive vs Target Validation

Metric                                                    | Status  | Archive Value | Target Value | Difference
Archive vs Target Issues (expected +1 for migration log)  | ✅ PASS | 6 (expected target: 7) | 7     | Perfect match
Archive vs Target Pull Requests                           | ✅ PASS | 29            | 29           | Perfect match
Archive vs Target Protected Branches                      | ✅ PASS | 1             | 1            | Perfect match
Archive vs Target Releases                                | ✅ PASS | 25            | 25           | Perfect match

📊 Passed: 16  📊 Failed: 1  📊 Warnings: 1

❌ Migration validation FAILED - Some data is missing in target
```

### Markdown Output

Use the `--markdown-table` flag to generate copy-paste ready markdown for documentation.

## Dependencies

- [Go](https://golang.org/doc/install) 1.20 or higher
- Key dependencies:
  - [Cobra](https://github.com/spf13/cobra) - CLI framework
  - [Viper](https://github.com/spf13/viper) - Configuration management
  - [go-github](https://github.com/google/go-github) - GitHub REST API client
  - [githubv4](https://github.com/shurcooL/githubv4) - GitHub GraphQL API client
  - [go-githubauth](https://github.com/jferrl/go-githubauth) - GitHub App authentication
  - [go-github-ratelimit](https://github.com/gofri/go-github-ratelimit) - Rate limit handling
  - [pterm](https://github.com/pterm/pterm) - Terminal styling and formatting

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](.github/contributing.md) for guidelines.

## License

[MIT](./LICENSE) © [Mona-Actions](https://github.com/mona-actions)
