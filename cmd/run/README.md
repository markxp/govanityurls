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
We must expose the service publicly, or the `go-get` tool cannot fetch the package metadata.

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
| `OAUTH_STATE_SECRET` | No | Signer key for OAuth states. Generates randomly if unset (unsuitable for multi-instance). |
| `ALLOWED_EMAILS` | No | Comma-separated list of user emails allowed to log in. |
| `ALLOWED_DOMAINS` | No | Comma-separated list of email domains allowed to log in. |

Do not forget to allow emails or domains (authorization). Otherwise, no one can access the admin console, even if they authenticate themselves with correct OIDC token.