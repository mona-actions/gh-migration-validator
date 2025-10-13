# Migration Archive Support

The GitHub Migration Validator supports working with GitHub migration archives to provide additional validation capabilities. Migration archives contain the actual data that was migrated and serve as an authoritative source for validation.

## Overview

Migration archives provide a comprehensive way to validate migrations by comparing three data sources:

- **Source API**: Live data from the source repository
- **Migration Archive**: The actual data that was migrated  
- **Target API**: Live data from the target repository

This three-way comparison ensures complete validation coverage and helps identify where any discrepancies occurred during the migration process.

## Features

- **Automatic Download**: Automatically find and download migration archives for a repository
- **Archive Analysis**: Extract and count key entities from migration archive JSON files
- **Three-way Validation**: Compare Source API ↔ Archive ↔ Target API for comprehensive validation
- **Interactive Selection**: Choose from multiple available migration archives for a repository

## Export with Migration Archive

### Automatic Archive Download

Download and analyze migration archives automatically during export:

```bash
gh migration-validator export \
  --source-organization "source-org" \
  --source-repo "my-repo" \
  --source-token "ghp_xxx" \
  --download-archive
```

### Using Existing Archive Directory

If you already have an extracted migration archive directory:

```bash
gh migration-validator export \
  --source-organization "source-org" \
  --source-repo "my-repo" \
  --source-token "ghp_xxx" \
  --archive-path "path/to/extracted/migration-archive"
```

### Options

- `--download-archive` (optional): Download and analyze migration archive automatically
- `--archive-path` (optional): Path to an existing extracted migration archive directory

**Note**: `--download-archive` and `--archive-path` are mutually exclusive. When using `--download-archive`, you must also provide `--source-organization`.

## Migration Archive Workflow

### 1. Find Available Migrations

The tool queries GitHub's API to find all migrations for your organization:

```text
✓ Found 3 migrations for organization: source-org
```

### 2. Select Repository Migration

Choose the specific migration for your repository (filters to only show "exported" migrations):

```text
? Select a migration for repository 'my-repo':
  ▸ Migration ID: abc123 (State: exported, Created: 2024-01-15)
    Migration ID: def456 (State: exported, Created: 2024-01-10)
```

### 3. Download and Extract

The tool downloads the migration archive with a descriptive filename and extracts it:

```text
⠸ Downloading migration archive...
✓ Downloaded: migration-my-repo-abc123.tar.gz
⠸ Extracting migration archive...
✓ Archive extracted to: /tmp/migration-abc123
```

### 4. Analyze Content

Parse JSON files in the archive to count entities:

```text
⠸ Analyzing migration archive...
✓ Migration archive analysis complete
  • Issues: 42
  • Pull Requests: 30  
  • Releases: 3
  • Organizations: 1
  • Repositories: 1
  • Users: 25
```

### 5. Enhanced Export

The export file includes both API data and migration archive metrics:

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

## Validation with Migration Archives

When validating from an export that contains migration archive data, the tool performs comprehensive three-way validation.

### Three-way Validation Process

1. **Source vs Archive**: Ensures the migration archive contains all expected data from the source
2. **Archive vs Target**: Validates that the target repository matches the migrated data

### Validation Example

```bash
gh migration-validator validate-from-export \
  --export-file ".exports/source-org_my-repo_export_20251013_144908.json" \
  --target-organization "target-org" \
  --target-repo "my-repo" \
  --target-token "ghp_yyy"
```

### Enhanced Validation Output

The validation tables clearly indicate which comparison is being made:

```text
Source vs Archive Validation:
┌─────────────────────────┬──────────────┬──────────────┬────────┐
│ Metric                  │ Source Value │ Archive Value│ Status │
├─────────────────────────┼──────────────┼──────────────┼────────┤
│ Issues                  │ 42           │ 42           │ ✅ PASS│
│ Pull Requests (Total)   │ 30           │ 30           │ ✅ PASS│
│ Releases                │ 3            │ 3            │ ✅ PASS│
└─────────────────────────┴──────────────┴──────────────┴────────┘

Archive vs Target Validation:
┌─────────────────────────┬──────────────┬──────────────┬────────┐
│ Metric                  │ Archive Value│ Target Value │ Status │
├─────────────────────────┼──────────────┼──────────────┼────────┤
│ Issues                  │ 42           │ 43           │ ⚠️ WARN│
│ Pull Requests (Total)   │ 30           │ 30           │ ✅ PASS│
│ Releases                │ 3            │ 3            │ ✅ PASS│
└─────────────────────────┴──────────────┴──────────────┴────────┘
```

## Archive Storage

### File Naming

Downloaded migration archives are stored with descriptive names:

- Format: `migration-{repo-name}-{migration-id}.tar.gz`
- Location: Current working directory (configurable)
- Extraction: Automatically extracted to temporary directories for analysis

### Examples

```text
migration-my-repo-abc123def456.tar.gz
migration-webapp-789xyz123456.tar.gz
migration-api-service-456def789abc.tar.gz
```

## Archive Data Analysis

### Supported Files

The tool analyzes the following files in migration archives:

- `issues_*.json` - Issue data
- `pull_requests_*.json` - Pull request data  
- `releases_*.json` - Release data
- `protected_branches_*.json` - Protected branches data

### Multi-file Support

Migration archives may contain data across multiple numbered JSON files:

- `issues_000001.json`
- `issues_000002.json`
- `pull_requests_000001.json`
- `pull_requests_000002.json`

The tool automatically processes all numbered files for each entity type and aggregates the counts.

### Analysis Process

1. **Scan Directory**: Find all relevant JSON files in the archive
2. **Parse Files**: Read and parse each JSON file
3. **Count Entities**: Count array elements in each file
4. **Aggregate**: Sum counts across all files of the same type
5. **Report**: Display final counts for each entity type

## Benefits

### Comprehensive Validation

Three-way validation provides confidence that:

- The migration archive captured all source data correctly
- The target repository contains all migrated data  
- Any discrepancies can be traced to their source

### Point-in-time Accuracy

Migration archives represent the exact data state at migration time, ensuring validation accuracy even if source repositories continue to change.

### Audit Trail

The combination of export files and migration archives provides a complete audit trail of the migration process and validation results.

## Troubleshooting

### Common Issues

1. **No migrations found**: Ensure your token has the necessary permissions and the organization has completed migrations
2. **Archive download fails**: Check network connectivity and ensure the migration is in "exported" state
3. **Archive analysis errors**: Verify the archive extracted correctly and contains expected JSON files

### Migration States

The tool only shows migrations in "exported" state, which indicates they are ready for download. Other states include:

### Permissions Required

To use migration archive features, your GitHub token needs:

- `read:org` - To list organization migrations
- `repo` - To access repository data for comparison
