# Google Cloud Run Deployment Example

This directory contains an example implementation and deployment configuration for running Go Vanity URLs on Google Cloud Run.

It utilizes OIDC OAuth 2.0 to protect the administrative console (`/_admin`).

---

## Files

- [main.go](file:///home/markxp/workspace/govanityurls/cmd/run/main.go): Application entry point for Cloud Run.
- [.env.example](file:///home/markxp/workspace/govanityurls/cmd/run/.env.example): Example environment variables configuration file.

---

## Deployment

### 1. Configure Shell Variables

Define the target project, region, and repository details as shell variables. These will be reused in the build and deployment commands:

```bash
$ export PROJECT_ID="your-gcp-project-id"
$ export REGION="us-central1"
$ export REPOSITORY="your-artifact-registry-repo"
```

### 2. Build and Publish the Container

Build the container image using buildpacks. Because the repository contains multiple main entry points (`cmd/gae` and `cmd/run`), you must specify `GOOGLE_BUILDABLE=./cmd/run` to direct the buildpack to compile the Cloud Run entry point.

```bash
# Build locally and push
$ pack build ${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPOSITORY}/govanityurls:latest \
    --builder gcr.io/buildpacks/builder:latest \
    --env GOOGLE_BUILDABLE=./cmd/run
$ docker push ${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPOSITORY}/govanityurls:latest

# Or submit remotely via Cloud Build
$ gcloud builds submit --pack="builder=gcr.io/buildpacks/builder:latest,image=${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPOSITORY}/govanityurls:latest,env=GOOGLE_BUILDABLE=./cmd/run" --project=${PROJECT_ID}
```

Alternatively, you can build OCI image with other tools, like `ko` and `buildah`. It's not covered here.

### 3. Deploy to Cloud Run

```bash
$ gcloud run deploy SERVICE_NAME_YOU_LIKE \
    --image ${REGION}-docker.pkg.dev/${PROJECT_ID}/${REPOSITORY}/govanityurls:latest \
    --platform managed \
    --region ${REGION} \
    --allow-unauthenticated \
    --project ${PROJECT_ID}
```
We must expose the service publicly, or the `go-get` tool cannot fetch the package metadata. IAP is **not** used: `/_admin*` is protected in-app by Google OIDC login, and `/_internal/*` by Cloud Tasks OIDC bearer-token verification.

### 4. (Optional) Enable Asynchronous Registration via Cloud Tasks

Cloud Tasks calls `POST /_internal/registerRepo` with an OIDC token minted for a dedicated service account. The app accepts the request only if the token is Google-signed, its `aud` matches `CLOUD_TASKS_OIDC_AUDIENCE` (default `CLOUD_TASKS_TARGET_URL`), and its `email` equals `CLOUD_TASKS_SERVICE_ACCOUNT`.

```bash
$ export RUNTIME_SA="your-cloud-run-runtime-sa@${PROJECT_ID}.iam.gserviceaccount.com"
$ export TASKS_SA="cloud-tasks-invoker@${PROJECT_ID}.iam.gserviceaccount.com"
$ export QUEUE="register-repo"

# Queue and the identity Cloud Tasks uses to call the service
$ gcloud tasks queues create ${QUEUE} --location=${REGION} --project=${PROJECT_ID}
$ gcloud iam service-accounts create cloud-tasks-invoker --project=${PROJECT_ID}

# The Cloud Run runtime SA must be able to enqueue tasks...
$ gcloud tasks queues add-iam-policy-binding ${QUEUE} \
    --location=${REGION} \
    --member="serviceAccount:${RUNTIME_SA}" \
    --role="roles/cloudtasks.enqueuer" \
    --project=${PROJECT_ID}

# ...and act as the Cloud Tasks SA to attach its OIDC token to each task.
$ gcloud iam service-accounts add-iam-policy-binding ${TASKS_SA} \
    --member="serviceAccount:${RUNTIME_SA}" \
    --role="roles/iam.serviceAccountUser" \
    --project=${PROJECT_ID}
```

Then set `CLOUD_TASKS_QUEUE=projects/${PROJECT_ID}/locations/${REGION}/queues/${QUEUE}`, `CLOUD_TASKS_TARGET_URL=https://go.example.com/_internal/registerRepo`, and `CLOUD_TASKS_SERVICE_ACCOUNT=${TASKS_SA}` on the service.

> [!WARNING]
> If either binding is missing, `CreateTask` fails and the app falls back to a synchronous Firestore write. Registration still succeeds, so the misconfiguration only shows up as `Failed to create task` errors in Cloud Logging.

**Security Note (`roles/run.invoker` vs. In-App OIDC Verification):**
- Because public `go get` requires `--allow-unauthenticated` (`allUsers` has `roles/run.invoker`), Cloud Run's IAM ingress layer will **not** block unauthenticated calls to `/_internal/registerRepo`—making in-app OIDC verification (`CLOUD_TASKS_SERVICE_ACCOUNT` + `CLOUD_TASKS_OIDC_AUDIENCE`) essential.
- Conversely, if `/_internal/registerRepo` is deployed on a restricted Cloud Run service (`--no-allow-unauthenticated`), you must also grant `roles/run.invoker` to `${TASKS_SA}`.

---

## Configuration

Configure the following environment variables in your Cloud Run service settings:

| Variable | Required | Description |
| :--- | :--- | :--- |
| `GOOGLE_CLOUD_PROJECT` | **Yes** | GCP Project ID. |
| `VANITY_HOST` | No | Optional. The domain name serving your vanity URLs. If omitted, the service uses the request "Host" header to determine the domain name. It's not recommended in production. |
| `PORT` | No | Automatically injected by Google Cloud Run. The container listens on this port (defaults to `8080` if unset). |
| `HEALTH_PORT` | No | Port for Cloud Run health checks (defaults to `8081`). |
| `FIRESTORE_COLLECTION` | No | Name of the Firestore collection (defaults to `vanity_urls`). |
| `CACHE_MAX_AGE` | No | Cache expiration in seconds (defaults to `300`). |
| `OAUTH_CLIENT_ID` | **Yes** | Google OAuth 2.0 Client ID for OIDC authentication. |
| `OAUTH_CLIENT_SECRET` | **Yes** | Google OAuth 2.0 Client Secret. |
| `OAUTH_REDIRECT_URL` | **Yes** | Authorized redirect URI (e.g., `https://go.example.com/_admin/callback`). |
| `OAUTH_STATE_SECRET` | **Yes** | Shared key (≥ 32 bytes) for signing the OAuth `state` cookie. Must be identical on every instance, because `/_admin/login` and `/_admin/callback` may hit different instances. Store it in Secret Manager. |
| `ALLOWED_EMAILS` | No | Comma-separated list of user emails allowed to log in. |
| `ALLOWED_DOMAINS` | No | Comma-separated list of email domains allowed to log in. |
| `CLOUD_TASKS_QUEUE` | No | Full Cloud Tasks queue path (`projects/.../locations/.../queues/...`) to enable async `/_internal/registerRepo`. |
| `CLOUD_TASKS_TARGET_URL` | Conditional | Required if `CLOUD_TASKS_QUEUE` is set. Target URL for `/_internal/registerRepo`. |
| `CLOUD_TASKS_SERVICE_ACCOUNT` | Conditional | Required if `CLOUD_TASKS_QUEUE` is set. Service account email used by Cloud Tasks to mint OIDC tokens and verified on `/_internal/*`. |
| `CLOUD_TASKS_OIDC_AUDIENCE` | No | Expected OIDC `aud` claim for `/_internal/*` (defaults to `CLOUD_TASKS_TARGET_URL`). |

Do not forget to allow emails or domains (authorization). Otherwise, no one can access the admin console, even if they authenticate themselves with correct OIDC token.