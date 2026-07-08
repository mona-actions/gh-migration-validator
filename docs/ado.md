# Azure DevOps Validation

The `ado` subcommand validates migrations from Azure DevOps to GitHub by comparing API metrics between the source ADO repository and the target GitHub repository. It works with both **Azure DevOps Server** (on-prem) and **Azure DevOps Services** (cloud). Flag names are aligned with GEI's `ado2gh`.

For cloud (Azure DevOps Services), omit `--ado-server-url` — it defaults to `https://dev.azure.com`. For on-prem (Azure DevOps Server), pass `--ado-server-url` including any collection path prefix (e.g. `https://tfs.example.com/tfs`).

You can validate a **single repository** or an **entire team project** (all repositories) in one run.

## Usage (cloud, single repository)

```bash
gh migration-validator ado \
  --ado-org "my-org" \
  --ado-team-project "MyProject" \
  --ado-repo "my-repo" \
  --ado-pat "your-ado-token" \
  --github-target-org "target-org" \
  --target-repo "my-repo" \
  --github-target-pat "ghp_yyy"
```

## Usage (on-prem, single repository)

```bash
gh migration-validator ado \
  --ado-server-url "https://ado.example.com/tfs" \
  --ado-org "my-collection" \
  --ado-team-project "MyProject" \
  --ado-repo "my-repo" \
  --ado-pat "your-ado-token" \
  --github-target-org "target-org" \
  --target-repo "my-repo" \
  --github-target-pat "ghp_yyy"
```

## Usage (entire team project)

Omit `--ado-repo` to validate every repository in the team project against the same-named repository in the target GitHub organization. This produces a single consolidated report (same format as `validate-org`).

```bash
gh migration-validator ado \
  --ado-server-url "https://ado.example.com/tfs" \
  --ado-org "my-collection" \
  --ado-team-project "MyProject" \
  --ado-pat "your-ado-token" \
  --github-target-org "target-org" \
  --github-target-pat "ghp_yyy" \
  --markdown-file "ado-validation-report.md"
```

When `--markdown-file` is specified in project mode, results are written incrementally after each repository completes, so partial progress survives interruptions.

## Usage (repository name mapping via CSV)

In project mode, when the ADO repository name differs from the target GitHub repository name, or you only want to validate a subset of repositories, provide a CSV mapping file via `--repo-list`. This mirrors the `validate-org` behavior.

```csv
source_repo,target_repo
MyRepo,my-repo-migrated
SharedLib,SharedLib
```

```bash
gh migration-validator ado \
  --ado-org "my-org" \
  --ado-team-project "MyProject" \
  --ado-pat "your-ado-token" \
  --github-target-org "target-org" \
  --github-target-pat "ghp_yyy" \
  --repo-list repos.csv
```

If a line has only one column, the target name is assumed to match the source. Lines starting with `#` are treated as comments. When `--repo-list` is omitted, every repository in the team project is validated against the same-named target repository.

## Environment Variables

```bash
export GHMV_ADO_SERVER_URL="https://ado.example.com/tfs"  # Optional: omit for Azure DevOps Services (cloud)
export GHMV_ADO_ORG="my-collection"
export GHMV_ADO_TEAM_PROJECT="MyProject"
export GHMV_ADO_REPO="my-repo"        # Optional: omit for whole-project validation
export GHMV_ADO_PAT="your-ado-token"
export GHMV_ADO_API_VERSION="6.0"     # Optional: auto-detected when omitted
export GHMV_TARGET_ORGANIZATION="target-org"
export GHMV_TARGET_TOKEN="ghp_yyy"
export GHMV_TARGET_REPO="my-repo"     # Required only for single-repo validation
export GHMV_NO_PRS="true"             # Optional: skip PR validation
export GHMV_NO_WEBHOOKS="true"        # Optional: skip webhook validation
gh migration-validator ado
```

## Options

### ADO Source Flags

- `--ado-server-url` / `-H` (optional): Azure DevOps Server URL for on-prem (e.g., `https://ado.example.com/tfs`). Omit for Azure DevOps Services (cloud); defaults to `https://dev.azure.com`.
- `--ado-org` / `-o` (required): Azure DevOps organization (cloud) or collection (on-prem) name
- `--ado-team-project` / `-P` (required): Azure DevOps team project name
- `--ado-repo` / `-r` (optional): Repository name. Omit to validate all repos in the team project.
- `--ado-pat` / `-k` (required): Azure DevOps personal access token
- `--ado-api-version` (optional): REST API version. Auto-detected when omitted (tries 7.1 down to 4.1). Pin explicitly if needed, e.g. `7.1` for cloud, `4.1`/`5.0` for older TFS/ADO Server.
- `--repo-list` (optional, project mode only): Path to a CSV file with `source_repo,target_repo` mappings. When omitted, all repos in the team project are validated with matching names.

### ADO-specific Skip Flags

- `--no-prs`: Skip pull request validation (useful for git-only migrations that don't migrate PRs)
- `--no-webhooks`: Skip webhook/service hook validation

### Shared Target Flags (inherited from root)

- `--github-target-org` / `-t` (required): Target GitHub organization
- `--github-target-pat` / `-b` (required): Target GitHub token (or `GHMV_TARGET_TOKEN`)
- `--target-repo` (required for single-repo mode): Target repository name
- `--target-hostname` / `-v`: GitHub Enterprise Server URL (optional)
- `--markdown-table` / `-m`: Output results in markdown format
- `--markdown-file`: Write markdown output to the specified file
- `--no-lfs`: Skip LFS object validation
- `--strict-exit`: Exit with status 2 on validation failures

## What Gets Validated (Azure DevOps → GitHub)

| Metric                                             | Status      | Notes                                             |
| -------------------------------------------------- | ----------- | ------------------------------------------------- |
| Pull Requests (Total, Active→Open, Completed→Merged, Abandoned→Closed) | ✅ Compared | ADO PR states mapped to GitHub terminology; skip with `--no-prs` |
| Tags                                               | ✅ Compared |                                                   |
| Commits                                            | ✅ Compared | Default branch only                               |
| Latest Commit SHA                                  | ✅ Compared | Mismatch caused by `git lfs migrate` downgraded to ⚠️ WARN when the parent commit matches (see notes) |
| Branch Policies vs Branch Protection Rules         | ℹ️ Advisory | Different concepts — shown for reference only     |
| Service Hooks                                      | ✅ Compared | Collection-level subscriptions scoped to the repo; skip with `--no-webhooks` |
| Issues                                             | ⏭️ Skipped  | ADO uses Work Items, not native git issues        |
| Releases                                           | ⏭️ Skipped  | ADO has no equivalent for git repos               |
| LFS Objects                                        | ✅ Compared | Counted from .gitattributes patterns; skip with `--no-lfs` |

## Notes

- Uses PAT authentication (HTTP Basic). When `--ado-api-version` is omitted, the tool auto-detects a supported REST API version by probing the server (tries newest to oldest: 7.1, 7.0, 6.0, 5.1, 5.0, 4.1), so it works across TFS 2018, ADO Server 2019/2020+, and ADO Services
- To pin a version explicitly, set `--ado-api-version` (or `GHMV_ADO_API_VERSION`); a `404` on an existing repository usually means the pinned version is not supported by that server
- For Azure DevOps Services (cloud), omit `--ado-server-url` (defaults to `https://dev.azure.com`). For on-prem, pass the server URL including any collection path prefix (e.g. `https://tfs.example.com/tfs`); legacy `https://{org}.visualstudio.com` URLs are also supported
- If the server is unreachable, version detection fails fast instead of retrying every candidate version
- LFS objects are counted by reading `.gitattributes` for LFS-tracked patterns and inspecting matching files for pointer content (same approach as the GitHub side); use `--no-lfs` to skip. Note that a manual migration which runs `git lfs migrate` converts previously non-LFS files into LFS, so the target may legitimately report more LFS objects than the ADO source
- When the source has no LFS objects but the target does (a signature of `git lfs migrate` rewriting the tip commit), the Latest Commit SHA mismatch is downgraded from ❌ FAIL to ⚠️ WARN if the parent commit (last commit − 1) still matches on both sides, indicating the history is otherwise identical. The Difference column explains the downgrade. If the parent commits also differ, it remains a ❌ FAIL
- The branch policy comparison is advisory only (ℹ️ INFO) since ADO branch policies and GitHub branch protection rules are fundamentally different concepts
- Service hooks are collection-level subscriptions; the count is a best-effort match on subscriptions scoped to the repository
