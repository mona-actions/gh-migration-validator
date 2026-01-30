# GitHub Migration Validator

A GitHub CLI extension for validating GitHub organization and repository migrations by comparing key metrics between source and target repositories.

## Overview

The GitHub Migration Validator helps ensure that your migration from one GitHub organization/repository to another has been completed successfully. It compares various repository metrics (issues, pull requests, tags, releases, commits) between source and target repositories and provides a detailed validation report.

## Documentation

- **[Migration Archive Support](docs/migration-archive.md)** - Comprehensive guide for enhanced validation using GitHub migration archives

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

### Environment Variables

You can use environment variables instead of flags:

```bash
export GHMV_SOURCE_ORGANIZATION="source-org"
export GHMV_TARGET_ORGANIZATION="target-org" 
export GHMV_SOURCE_TOKEN="ghp_xxx"
export GHMV_TARGET_TOKEN="ghp_yyy"
export GHMV_SOURCE_REPO="my-repo"
export GHMV_TARGET_REPO="my-repo"
export GHMV_MARKDOWN_TABLE="true"
export GHMV_MARKDOWN_FILE="validation-report.md"

gh migration-validator
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

The tool provides both export and validation capabilities that work together to enable point-in-time migration validation:

1. **Export**: Capture repository data at a specific point in time
2. **Validate-from-Export**: Validate target repositories against exported snapshots

This workflow is particularly useful when:

- The source repository continues to receive changes during migration
- You need to validate against the exact state when migration occurred
- You want to create audit trails of migration validation

### Export Usage

```bash
gh migration-validator export \
  --github-source-org "source-org" \
  --source-repo "my-repo" \
  --github-source-pat "ghp_xxx" \
  --format json \
  --output ".exports/my-export.json"
```

### Export with Migration Archive

The tool can also download and analyze migration archives to include additional validation metrics. See the [Migration Archive Documentation](docs/migration-archive.md) for detailed information.

### Export Options

- `--github-source-org` (required): Source organization name
- `--source-repo` (required): Source repository name  
- `--github-source-pat` (required): GitHub token with read permissions
- `--source-hostname` (optional): GitHub Enterprise Server URL
- `--format` (optional): Export format - `json` or `csv` (default: `json`)
- `--output` (optional): Output file path (auto-generated if not specified)
- `--download` (optional): Download and analyze migration archive automatically
- `--download-path` (optional): Directory to download migration archives to (default: ./migration-archives)
- `--archive-path` (optional): Path to an existing extracted migration archive directory

**Note**: `--download` and `--archive-path` are mutually exclusive. For detailed migration archive usage, see [Migration Archive Documentation](docs/migration-archive.md).

### Export Output Formats

**JSON Format:**

```json
{
  "export_timestamp": "2025-10-13T14:49:08Z",
  "repository_data": {
    "owner": "source-org",
    "name": "my-repo",
    "issues": 42,
    "pull_requests": {
      "open": 5,
      "closed": 10, 
      "merged": 15,
      "total": 30
    },
    "tags": 8,
    "releases": 3,
    "commits": 150,
    "latest_commit_sha": "abc123def456",
    "branch_protection_rules": 4,
    "webhooks": 2
  },
  "migration_archive": {
    "issues": 42,
    "pull_requests": 30,
    "protected_branches" : 1,
    "releases": 3
  }
}
```

When migration archive data is included, the export will contain additional `migration_archive` metrics. See [Migration Archive Documentation](docs/migration-archive.md) for details.

**CSV Format:**

Contains the same data in CSV format with headers for easy analysis in spreadsheet applications.

### Default Export Location

When no output file is specified, exports are automatically saved to `.exports/` directory with timestamped filenames:

- `.exports/{owner}_{repo}_export_{timestamp}.{format}`

Example: `.exports/mona-actions_my-repo_export_20251002_144908.json`

## Validate-from-Export

The `validate-from-export` command allows you to validate a target repository against a previously exported snapshot of source repository data. This is essential for validating migrations when the source repository may have changed since the migration occurred.

### Validate-from-Export Usage

```bash
gh migration-validator validate-from-export \
  --export-file ".exports/mona-actions_my-repo_export_20251002_144908.json" \
  --github-target-org "target-org" \
  --target-repo "my-repo" \
  --github-target-pat "ghp_yyy"
```

### Using Existing Archive Directory

If you already have an extracted migration archive directory:

```bash
gh migration-validator export \
  --github-source-org "source-org" \
  --source-repo "my-repo" \
  --github-source-pat "ghp_xxx" \
  --archive-path "path/to/extracted/migration-archive"
```

### Validate-from-Export Options

- `--export-file` (required): Path to the exported JSON file containing source data
- `--github-target-org` (required): Target organization name
- `--target-repo` (required): Target repository name
- `--github-target-pat` (required): GitHub token with read permissions for target
- `--target-hostname` (optional): GitHub Enterprise Server URL for target
- `--markdown-table` (optional): Output results in markdown format
- `--markdown-file` (optional): Write markdown output to the specified file; uses the same content without the surrounding ```markdown fences

### Environment Variables for Validate-from-Export

```bash
export GHMV_TARGET_ORGANIZATION="target-org"
export GHMV_TARGET_TOKEN="ghp_yyy"
export GHMV_TARGET_REPO="my-repo"
export GHMV_MARKDOWN_TABLE="true"
export GHMV_MARKDOWN_FILE="validation-report.md"

gh migration-validator validate-from-export --export-file "path/to/export.json"
```

### Complete Export and Validation Workflow

1. **Export source data before migration:**

   ```bash
   gh migration-validator export \
     --github-source-org "source-org" \
     --source-repo "my-repo" \
     --github-source-pat "ghp_xxx"
   ```

2. **Perform your migration** (using GitHub's migration tools)

3. **Validate against the export:**

   ```bash
   gh migration-validator validate-from-export \
     --export-file ".exports/source-org_my-repo_export_20251002_144908.json" \
     --github-target-org "target-org" \
     --target-repo "my-repo" \
     --github-target-pat "ghp_yyy"
   ```

This ensures you're validating against the exact state of the source repository when the migration occurred, regardless of any subsequent changes.

## Migration Archive Support

The tool supports working with GitHub migration archives for enhanced validation capabilities. Migration archives provide three-way validation comparing Source API ↔ Archive ↔ Target API data.

For comprehensive documentation on migration archive features, workflow, and usage examples, see [Migration Archive Documentation](docs/migration-archive.md).

## What Gets Validated

The tool compares the following metrics between source and target repositories:

- **Issues**: Total count (expects +1 in target for migration log issue)
- **Pull Requests**: Total, Open, Merged, and Closed counts
- **Tags**: Total count of Git tags
- **Releases**: Total count of GitHub releases
- **Commits**: Total commit count on default branch
- **Branch Protection Rules**: Total count of branch protection rules configured for the repository
- **Webhooks**: Total count of active repository webhooks
- **LFS Objects**: Total count of Git LFS (Large File Storage) objects referenced in the repository
- **Latest Commit SHA**: Ensures both repositories have the same latest commit in default branch

## Validation Results

- ✅ **PASS**: Metrics match expected values
- ❌ **FAIL**: Target is missing data from source
- ⚠️ **WARN**: Target has more data than source (usually acceptable)

## Output Formats

### Console Output

The tool provides a formatted table with colored status indicators and a summary.

Example:

```markdown
# 🔄 Source vs Target Validation

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
```



# 📦 Migration Archive vs Source Validation

Metric                               | Status  | Source API Value | Archive Value | Difference   
Archive vs Source Issues             | ❌ FAIL | 2                | 6             | Missing: 4   
Archive vs Source Pull Requests      | ✅ PASS | 29               | 29            | Perfect match
Archive vs Source Protected Branches | ✅ PASS | 1                | 1             | Perfect match
Archive vs Source Releases           | ✅ PASS | 25               | 25            | Perfect match



# 🎯 Migration Archive vs Target Validation

Metric                                                   | Status  | Archive Value          | Target Value | Difference   
Archive vs Target Issues (expected +1 for migration log) | ✅ PASS | 6 (expected target: 7) | 7            | Perfect match
Archive vs Target Pull Requests                          | ✅ PASS | 29                     | 29           | Perfect match
Archive vs Target Protected Branches                     | ✅ PASS | 1                      | 1            | Perfect match
Archive vs Target Releases                               | ✅ PASS | 25                     | 25           | Perfect match


📊 Passed: 16
📊 Failed: 1
📊 Warnings: 1


ERROR   ❌ Migration validation FAILED - Some data is missing in target
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
