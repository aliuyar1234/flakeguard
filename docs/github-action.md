# FlakeGuard GitHub Action

Upload JUnit XML test results from GitHub Actions to FlakeGuard for flake detection.

## Quick Start

Add this step to your GitHub Actions workflow after running tests:

```yaml
- name: Upload test results to FlakeGuard
  uses: aliuyar1234/flakeguard/action@v1
  if: always() # Upload even if tests fail
  with:
    flakeguard_url: 'https://flakeguard.example.com'
    project_slug: 'my-project'
    api_key: ${{ secrets.FLAKEGUARD_API_KEY }}
    junit_paths: 'test-results/**/*.xml'
```

## Versioning strategy

Recommended pinning options:

- Stable major line: `uses: aliuyar1234/flakeguard/action@v1`
- Specific release: `uses: aliuyar1234/flakeguard/action@v1.2.3`
- Latest (not recommended for production): `uses: aliuyar1234/flakeguard/action@main`

To publish new versions:

- Create immutable tags like `v1.2.3`.
- Move the floating major tag `v1` to the latest `v1.x.y`.

## Inputs

| Input | Description | Required | Default |
|-------|-------------|----------|---------|
| `flakeguard_url` | Base URL of your FlakeGuard instance | Yes | - |
| `project_slug` | Project slug from FlakeGuard | Yes | - |
| `api_key` | Project API key (store in GitHub Secrets) | Yes | - |
| `junit_paths` | Glob for JUnit XML files | Yes | - |
| `job_variant` | Optional variant label (matrix builds, OS, runtime) | No | `''` |

## Examples

### Matrix Builds with `job_variant`

```yaml
jobs:
  test:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, windows-latest, macos-latest]
        python: ['3.11', '3.12']
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-python@v5
        with:
          python-version: ${{ matrix.python }}
      - run: pytest --junit-xml=test-results/junit.xml

      - name: Upload to FlakeGuard
        uses: aliuyar1234/flakeguard/action@v1
        if: always()
        with:
          flakeguard_url: 'https://flakeguard.example.com'
          project_slug: 'my-project'
          api_key: ${{ secrets.FLAKEGUARD_API_KEY }}
          junit_paths: 'test-results/**/*.xml'
          job_variant: '${{ matrix.os }}-python-${{ matrix.python }}'
```

## What It Uploads

The action sends a multipart request to `POST /api/v1/ingest/junit`:

- `meta` (application/json): run metadata derived from the GitHub Actions context
- `junit` (file): one part per matched JUnit XML file

Metadata includes:

- `project_slug`
- `repo_full_name`
- `workflow_name`, `workflow_ref`
- `github_run_id`, `github_run_attempt`, `github_run_number`, `run_url`
- `sha`, `branch`, `event`, `pr_number`
- `job_name`, `job_variant`
- `started_at`, `completed_at`

## Behavior Notes

- If no files match `junit_paths`, the action logs a warning and exits `0` (does not fail your workflow).
- The API key is masked via `::add-mask::` in `action.yml` and is never printed by the upload script.

## Troubleshooting

### 400 `invalid_meta`

- Ensure `project_slug` matches the project behind your API key.
- Verify the workflow is running on GitHub Actions (the action requires standard `GITHUB_*` context variables).

### 401 Unauthorized

- Ensure the secret contains a valid project API key with scope `ingest:write`.
- Ensure the key is not revoked.

### 413 Payload Too Large

- Reduce the size/number of JUnit files being uploaded, or increase limits on the FlakeGuard server (`FG_MAX_UPLOAD_BYTES`, `FG_MAX_UPLOAD_FILES`, `FG_MAX_FILE_BYTES`).

### Network Errors

- Verify `flakeguard_url` is reachable from GitHub-hosted runners.
- Ensure you include the protocol (`https://`).
