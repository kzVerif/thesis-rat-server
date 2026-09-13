# Antivirus scan results API

Both endpoints require the existing authentication mechanism and `av.read`
permission. Users without `av.read` receive `403`.

For existing databases, insert the `av.read` permission and assign it to the
appropriate roles through `role_permissions` before using these endpoints.
`seed.sql` includes this permission and grants it to `ADMINISTRATOR`.

## List results

`GET /api/av-scan-results?page=1&limit=20`

Reads `av_scan_results`. Each item is one agent's scan result, linked to an
`av_jobs` job by `job_id`.

| Query | Default | Allowed values |
| --- | --- | --- |
| `page` | `1` | Positive integer |
| `limit` | `20` | Integer from 1 to 100 |
| `agent_id` | All | UUID |
| `job_id` | All | UUID |
| `command_id` | All | UUID |
| `status` | All | PENDING, RUNNING, COMPLETED, FAILED, CANCELLED (case insensitive) |

Filters can be combined. Results are ordered by `created_at DESC, id DESC`.
The response contains `av_scan_results` (array) and `pagination`:

```json
{
  "av_scan_results": [],
  "pagination": { "page": 1, "limit": 20, "total": 0, "total_pages": 0 }
}
```

`total` counts all results matching the filters. Pages beyond the last page return
an empty array while preserving the matching total. Offset pagination may shift
between requests when results are inserted or deleted.

Each result includes `id`, `agent_id`, `command_id`, `job_id`, `scan_type`,
`started_at`, `finished_at`, `total_files_scanned`, `threats_found`,
`threat_details`, `status`, and `created_at`. Nullable times and threat details
are returned as JSON `null`; populated threat details are JSON, not a string.

## Get one result

`GET /api/av-scan-results/:id`

Returns a single result object with the fields above. `id` is the scan result UUID,
not the job UUID. To fetch a job's results, use the list endpoint with `job_id`.

## Errors

- `400`: invalid pagination, UUID, or status.
- `401`: authentication required.
- `403`: missing permission.
- `404`: result not found (detail endpoint).
- `500`: database/read failure.
