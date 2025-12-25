# FlakeGuard Runbook

Operational guide for running and maintaining FlakeGuard.

## Table of Contents

- [API Key Management](#api-key-management)
- [Slack Webhook Configuration](#slack-webhook-configuration)
- [Retention Policy](#retention-policy)
- [Database Maintenance](#database-maintenance)
- [Troubleshooting and Diagnostics](#troubleshooting-and-diagnostics)

---

## API Key Management

### Creating API Keys

API keys are scoped to projects and can be created by project admins.

**Via Web UI**:

1. Log in to FlakeGuard dashboard
2. Navigate to Project Settings
3. Click "API Keys" tab
4. Click "Create API Key"
5. Enter a descriptive name (e.g., "GitHub Actions - Main Repo")
6. Copy the generated key (shown only once)
7. Store in CI/CD secrets (e.g., GitHub Secrets)

**Security Best Practices**:

- One API key per CI/CD system or integration
- Use descriptive names to track usage
- Rotate keys periodically (every 90 days recommended)
- Revoke unused keys immediately
- Never commit API keys to source control
- Use environment variables or secrets management

### Rotating API Keys

1. Create new API key in FlakeGuard dashboard
2. Update CI/CD secrets with new key
3. Verify new key works (check CI/CD runs)
4. Revoke old key in FlakeGuard dashboard

### Revoking API Keys

**Via Web UI**:

1. Navigate to Project Settings > API Keys
2. Find the key to revoke
3. Click "Revoke" button
4. Confirm revocation

**Effect**: Revoked keys are immediately invalid. API requests with revoked keys return `401 Unauthorized`.

### Monitoring API Key Usage

- View recent API requests in Project Activity log
- Track rate limit usage per key
- Audit API key creation/revocation in Audit Log

---

## Slack Webhook Configuration

FlakeGuard can send notifications to Slack when new flakes are detected.

### Setting Up Slack Notifications

**Step 1: Create Slack Webhook**

1. Go to [Slack App Directory](https://api.slack.com/apps)
2. Create new app or use existing app
3. Enable "Incoming Webhooks"
4. Create webhook for desired channel
5. Copy webhook URL (e.g., `https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXX`)

**Step 2: Configure Project in FlakeGuard**

1. Log in to FlakeGuard dashboard
2. Navigate to Project Settings
3. Click "Notifications" tab
4. Enter Slack webhook URL
5. Configure notification thresholds:
   - **Minimum flake rate**: Only notify if flake rate exceeds threshold (e.g., 0.10 = 10%)
   - **Cooldown period**: Minimum time between notifications for same test (e.g., 24 hours)
6. Click "Save"

**Step 3: Test Integration**

1. Click "Send Test Notification" button
2. Verify message appears in Slack channel
3. Adjust formatting/channel if needed

### Notification Format

Slack notifications include:

- **Test name**: Fully qualified test identifier
- **Flake rate**: Percentage of runs that flake (e.g., "15%")
- **Recent runs**: Pass/fail history
- **Dashboard link**: Direct link to flake details
- **Context**: Repository, branch, workflow

Example message:

```
🔴 New Flake Detected: test_user_login

Flake Rate: 15% (15 flakes in 100 runs)
Project: my-project
Test: tests/auth/test_user.py::UserAuthTests::test_user_login

Recent runs: ✅ ✅ ❌ ✅ ❌ ✅ ✅ ✅

View details: https://flakeguard.example.com/projects/42/flakes/123
```

### Troubleshooting Slack Notifications

**No notifications received**:

1. Verify webhook URL is correct in Project Settings
2. Check Slack app has permission to post to channel
3. Verify flake rate exceeds minimum threshold
4. Check cooldown period hasn't blocked notification
5. Review FlakeGuard logs for webhook errors

**Notifications delayed**:

- Webhook timeout setting: `FG_SLACK_TIMEOUT_MS` (default: 2000ms)
- FlakeGuard uses fire-and-forget for Slack webhooks (won't block ingestion)
- Check Slack API status if persistent delays

**Webhook failures**:

- FlakeGuard logs webhook failures but continues processing
- Check logs: `grep "slack webhook failed" /var/log/flakeguard.log`
- Verify webhook URL hasn't been revoked in Slack

---

## Retention Policy

FlakeGuard automatically cleans up old data to manage storage.

### What Gets Deleted

**JUnit File Content** (default: 30 days):
- Raw JUnit XML content is cleared from `junit_files.content` column
- Metadata preserved: filename, test count, upload timestamp
- **Not deleted**: Test results, flake events, statistics

**Flake Events** (default: 180 days):
- Individual test run records deleted from `flake_events` table
- **Not deleted**: Aggregated statistics in `flake_stats` table

**What's Never Deleted**:
- `flake_stats`: Aggregated flake statistics (preserved indefinitely)
- Test metadata: Test names, files, classes
- Project and organization data

### Configuring Retention Periods

Set environment variables:

```bash
# Retain JUnit content for 60 days (default: 30)
FG_RETENTION_JUNIT_DAYS=60

# Retain flake events for 365 days (default: 180)
FG_RETENTION_EVENTS_DAYS=365
```

### Retention Job Schedule

- **Production** (`FG_ENV=prod`): Runs daily at 03:00 UTC
- **Development** (`FG_ENV=dev`): Runs every minute (for testing)

### Manual Retention Execution

Retention runs automatically, but you can trigger manually:

```sql
-- Clear JUnit content older than 30 days
UPDATE junit_files
SET content = NULL
WHERE created_at < NOW() - INTERVAL '30 days'
  AND content IS NOT NULL;

-- Delete flake events older than 180 days
DELETE FROM flake_events
WHERE created_at < NOW() - INTERVAL '180 days';
```

### Monitoring Retention

Check FlakeGuard logs for retention job execution:

```bash
grep "Retention job completed" /var/log/flakeguard.log
```

Example log entry:

```
2024-01-20T03:00:15Z INFO Retention job completed: cleared 1234 junit_content, deleted 5678 events
```

---

## Database Maintenance

### Backup Recommendations

**Frequency**:
- Daily automated backups (minimum)
- Before major upgrades
- Before manual data operations

**What to backup**:
- Full PostgreSQL database dump
- Include all tables (schema + data)

**Backup command**:

```bash
pg_dump -h localhost -U flakeguard_user -d flakeguard > backup-$(date +%Y%m%d).sql
```

**Restore command**:

```bash
psql -h localhost -U flakeguard_user -d flakeguard < backup-20240120.sql
```

### Vacuum and Analyze

PostgreSQL maintenance for optimal performance:

```sql
-- Reclaim space from deleted rows (especially after retention cleanup)
VACUUM ANALYZE flake_events;
VACUUM ANALYZE junit_files;

-- Full vacuum (more aggressive, requires table lock)
VACUUM FULL flake_events;
```

**Schedule**: Run `VACUUM ANALYZE` weekly, especially after retention job runs.

### Index Maintenance

Monitor index usage and rebuild if needed:

```sql
-- Check index bloat
SELECT schemaname, tablename, indexname,
       pg_size_pretty(pg_relation_size(indexrelid)) AS index_size
FROM pg_stat_user_indexes
ORDER BY pg_relation_size(indexrelid) DESC;

-- Rebuild index if bloated
REINDEX INDEX idx_flake_events_created_at;
```

### Disk Space Monitoring

Monitor database size:

```sql
-- Database size
SELECT pg_size_pretty(pg_database_size('flakeguard'));

-- Table sizes
SELECT schemaname, tablename,
       pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS size
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;
```

Set up alerts when disk usage exceeds 80%.

### Migration Best Practices

1. Backup database before migration
2. Test migrations in staging environment
3. Run migrations during low-traffic window
4. Monitor application logs during migration
5. Have rollback plan ready

---

## Troubleshooting and Diagnostics

### Common Issues

#### High Memory Usage

**Symptoms**: FlakeGuard process using excessive RAM

**Diagnosis**:

```bash
# Check process memory
ps aux | grep flakeguard

# Check database connections
psql -c "SELECT count(*) FROM pg_stat_activity WHERE datname = 'flakeguard';"
```

**Solutions**:

1. Check for connection leaks (should match connection pool size)
2. Review slow queries: `SELECT * FROM pg_stat_statements ORDER BY total_exec_time DESC LIMIT 10;`
3. Increase connection pool size if needed (but investigate root cause first)
4. Restart FlakeGuard service: `systemctl restart flakeguard`

#### Slow JUnit Ingestion

**Symptoms**: Upload requests taking > 5 seconds

**Diagnosis**:

```bash
# Check recent ingestion performance
grep "ingestion completed" /var/log/flakeguard.log | tail -20

# Check database query performance
psql -c "SELECT query, mean_exec_time, calls FROM pg_stat_statements ORDER BY mean_exec_time DESC LIMIT 10;"
```

**Solutions**:

1. Verify database indexes exist (check migration logs)
2. Run `VACUUM ANALYZE` on `junit_files` and `flake_events` tables
3. Check disk I/O: `iostat -x 1`
4. Scale database vertically (more CPU/RAM) if consistently slow

#### Rate Limit Errors

**Symptoms**: API returns `429 Too Many Requests`

**Current limit**: 120 requests per minute per API key (default)

**Solutions**:

1. Implement retry with exponential backoff in client
2. Increase rate limit: Set `FG_RATE_LIMIT_RPM=240` in environment
3. Use separate API keys for different CI jobs
4. Batch JUnit file uploads (multiple files in one request)

#### Database Connection Errors

**Symptoms**: "Too many connections" or "Connection refused"

**Diagnosis**:

```sql
-- Check current connections
SELECT count(*) FROM pg_stat_activity;

-- Check max connections
SHOW max_connections;
```

**Solutions**:

1. Increase PostgreSQL `max_connections`: Edit `postgresql.conf`
2. Reduce connection pool size: Review `FG_DATABASE_DSN` connection params
3. Check for connection leaks: Monitor connections over time
4. Restart PostgreSQL: `systemctl restart postgresql`

### Logging

**Log levels** (set via `FG_LOG_LEVEL`):

- `debug`: Verbose logging (use in development only)
- `info`: Normal operations (default)
- `warn`: Warnings and recoverable errors
- `error`: Errors requiring attention

**Log locations**:

- Systemd: `journalctl -u flakeguard -f`
- File: `/var/log/flakeguard.log` (if configured)
- Docker: `docker logs flakeguard`

**Useful log queries**:

```bash
# Recent errors
journalctl -u flakeguard --since "1 hour ago" | grep ERROR

# Ingestion activity
journalctl -u flakeguard | grep "ingestion completed"

# Retention job execution
journalctl -u flakeguard | grep "Retention job"

# Rate limit hits
journalctl -u flakeguard | grep "rate limit exceeded"
```

### Health Checks

**HTTP endpoint**: `GET /health`

Returns `200 OK` if service is healthy:

```json
{
  "status": "ok",
  "database": "connected",
  "version": "1.0.0"
}
```

**Database connectivity**:

```bash
psql -h localhost -U flakeguard_user -d flakeguard -c "SELECT 1;"
```

**Service status**:

```bash
systemctl status flakeguard
```

### Performance Monitoring

**Key metrics to track**:

1. **Ingestion latency**: Time to process JUnit upload (target: < 2s)
2. **Database query time**: Average query execution time (target: < 100ms)
3. **Rate limit hits**: Frequency of 429 responses (target: < 1%)
4. **Disk usage**: Database size growth rate
5. **Memory usage**: FlakeGuard process RSS (target: < 2GB)
6. **Connection pool**: Active vs idle connections

**Monitoring tools**:

- Prometheus + Grafana (recommended)
- CloudWatch (AWS)
- DataDog, New Relic (SaaS options)

### Emergency Procedures

**Service won't start**:

1. Check configuration: `FG_DATABASE_DSN`, `FG_JWT_SECRET` set?
2. Check database connectivity: Can you connect via `psql`?
3. Review logs: `journalctl -u flakeguard -n 100`
4. Verify migrations ran: `SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1;`

**Database migration failed**:

1. Check migration error in logs
2. Restore from backup
3. Fix migration SQL
4. Retry migration
5. Contact support if unrecoverable

**Disk full**:

1. Immediate: Delete old backups, clear temp files
2. Short-term: Reduce retention periods (restart retention job)
3. Long-term: Provision more disk space, archive old data

### Getting Help

- **Documentation**: [docs/](.)
- **GitHub Issues**: Report bugs with logs and reproduction steps
- **Email Support**: support@flakeguard.example.com (if applicable)
- **Community Slack**: Join #flakeguard (if applicable)

When reporting issues, include:

1. FlakeGuard version
2. Environment (dev/prod)
3. Configuration (redacted)
4. Logs (last 50 lines before error)
5. Steps to reproduce
