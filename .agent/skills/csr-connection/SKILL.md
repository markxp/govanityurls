---
name: csr-connection
description: >-
  Configures and troubleshoots Google Cloud Source Repositories (CSR)
  connections, IAM roles and permissions, repository URL formats (Git clone vs
  Cloud Console UI vs `go-import` / `go-source` tags), Git authentication
  (`gcloud.sh`, ADC, Cloud Build mirroring), and `govanityurls` fallback
  wiring. Use when setting up CSR access, resolving CSR repository addresses,
  granting `roles/source.*` IAM bindings, configuring Git credential helpers, or
  wiring `contrib/cloudsourcerepo` into Cloud Run / App Engine.
---

# Google Cloud Source Repositories (CSR) Connection Guide

Architecture, IAM roles, URL address conventions, Git authentication, and `govanityurls` integration patterns for Google Cloud Source Repositories (`sourcerepo.googleapis.com`).

> [!NOTE]
> **Service Lifecycle Context**: Cloud Source Repositories reached End-of-Sale for net-new organizations/projects without prior usage on June 17, 2024. Existing CSR projects/repositories continue operating normally; net-new setups without grandfathered access should evaluate **Secure Source Manager** or **Developer Connect** + GitHub/GitLab.

---

## 1. Mandatory Identity Preflight

Before running any `gcloud` or IAM command:
```bash
gcloud config get-value account
gcloud config get-value project
```
Verify enterprise (`@<company-domain>`) vs personal (`@gmail.com`) account alignment. Never modify IAM bindings autonomously without explicit user confirmation.

---

## 2. Repository Address & URL Formats

CSR uses distinct hostnames and path structures depending on whether the caller is `git` / `go get`, a human browser (`pkg.go.dev`), or the REST API.

| Purpose | Address Format | Example (`project = my-proj`, `repo = my-mod`) |
| :--- | :--- | :--- |
| **REST API Resource Name** | `projects/{PROJECT_ID}/repos/{REPO_NAME}` | `projects/my-proj/repos/my-mod` |
| **REST API Endpoint** | `https://sourcerepo.googleapis.com/v1/projects/{PROJECT_ID}/repos/{REPO_NAME}` | *(Use the `csr-get-repository` skill for HTTP calls)* |
| **Git Clone & `go-import` Repo URL** | `https://source.developers.google.com/p/{PROJECT_ID}/r/{REPO_NAME}` | `https://source.developers.google.com/p/my-proj/r/my-mod` |
| **SSH Clone URL** | `ssh://{EMAIL}@source.developers.google.com:2022/p/{PROJECT_ID}/r/{REPO_NAME}` | `ssh://dev@example.com@source.developers.google.com:2022/p/my-proj/r/my-mod` |
| **Browser UI (`ui-home`)** | `https://source.cloud.google.com/{PROJECT_ID}/{REPO_NAME}` | `https://source.cloud.google.com/my-proj/my-mod` |
| **Directory View (`ui-directory`)** | `https://source.cloud.google.com/{PROJECT_ID}/{REPO_NAME}/+/{BRANCH}:{dir}` | `https://source.cloud.google.com/my-proj/my-mod/+/main:{dir}` |
| **File + Line View (`ui-file`)** | `https://source.cloud.google.com/{PROJECT_ID}/{REPO_NAME}/+/{BRANCH}:{dir}/{file}#L{line}` | `https://source.cloud.google.com/my-proj/my-mod/+/main:{dir}/{file}#L{line}` |

### How to Ask CSR for a Repository's Canonical Address
1. **Via REST HTTP (`projects.repos.get`)**: Query `GET https://sourcerepo.googleapis.com/v1/projects/{PROJECT_ID}/repos/{REPO_NAME}` and read the `.url` field (returns `https://source.developers.google.com/p/{PROJECT_ID}/r/{REPO_NAME}`). See [csr-get-repository](../csr-get-repository/SKILL.md).
2. **Via `gcloud` CLI**:
   ```bash
   gcloud source repos describe "${REPO_NAME}" \
     --project="${PROJECT_ID}" \
     --format="value(url)" \
     --quiet
   ```
3. **In `govanityurls` (`go-import` & `go-source` Mapping)**:
   * `RepoConfig.Repo` **must** use `source.developers.google.com/p/<project>/r/<repo>` (matches `cloudsourcerepo.Registry.ServerName()`).
   * `cloudsourcerepo.Registry.Display()` ([contrib/cloudsourcerepo/cloudsourcerepo.go](file:///home/markxp/workspace/govanityurls/contrib/cloudsourcerepo/cloudsourcerepo.go)) automatically rewrites `source.developers.google.com/p/<project>/r/<repo>` into the 3-part `source.cloud.google.com/<project>/<repo>` `go-source` string.

---

## 3. IAM Roles & Least-Privilege Bindings

Never grant basic roles (`roles/owner`, `roles/editor`, `roles/viewer`). Prefer **repository-level** IAM bindings over project-level bindings whenever scoping to specific repositories.

| Predefined Role | Key Permissions Included | Target Principal & Use Case |
| :--- | :--- | :--- |
| **`roles/source.reader`** *(Source Repository Reader)* | `source.repos.get`, `source.repos.list` | **Cloud Run / GAE runtime SA** running `govanityurls` `RepoChecker` fallback; developers/CI running `go get` on private CSR modules; read-only build triggers. |
| **`roles/source.writer`** *(Source Repository Writer)* | Reader + `source.repos.update` | Developers pushing commits/tags; **Cloud Build mirror SA** pushing branches/tags to a mirror CSR repository. |
| **`roles/source.admin`** *(Source Repository Administrator)* | Writer + `source.repos.create`, `source.repos.delete`, `source.repos.getIamPolicy`, `source.repos.setIamPolicy` | Platform/IaC automation managing repository lifecycle and repo-level IAM policies. |

### IAM Policy Commands (Requires User Approval Before Execution)

* **Repository-level binding** (least privilege for a specific repo):
  ```bash
  gcloud source repos add-iam-policy-binding "${REPO_NAME}" \
    --project="${PROJECT_ID}" \
    --member="serviceAccount:${RUNTIME_SA}" \
    --role="roles/source.reader" \
    --quiet
  ```
* **Project-level binding** (required when `govanityurls` dynamically resolves arbitrary repos in `${PROJECT_ID}` via `FallbackConfig`):
  ```bash
  gcloud projects add-iam-policy-binding "${PROJECT_ID}" \
    --member="serviceAccount:${RUNTIME_SA}" \
    --role="roles/source.reader" \
    --condition=None \
    --quiet
  ```
* **Inspect repo IAM policy**:
  ```bash
  gcloud source repos get-iam-policy "${REPO_NAME}" \
    --project="${PROJECT_ID}" \
    --format=json \
    --quiet
  ```

---

## 4. Client & `go get` Authentication

### A. Local Developer Workstation (`go get` & `git` over HTTPS)
Configure Git to authenticate to `source.developers.google.com` using the `gcloud` credential helper (keyless, short-lived OAuth tokens — never generate static passwords or SA JSON keys):

```bash
# Linux / macOS
git config --global credential.'https://source.developers.google.com'.helper gcloud.sh
```

For private Go vanity modules backed by CSR, configure `GOPRIVATE` / `GONOSUMDB` so the Go toolchain bypasses the public proxy/checksum DB and uses local Git credentials directly:

```bash
go env -w GOPRIVATE="go.example.com/*"
```

### B. Cloud Build / CI Mirroring
In headless Cloud Build steps interacting with CSR over HTTPS, attach a user-managed service account with `roles/source.reader` (or `roles/source.writer` for mirroring) and enable the helper:

```bash
git config --global credential.helper gcloud.sh
git remote add csr "https://source.developers.google.com/p/${PROJECT_ID}/r/${REPO_NAME}"
git fetch csr
```

### C. Cloud Run / App Engine Workload (`govanityurls` Fallback)
Wire `contrib/cloudsourcerepo` using Application Default Credentials (ADC) attached to the user-managed runtime service account:

```go
csrChecker, err := cloudsourcerepo.NewService(ctx, csrProjectID)
if err != nil {
    // handle initialization error
}
csrRegistry := &cloudsourcerepo.Registry{Branch: "main"}

fallback := &govanityurls.FallbackConfig{
    RepoChecker: csrChecker,
    Registry:    csrRegistry,
}
app := govanityurls.NewApp(
    vanityHost,
    cacheMaxAgeSec,
    store,
    fallback,
    []registry.Registry{&github.GithubRegistry{}, &bitbucket.Registry{}},
    asyncCfg,
    logger,
)
```

---

## 5. Troubleshooting Checklist (`403` / `404` / `go get` Failures)

1. **`403 PERMISSION_DENIED` on `Repos.Get` or `git fetch`**:
   * Verify `sourcerepo.googleapis.com` is enabled on `${PROJECT_ID}`.
   * Run Policy Troubleshooter for `source.repos.get`:
     ```bash
     gcloud policy-intelligence troubleshoot-policy "//cloudresourcemanager.googleapis.com/projects/${PROJECT_ID}" \
       --principal-email="${PRINCIPAL_EMAIL}" \
       --permission="source.repos.get" \
       --quiet
     ```
   * Check VPC Service Controls (`VpcServiceControlAuditMetadata`) and Organization IAM v2 Deny policies if Allow bindings exist.
2. **`go get` fails with `404` or checksum mismatch on private CSR repo**:
   * Confirm `GOPRIVATE` includes the vanity domain (`go env GOPRIVATE`).
   * Confirm `git ls-remote https://source.developers.google.com/p/${PROJECT_ID}/r/${REPO_NAME}` succeeds locally via `credential.helper=gcloud.sh`.
3. **Subdirectory module tag resolution fails (Go 1.25+ `subdir`)**:
   * Ensure Git tags in the CSR repository include the subdirectory prefix (e.g., `pkg/util/v1.0.0`, not `v1.0.0`).
