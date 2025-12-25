# FlakeGuard API Documentation

REST API for programmatic access to FlakeGuard.

## Authentication

All API requests require authentication using an API key.

### Bearer Token Authentication

Include your API key in the `Authorization` header:

```
Authorization: Bearer <your-api-key>
```

### Getting an API Key

1. Log in to FlakeGuard dashboard
2. Navigate to your project settings
3. Click "API Keys" tab
4. Click "Create API Key"
5. Copy the key (it will only be shown once)
6. Store it securely (e.g., GitHub Secrets)

## Endpoints

### POST /api/v1/ingest/junit

Upload JUnit XML test results for flake detection.

**Authentication**: Required (API key)

**Request**: `multipart/form-data`

**Form Parameters**:

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project_slug` | string | Yes | Project identifier from FlakeGuard dashboard |
| `files` | file[] | Yes | One or more JUnit XML files (max 1MB each, 5MB total) |
| `repository` | string | No | Repository name (e.g., `owner/repo`) |
| `commit_sha` | string | No | Git commit SHA |
| `branch` | string | No | Git branch name |
| `run_id` | string | No | CI run identifier |
| `run_attempt` | string | No | CI run attempt number (for retries) |
| `workflow_name` | string | No | CI workflow name |
| `job_name` | string | No | CI job name |
| `job_variant` | string | No | Job variant identifier (e.g., `python-3.9`, `ubuntu-latest`) |

**Response**: `200 OK`

```json
{
  "files_ingested": 3,
  "tests_total": 142,
  "tests_passed": 140,
  "tests_failed": 2,
  "tests_skipped": 0,
  "flakes_detected": 1,
  "message": "Ingested 3 JUnit files with 142 tests"
}
```

**Error Responses**:

- `400 Bad Request`: Invalid request (missing fields, invalid XML)
  ```json
  {
    "error": "project_slug is required"
  }
  ```

- `401 Unauthorized`: Invalid or missing API key
  ```json
  {
    "error": "Invalid API key"
  }
  ```

- `404 Not Found`: Project not found
  ```json
  {
    "error": "Project not found: my-project"
  }
  ```

- `413 Payload Too Large`: Upload exceeds size limits
  ```json
  {
    "error": "Total upload size exceeds limit (5MB)"
  }
  ```

- `429 Too Many Requests`: Rate limit exceeded
  ```json
  {
    "error": "Rate limit exceeded. Try again in 60 seconds."
  }
  ```

**Example with curl**:

```bash
curl -X POST https://flakeguard.example.com/api/v1/ingest/junit \
  -H "Authorization: Bearer your-api-key-here" \
  -F "project_slug=my-project" \
  -F "repository=owner/repo" \
  -F "commit_sha=abc123def456" \
  -F "branch=main" \
  -F "files=@test-results/junit.xml" \
  -F "files=@test-results/integration.xml"
```

### GET /api/v1/projects/{id}/flakes

Retrieve flaky tests for a project.

**Authentication**: Required (API key)

**Path Parameters**:

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | integer | Project ID from FlakeGuard dashboard |

**Query Parameters**:

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `status` | string | `active` | Filter by status: `active`, `resolved`, `all` |
| `min_flake_rate` | float | `0.0` | Minimum flake rate (0.0-1.0) |
| `limit` | integer | `50` | Maximum results to return (1-1000) |
| `offset` | integer | `0` | Pagination offset |

**Response**: `200 OK`

```json
{
  "flakes": [
    {
      "id": 123,
      "test_name": "test_user_login",
      "test_class": "UserAuthTests",
      "test_file": "tests/auth/test_user.py",
      "flake_rate": 0.15,
      "total_runs": 100,
      "flake_count": 15,
      "first_seen": "2024-01-15T10:30:00Z",
      "last_seen": "2024-01-20T14:45:00Z",
      "status": "active",
      "patterns": [
        "Intermittent timeout connecting to database",
        "Race condition in user session creation"
      ]
    }
  ],
  "total": 1,
  "limit": 50,
  "offset": 0
}
```

**Error Responses**:

- `401 Unauthorized`: Invalid or missing API key
- `403 Forbidden`: API key does not have access to this project
- `404 Not Found`: Project not found

**Example with curl**:

```bash
# Get all active flakes
curl https://flakeguard.example.com/api/v1/projects/42/flakes \
  -H "Authorization: Bearer your-api-key-here"

# Get flakes with rate >= 10%
curl "https://flakeguard.example.com/api/v1/projects/42/flakes?min_flake_rate=0.1" \
  -H "Authorization: Bearer your-api-key-here"

# Get all flakes (active and resolved)
curl "https://flakeguard.example.com/api/v1/projects/42/flakes?status=all" \
  -H "Authorization: Bearer your-api-key-here"

# Pagination
curl "https://flakeguard.example.com/api/v1/projects/42/flakes?limit=20&offset=40" \
  -H "Authorization: Bearer your-api-key-here"
```

## Rate Limits

Default rate limit: **120 requests per minute** per API key.

Rate limit headers included in all responses:

```
X-RateLimit-Limit: 120
X-RateLimit-Remaining: 115
X-RateLimit-Reset: 1642345678
```

When rate limited, retry after the time indicated in `X-RateLimit-Reset` (Unix timestamp).

## Error Handling

All error responses follow this format:

```json
{
  "error": "Human-readable error message",
  "code": "ERROR_CODE",
  "details": {
    "field": "Additional context"
  }
}
```

Common HTTP status codes:

- `200 OK`: Request succeeded
- `400 Bad Request`: Invalid request parameters
- `401 Unauthorized`: Missing or invalid API key
- `403 Forbidden`: API key lacks permission
- `404 Not Found`: Resource not found
- `413 Payload Too Large`: Upload too large
- `429 Too Many Requests`: Rate limit exceeded
- `500 Internal Server Error`: Server error (contact support)

## Testing the API

### Test JUnit Upload

```bash
# Create a test JUnit file
cat > test-junit.xml << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<testsuites>
  <testsuite name="ExampleTests" tests="2" failures="0" errors="0">
    <testcase name="test_example_1" classname="ExampleTests" time="0.123"/>
    <testcase name="test_example_2" classname="ExampleTests" time="0.456"/>
  </testsuite>
</testsuites>
EOF

# Upload to FlakeGuard
curl -X POST https://flakeguard.example.com/api/v1/ingest/junit \
  -H "Authorization: Bearer your-api-key-here" \
  -F "project_slug=my-project" \
  -F "files=@test-junit.xml" \
  -F "branch=test-branch" \
  -F "commit_sha=test123"
```

### Test Flake Retrieval

```bash
# Get flakes for project ID 42
curl https://flakeguard.example.com/api/v1/projects/42/flakes \
  -H "Authorization: Bearer your-api-key-here" \
  | jq .
```

## SDK and Libraries

Currently, FlakeGuard provides:

- **GitHub Action**: Automated JUnit upload from GitHub Actions workflows
- **REST API**: Direct API access for custom integrations

Community SDKs are welcome! See [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines.

## Support

- Documentation: [docs/](.)
- GitHub Issues: Report bugs and request features
- Runbook: [docs/runbook.md](./runbook.md) for operational guidance
