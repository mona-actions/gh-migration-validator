# Export and Validation Workflow

The tool provides both export and validation capabilities that work together to enable point-in-time migration validation:

1. **Export**: Capture repository data at a specific point in time
2. **Validate-from-Export**: Validate target repositories against exported snapshots

This workflow is particularly useful when:

- The source repository continues to receive changes during migration
- You need to validate against the exact state when migration occurred
- You want to create audit trails of migration validation

## Export Usage

```bash
gh migration-validator export \
  --github-source-org "source-org" \
  --source-repo "my-repo" \
  --github-source-pat "ghp_xxx" \
  --format json \
  --output ".exports/my-export.json"
```

## Export with Migration Archive

The tool can also download and analyze migration archives to include additional validation metrics. See the [Migration Archive Documentation](migration-archive.md) for detailed information.

## Export Options

- `--github-source-org` (required): Source organization name
- `--source-repo` (required): Source repository name
- `--github-source-pat` (required): GitHub token with read permissions
- `--source-hostname` (optional): GitHub Enterprise Server URL
- `--format` (optional): Export format - `json` or `csv` (default: `json`)
- `--output` (optional): Output file path (auto-generated if not specified)
- `--download` (optional): Download and analyze migration archive automatically
- `--download-path` (optional): Directory to download migration archives to (default: ./migration-archives)
- `--archive-path` (optional): Path to an existing extracted migration archive directory
- `--no-lfs` (optional): Skip LFS object validation

**Note**: `--download` and `--archive-path` are mutually exclusive. For detailed migration archive usage, see [Migration Archive Documentation](migration-archive.md).

## Export Output Formats

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
    "protected_branches": 1,
    "releases": 3
  }
}
```

When migration archive data is included, the export will contain additional `migration_archive` metrics. See [Migration Archive Documentation](migration-archive.md) for details.

**CSV Format:**

Contains the same data in CSV format with headers for easy analysis in spreadsheet applications.

## Default Export Location

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
- `--no-lfs` (optional): Skip LFS object validation
- `--strict-exit` (optional): Exit with status 2 on validation failures

### Environment Variables for Validate-from-Export

```bash
export GHMV_TARGET_ORGANIZATION="target-org"
export GHMV_TARGET_TOKEN="ghp_yyy"
export GHMV_TARGET_REPO="my-repo"
export GHMV_MARKDOWN_TABLE="true"
export GHMV_MARKDOWN_FILE="validation-report.md"
export GHMV_NO_LFS="true"  # Optional: skip LFS validation

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
