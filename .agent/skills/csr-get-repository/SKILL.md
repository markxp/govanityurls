---
name: csr-get-repository
description: >-
  Fetches a Google Cloud Source Repository (`<project>/<repository>`) metadata
  and clone URL via direct HTTP REST call (`projects.repos.get`), `gcloud`, or
  the Go `sourcerepo/v1` client (`contrib/cloudsourcerepo`). Trigger on
  `csr:get-repository`, `projects.repos.get`, checking if a CSR repository
  exists, or retrieving a CSR repository URL in an HTTP call.
---

# CSR: Get Repository (`csr:get-repository`)

Retrieves repository metadata (including the canonical Git clone `url`) for `<project>/<repository>` from the Google Cloud Source Repositories v1 REST API.

---

## 1. Mandatory Identity Preflight

Before executing any live `gcloud` or authenticated `curl` call against Google Cloud:
1. Check active account and project:
   ```bash
   gcloud config get-value account
   gcloud config get-value project
   ```
2. **Hard Boundary**: Enterprise repositories must use the matching `@<company-domain>` identity; personal projects must use `@gmail.com`. Halt immediately on mismatch and output `gcloud config configurations activate <name>`.

---

## 2. Direct HTTP Call (`projects.repos.get`)

### Endpoint Specification
* **Method & URL**:
  ```http
  GET https://sourcerepo.googleapis.com/v1/projects/{PROJECT_ID}/repos/{REPO_NAME}
  ```
  *(Note: `{REPO_NAME}` may contain slashes, e.g., `team/my-repo`; the wildcard `projects/*/repos/**` matches multi-segment names unescaped).*
* **Required IAM Permission**: `source.repos.get` (granted by `roles/source.reader`, `roles/source.writer`, or `roles/source.admin`).
* **OAuth 2.0 Scopes**:
  * `https://www.googleapis.com/auth/source.read_only` (least-privilege read)
  * `https://www.googleapis.com/auth/source.read_write`
  * `https://www.googleapis.com/auth/source.full_control`
  * `https://www.googleapis.com/auth/cloud-platform`

### Executable `curl` Blueprint

Obtain a bearer token first (prefer Application Default Credentials or active `gcloud` login), then issue the HTTP `GET` request:

```bash
# Step 1: Print access token (or use `gcloud auth print-access-token`)
ACCESS_TOKEN="$(gcloud auth application-default print-access-token)"

# Step 2: Query <project>/<repository> over HTTP
curl -sS -X GET \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  -H "X-Goog-User-Project: ${PROJECT_ID}" \
  -H "Accept: application/json" \
  "https://sourcerepo.googleapis.com/v1/projects/${PROJECT_ID}/repos/${REPO_NAME}"
```

### Expected JSON Response (`200 OK`)

```json
{
  "name": "projects/my-gcp-project/repos/my-repo",
  "size": "48291",
  "url": "https://source.developers.google.com/p/my-gcp-project/r/my-repo",
  "pubsubConfigs": {},
  "mirrorConfig": {
    "url": "https://github.com/org/my-repo.git",
    "webhookId": "...",
    "deployKeyId": "..."
  }
}
```

### HTTP Status Handling
| Status | Meaning | Action in `govanityurls` |
| :--- | :--- | :--- |
| `200 OK` | Repository exists | Extract `.url` (`https://source.developers.google.com/p/<project>/r/<repo>`) for `RepoConfig.Repo`. |
| `403 PERMISSION_DENIED` | Caller lacks `source.repos.get` (or API disabled / VPC-SC block) | Grant `roles/source.reader` on the repo or project to the caller SA (see `csr-connection` skill). |
| `404 NOT_FOUND` | Repository does not exist in `<project>` | Treat as cache miss / unresolvable fallback (`CheckRepo` returns `"", nil`). |

---

## 3. Listing Repositories in a Project (`projects.repos.list`)

When the exact `<repository>` name is unknown:
* **HTTP Call** (requires `source.repos.list`):
  ```bash
  curl -sS -X GET \
    -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    -H "X-Goog-User-Project: ${PROJECT_ID}" \
    "https://sourcerepo.googleapis.com/v1/projects/${PROJECT_ID}/repos?pageSize=50"
  ```

---

## 4. `gcloud` CLI Equivalent

*(If `gcloud` is invoked directly by an agent, validate flags via `gcloud help source repos describe` first.)*

```bash
gcloud source repos describe "${REPO_NAME}" \
  --project="${PROJECT_ID}" \
  --format="json(name,url,size,mirrorConfig)" \
  --quiet
```

---

## 5. Codebase Integration (`contrib/cloudsourcerepo`)

In `govanityurls`, [contrib/cloudsourcerepo/cloudsourcerepo.go](file:///home/markxp/workspace/govanityurls/contrib/cloudsourcerepo/cloudsourcerepo.go) wraps `projects.repos.get` via `google.golang.org/api/sourcerepo/v1`:

* `RepoChecker.CheckRepo(ctx, repoName)` trims leading slashes, formats `projects/{projectID}/repos/{repoName}`, calls `svc.Projects.Repos.Get(resourceName).Do()`, and returns `repo.Url` (mapping `404` to `"", nil`).
* Used in `App` via `FallbackConfig` ([app.go](file:///home/markxp/workspace/govanityurls/app.go)) to auto-discover single-segment unregistered modules and persist them to storage.
* Run unit and integration tests:
  ```bash
  go test -v ./contrib/cloudsourcerepo/...
  GOOGLE_CLOUD_PROJECT="${PROJECT_ID}" go test -tags=integration -v ./contrib/cloudsourcerepo/...
  ```
