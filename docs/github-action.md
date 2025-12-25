# FlakeGuard GitHub Action

Upload JUnit test results to FlakeGuard for automated flake detection.

## Overview

The FlakeGuard GitHub Action uploads JUnit XML test results to your FlakeGuard instance during CI runs. It automatically extracts GitHub context (repository, commit, branch, workflow, job) and associates test results with your project.

## Quick Start

Add this step to your GitHub Actions workflow after running tests:

```yaml
- name: Upload test results to FlakeGuard
  uses: ./action  # Or your-org/flakeguard-action@v1 when published
  if: always()  # Upload even if tests fail
  with:
    flakeguard_url: 'https://flakeguard.example.com'
    project_slug: 'my-project'
    api_key: ${{ secrets.FLAKEGUARD_API_KEY }}
    junit_paths: 'test-results/**/*.xml'
```

## Inputs

| Input | Description | Required | Default |
|-------|-------------|----------|---------|
| `flakeguard_url` | FlakeGuard server URL (e.g., `https://flakeguard.example.com`) | Yes | - |
| `project_slug` | Project slug from FlakeGuard dashboard | Yes | - |
| `api_key` | FlakeGuard API key (use `secrets.FLAKEGUARD_API_KEY`) | Yes | - |
| `junit_paths` | Glob pattern for JUnit XML files (e.g., `test-results/**/*.xml`) | Yes | - |
| `job_variant` | Optional job variant identifier (e.g., `python-3.9`, `ubuntu-latest`) | No | `''` |

## Examples

### Basic Workflow

```yaml
name: Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Run tests
        run: |
          npm test -- --reporter=junit --outputFile=test-results/junit.xml

      - name: Upload to FlakeGuard
        uses: ./action
        if: always()  # Upload even if tests fail
        with:
          flakeguard_url: 'https://flakeguard.example.com'
          project_slug: 'my-project'
          api_key: ${{ secrets.FLAKEGUARD_API_KEY }}
          junit_paths: 'test-results/**/*.xml'
```

### Matrix Builds with Job Variants

Use `job_variant` to distinguish results from different matrix combinations:

```yaml
name: Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, windows-latest, macos-latest]
        python-version: ['3.9', '3.10', '3.11']

    steps:
      - uses: actions/checkout@v4

      - name: Set up Python
        uses: actions/setup-python@v4
        with:
          python-version: ${{ matrix.python-version }}

      - name: Run tests
        run: |
          pytest --junit-xml=test-results/junit.xml

      - name: Upload to FlakeGuard
        uses: ./action
        if: always()
        with:
          flakeguard_url: 'https://flakeguard.example.com'
          project_slug: 'my-project'
          api_key: ${{ secrets.FLAKEGUARD_API_KEY }}
          junit_paths: 'test-results/**/*.xml'
          job_variant: '${{ matrix.os }}-python-${{ matrix.python-version }}'
```

### Multiple Test Suites

Upload results from multiple test directories:

```yaml
- name: Upload unit test results
  uses: ./action
  if: always()
  with:
    flakeguard_url: 'https://flakeguard.example.com'
    project_slug: 'my-project'
    api_key: ${{ secrets.FLAKEGUARD_API_KEY }}
    junit_paths: 'test-results/unit/**/*.xml'
    job_variant: 'unit-tests'

- name: Upload integration test results
  uses: ./action
  if: always()
  with:
    flakeguard_url: 'https://flakeguard.example.com'
    project_slug: 'my-project'
    api_key: ${{ secrets.FLAKEGUARD_API_KEY }}
    junit_paths: 'test-results/integration/**/*.xml'
    job_variant: 'integration-tests'
```

## Metadata Captured

The action automatically extracts the following metadata from GitHub Actions context:

- **Repository**: `GITHUB_REPOSITORY` (e.g., `owner/repo`)
- **Commit SHA**: `GITHUB_SHA`
- **Branch**: Extracted from `GITHUB_REF` (handles branches, PRs, tags)
- **Workflow Name**: `GITHUB_WORKFLOW`
- **Job Name**: `GITHUB_JOB`
- **Run ID**: `GITHUB_RUN_ID`
- **Run Attempt**: `GITHUB_RUN_ATTEMPT`
- **Job Variant**: User-provided input (optional)

## Troubleshooting

### No files found matching pattern

**Symptom**: Warning message "No JUnit files found matching pattern"

**Solution**: This is not an error - the action exits successfully (exit 0) and continues your workflow. Check:

1. Verify your test framework generates JUnit XML output
2. Check the file path in `junit_paths` matches where tests write results
3. Ensure tests ran before the upload step
4. Try listing files: `run: ls -la test-results/` before upload

### Upload fails with 401 Unauthorized

**Symptom**: Error "Upload failed with HTTP status 401"

**Solution**:

1. Verify `FLAKEGUARD_API_KEY` secret is set in repository settings
2. Check the API key has not expired in FlakeGuard dashboard
3. Ensure you're using the correct project slug

### Upload fails with 404 Not Found

**Symptom**: Error "Upload failed with HTTP status 404"

**Solution**:

1. Verify `flakeguard_url` is correct (include protocol: `https://`)
2. Check the FlakeGuard server is accessible from GitHub Actions
3. Verify the project slug exists in FlakeGuard

### Upload fails with 413 Payload Too Large

**Symptom**: Error "Upload failed with HTTP status 413"

**Solution**:

1. Check total JUnit file size (default limit: 5MB)
2. Check individual file size (default limit: 1MB)
3. Contact FlakeGuard admin to adjust limits: `FG_MAX_UPLOAD_BYTES`, `FG_MAX_FILE_BYTES`

### Connection timeout

**Symptom**: Error "curl command failed"

**Solution**:

1. Verify FlakeGuard server is accessible from GitHub Actions runners
2. Check firewall/network settings
3. Try accessing FlakeGuard URL manually: `curl https://flakeguard.example.com`

## Best Practices

1. **Always use `if: always()`**: Upload results even if tests fail - flakes often cause failures
2. **Use secrets for API key**: Never hardcode API keys - use `${{ secrets.FLAKEGUARD_API_KEY }}`
3. **Use job variants for matrix builds**: Distinguish results from different configurations
4. **Upload after each test suite**: For multiple test types, upload separately with different variants
5. **Check test output paths**: Verify your test framework writes to the expected location

## Security

- API keys are marked as sensitive and will not appear in GitHub Actions logs
- Use GitHub Secrets to store your FlakeGuard API key
- API keys are sent via HTTPS with Authorization Bearer header

## Next Steps

- [API Documentation](./api.md) - Learn about FlakeGuard API endpoints
- [Runbook](./runbook.md) - Operational guide for FlakeGuard
- [Complete Workflow Example](../examples/github-workflow.yml) - Full working example
