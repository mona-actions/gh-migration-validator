# Validate Organization

The `validate-org` command validates repositories migrated from a source organization to a target organization in a single run, producing one consolidated report.

By default it lists all repositories in the source org and validates each one against the same-named repo in the target org. This works well for **org-by-org migrations** (e.g. using GEI) where repository names are preserved and all repos (including forks and archives) are migrated. Forked repos have issue validation skipped automatically since forks have issues disabled.

When source and target repository names differ, or you only want to validate a subset, provide a CSV mapping file via `--repo-list`.

## Usage

```bash
gh migration-validator validate-org \
  --github-source-org "source-org" \
  --github-target-org "target-org" \
  --github-source-pat "ghp_xxx" \
  --github-target-pat "ghp_yyy"
```

## With Repository Mapping (CSV)

When not all source repos exist in the target, or names differ:

```csv
# repos.csv
source_repo,target_repo
my-app,my-app-migrated
shared-lib,shared-lib
internal-tools,internal-tools
```

```bash
gh migration-validator validate-org \
  --github-source-org "source-org" \
  --github-target-org "target-org" \
  --github-source-pat "ghp_xxx" \
  --github-target-pat "ghp_yyy" \
  --repo-list repos.csv
```

If a CSV line has only one column, the target name is assumed to match the source. Lines starting with `#` are comments. A header row of `source_repo,target_repo` or `source,target` is automatically skipped.

## With Markdown Report

```bash
gh migration-validator validate-org \
  --github-source-org "source-org" \
  --github-target-org "target-org" \
  --github-source-pat "ghp_xxx" \
  --github-target-pat "ghp_yyy" \
  --markdown-table \
  --markdown-file "org-validation-report.md"
```

When `--markdown-file` is specified, results are written incrementally after each repository completes, so partial progress survives interruptions (e.g. Ctrl+C).

## Environment Variables

```bash
export GHMV_SOURCE_ORGANIZATION="source-org"
export GHMV_SOURCE_TOKEN="ghp_xxx"
export GHMV_TARGET_ORGANIZATION="target-org"
export GHMV_TARGET_TOKEN="ghp_yyy"

gh migration-validator validate-org
```

## Options

### Source Flags

- `--github-source-org` / `-s` (required): Source GitHub organization
- `--github-source-pat` / `-a` (required): Source GitHub token with read permissions
- `--source-hostname` / `-u` (optional): GitHub Enterprise Server URL for source

### Repository Selection

- `--repo-list` (optional): Path to CSV file with `source_repo,target_repo` mappings. When omitted, all repos from the source org are validated with matching names.

### Shared Target Flags (inherited from root)

- `--github-target-org` / `-t` (required): Target GitHub organization
- `--github-target-pat` / `-b` (required): Target GitHub token with read permissions
- `--target-hostname` / `-v` (optional): GitHub Enterprise Server URL for target
- `--markdown-table` / `-m`: Output results in markdown format
- `--markdown-file`: Write consolidated markdown report to a file
- `--no-lfs`: Skip LFS object validation
- `--strict-exit`: Exit with status 2 on validation failures

## How It Works

1. Determines repositories to validate, either from `--repo-list` CSV or by listing all repos in the source org
2. For each repo pair, validates source against target (names can differ when using CSV mapping)
3. Repos that fail or cannot be accessed are recorded with an error but do not stop validation of remaining repos
4. When `--markdown-file` is set, the report is updated after each repo (incremental writes)
5. Produces a **single summary table** with per-repository pass/fail/warn/info status

## Output

The consolidated report includes:

- A summary table with each repository's overall status (pass/fail/warn/info)
- Total counts across all repositories
- Per-repository detailed validation results (in the markdown report)
