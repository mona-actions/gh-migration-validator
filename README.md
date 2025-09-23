# gh-migration-validator

`gh-migration-validator` is a GitHub CLI extension for validating GitHub organization migrations. It helps ensure that your migration from one GitHub organization to another has been completed successfully by comparing repositories other resources between source and target organizations.

## Install

```bash
gh extension install mona-actions/gh-migration-validator
```

## Dependencies

- [Go](https://golang.org/doc/install) 1.20 or higher
- Key dependencies:
  - [Cobra](https://github.com/spf13/cobra) - CLI framework for command line applications
  - [Viper](https://github.com/spf13/viper) - Configuration management with environment variable support
  - [go-github](https://github.com/google/go-github) - GitHub REST API v3 client
  - [githubv4](https://github.com/shurcooL/githubv4) - GitHub GraphQL API v4 client
  - [go-githubauth](https://github.com/jferrl/go-githubauth) - GitHub App authentication
  - [go-github-ratelimit](https://github.com/gofri/go-github-ratelimit) - Rate limit handling

## Features

- Pre-configured GitHub API clients:
  - REST API client using [`go-github`](https://github.com/google/go-github)
  - GraphQL API client with rate limit handling
  - Support for both Personal Access Token and GitHub App authentication
  - Enterprise Server support via hostname configuration

- Common CLI flags:
  - Source/target organization flags
  - Token authentication flags
  - Enterprise hostname support
  - All flags support environment variable configuration

- Built-in release management:
  - Automated versioning using Release Drafter
  - Version bumping based on PR labels
  - Automated changelog generation
  - Pre-compiled extension binaries

### Environment variables

GitHub App authentication in this migration validator is not handled by flags, but by environment variables. You can set them in your shell or in a `.env` file.
This can be quickly changed to add flags for the app ID, private key, and installation ID.

It's recommended to use the prefix set in the `viper` configuration, which is `GHMV_` (GitHub Migration Validator), to avoid conflicts with other environment variables.

```sh
# Required for GitHub App auth
export GHMV_SOURCE_APP_ID="123456"
export GHMV_SOURCE_PRIVATE_KEY="-----BEGIN RSA -----\n..."
export GHMV_SOURCE_INSTALLATION_ID="987654"

# Optional Enterprise Server URL
export GHMV_SOURCE_HOSTNAME="https://github.example.com"
```

## License

- [MIT](./license) (c) [Mona-Actions](https://github.com/mona-actions)
- [Contributing](./contributing.md)
