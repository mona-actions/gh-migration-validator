# Bitbucket Validation

The `bitbucket` subcommand validates migrations from Bitbucket (Server / Data Center) to GitHub by comparing API metrics between the source Bitbucket instance and the target GitHub repository. This is useful for verifying that repository data was migrated correctly when moving from Bitbucket to GitHub.

## Usage

```bash
gh migration-validator bitbucket \
  --bbs-server-url "https://bitbucket.example.com" \
  --bbs-project "PROJ" \
  --bbs-repo "my-repo" \
  --bbs-token "your-bbs-token" \
  --github-target-org "target-org" \
  --target-repo "my-repo" \
  --github-target-pat "ghp_yyy"
```

## Environment Variables

```bash
export GHMV_BBS_SERVER_URL="https://bitbucket.example.com"
export GHMV_BBS_PROJECT="PROJ"
export GHMV_BBS_REPO="my-repo"
export GHMV_BBS_TOKEN="your-bbs-token"
export GHMV_TARGET_ORGANIZATION="target-org"
export GHMV_TARGET_TOKEN="ghp_yyy"
export GHMV_TARGET_REPO="my-repo"
gh migration-validator bitbucket
```

## Options

### Bitbucket Source Flags

- `--bbs-server-url` / `-H` (required): Bitbucket Server URL (aligned with [GEI `bbs2gh`](https://docs.github.com/en/migrations/using-github-enterprise-importer/migrating-from-bitbucket-server-to-github-enterprise-cloud/migrating-repositories-from-bitbucket-server-to-github-enterprise-cloud))
- `--bbs-project` / `-p` (required): Project key (use `~username` for personal repos)
- `--bbs-repo` / `-r` (required): Repository slug
- `--bbs-token` / `-k` (required): Personal access token (or `GHMV_BBS_TOKEN`)

### Shared Target Flags (inherited from root)

- `--github-target-org` / `-t` (required): Target GitHub organization
- `--github-target-pat` / `-b` (required): Target GitHub token (or `GHMV_TARGET_TOKEN`)
- `--target-repo` (required): Target repository name
- `--target-hostname` / `-v`: GitHub Enterprise Server URL (optional)
- `--markdown-table` / `-m`: Output results in markdown format
- `--markdown-file`: Write markdown output to the specified file
- `--no-lfs`: Skip LFS object validation
- `--strict-exit`: Exit with status 2 on validation failures

## What Gets Validated (Bitbucket → GitHub)

| Metric                                               | Status      | Notes                                         |
| ---------------------------------------------------- | ----------- | --------------------------------------------- |
| Pull Requests (Total, Open, Merged, Declined→Closed) | ✅ Compared | Bitbucket "Declined" maps to GitHub "Closed"  |
| Tags                                                 | ✅ Compared |                                               |
| Commits                                              | ✅ Compared | Default branch only                           |
| Latest Commit SHA                                    | ✅ Compared |                                               |
| Branch Permissions vs Branch Protection Rules        | ℹ️ Advisory | Different concepts — shown for reference only |
| Webhooks                                             | ✅ Compared |                                               |
| Issues                                               | ⏭️ Skipped  | Bitbucket uses Jira, not native issues        |
| Releases                                             | ⏭️ Skipped  | Bitbucket has no equivalent                   |
| LFS Objects                                          | ⏭️ Skipped  | TODO                                          |

## Notes

- Requires Bitbucket Server 5.5+ (uses Bearer token PAT authentication)
- The branch permissions comparison is advisory only (ℹ️ INFO) since Bitbucket branch permissions and GitHub branch protection rules are fundamentally different concepts
