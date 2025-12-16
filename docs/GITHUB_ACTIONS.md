# GitHub Actions Setup

This guide explains how to use Destill with GitHub Actions workflow runs.

## Authentication

Destill uses the GitHub REST API to fetch workflow runs, jobs, logs, and artifacts. You'll need a Personal Access Token (PAT) with appropriate permissions.

### Creating a Personal Access Token

1. Go to GitHub Settings → Developer settings → Personal access tokens → Tokens (classic)
2. Click "Generate new token (classic)"
3. Give your token a descriptive name (e.g., "Destill CLI")
4. Select the following scopes:
   - `repo` - Full control of private repositories
     - This includes access to Actions runs, workflows, and logs
5. Click "Generate token"
6. Copy the token (it starts with `ghp_`)

### Setting the Token

```bash
export GITHUB_TOKEN="ghp_your_token_here"
```

To make this permanent, add it to your shell profile:

```bash
# Add to ~/.bashrc or ~/.zshrc
echo 'export GITHUB_TOKEN="ghp_your_token_here"' >> ~/.zshrc
source ~/.zshrc
```

## Usage

### Analyze a Workflow Run

```bash
destill analyze "https://github.com/owner/repo/actions/runs/123456"
```

The URL can be copied directly from the Actions tab in your GitHub repository.

### Finding Workflow Run URLs

1. Navigate to your repository on GitHub
2. Click the "Actions" tab
3. Click on any workflow run
4. Copy the URL from your browser (format: `https://github.com/owner/repo/actions/runs/RUN_ID`)

## How It Works

When you analyze a GitHub Actions workflow run, Destill:

1. **Fetches workflow metadata** - Uses the GitHub API to get the run details
2. **Retrieves all jobs** - Gets the list of jobs that ran in the workflow
3. **Downloads job logs** - Fetches logs for each job (GitHub provides logs as text)
4. **Analyzes logs** - Runs pattern-based detection to find errors and failures
5. **Boosts confidence** - Errors from failed jobs get higher confidence scores
6. **Displays findings** - Shows results in an interactive TUI, sorted by confidence

## Differences from Buildkite

GitHub Actions has some architectural differences compared to Buildkite:

| Feature | Buildkite | GitHub Actions |
|---------|-----------|----------------|
| **Log format** | Plain text streams | Plain text (accessed via redirect) |
| **Job types** | Distinguishes script/command jobs | All jobs treated as "script" type |
| **Artifacts** | Per-job artifacts | Per-run artifacts (shared across jobs) |
| **API authentication** | API token only | Personal Access Token (PAT) |
| **Rate limiting** | Generous | 5,000 requests/hour (authenticated) |

## Supported Features

- ✅ Workflow run metadata (status, conclusion, timestamps)
- ✅ Job-level details (name, status, conclusion, steps)
- ✅ Full job logs with pattern-based analysis
- ✅ Failed job detection with confidence boosting
- ✅ Interactive TUI with findings sorted by confidence
- ✅ Local mode (no infrastructure required)
- ✅ Distributed mode (with Redpanda + Postgres)

## Limitations

1. **Artifact access** - GitHub artifacts are per-run, not per-job. Destill associates artifacts with all jobs in the run.
2. **Log retention** - GitHub retains logs for 90 days. Older runs cannot be analyzed.
3. **Rate limits** - GitHub API has rate limits (5,000 requests/hour for authenticated users). For high-volume usage, consider caching or distributed mode.

## Troubleshooting

### "Authentication failed"

**Problem**: Your GitHub token is invalid or missing.

**Solution**:
```bash
# Check if token is set
echo $GITHUB_TOKEN

# Regenerate token if needed (see "Creating a Personal Access Token" above)
export GITHUB_TOKEN="ghp_your_new_token"
```

### "Build not found" (404 error)

**Problem**: The workflow run doesn't exist or you don't have access.

**Solution**:
- Verify the URL is correct
- Check that your token has access to the repository
- For private repos, ensure your PAT has `repo` scope

### "Rate limited"

**Problem**: You've exceeded GitHub's API rate limit.

**Solution**:
- Wait for the rate limit to reset (shown in error message)
- Use distributed mode to cache results in Postgres
- Consider using a GitHub App instead of PAT for higher limits

## Example Workflow Configuration

Here's a complete example of a workflow that works well with Destill:

```yaml
name: CI

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Run tests
        run: go test -v ./... -coverprofile=coverage.out
```

Then analyze with:

```bash
destill analyze "https://github.com/yourusername/yourrepo/actions/runs/123456"
```

## Best Practices

1. **Descriptive job names** - Makes it easier to identify issues in the TUI
2. **Structure your logs** - Use clear error messages and structured output
3. **Keep token secure** - Never commit your GitHub token to source control

## Next Steps

- See [MCP_INTEGRATION.md](./MCP_INTEGRATION.md) to use Destill with Claude Desktop/Code
- Read the main [README.md](../README.md) for more features and distributed mode setup
- Check [ARCHITECTURE.md](../ARCHITECTURE.md) to understand how Destill works
