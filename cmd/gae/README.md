# Google App Engine (GAE) Deployment Example

This directory contains an example implementation and deployment configuration for running Go Vanity URLs on Google App Engine (standard environment).

It uses Google Identity-Aware Proxy (IAP) to secure the admin routes (`/_admin`, `/_internal`).

---

## Files

- [main.go](file:///home/markxp/workspace/govanityurls/cmd/gae/main.go): Application entry point for GAE.
- [default.app.yaml](file:///home/markxp/workspace/govanityurls/cmd/gae/default.app.yaml): App Engine configuration for the default service (handling public traffic).
- [protected.app.yaml](file:///home/markxp/workspace/govanityurls/cmd/gae/protected.app.yaml): App Engine configuration for the administrative service (protected by IAP).
- [dispatch.yaml](file:///home/markxp/workspace/govanityurls/cmd/gae/dispatch.yaml): Routing configuration to dispatch traffic to the appropriate service.

---

## Deployment

### 1. Configure Shell Variables

Define the target GCP Project ID:

```bash
$ export PROJECT_ID="your-gcp-project-id"
```

### 2. Deploy to App Engine

Navigate to `cmd/gae` and deploy using the provided configurations:

```bash
$ cd cmd/gae
$ gcloud app deploy default.app.yaml protected.app.yaml dispatch.yaml --project=${PROJECT_ID}
```

This deploys two services:
1. **`default`**: Serves the public `go get` requests.
2. **`protected`**: Serves the administrative console. The `dispatch.yaml` file routes all `/_admin*` and `/_internal*` requests here.

---

## Configuration

The services are configured using environment variables in the `app.yaml` files or GAE settings.

| Variable | Required | Description |
| :--- | :--- | :--- |
| `GOOGLE_CLOUD_PROJECT` | **Yes** | GCP Project ID. |
| `FIRESTORE_COLLECTION` | No | Name of the Firestore collection (defaults to `vanity_urls`). |
| `VANITY_HOST` | No | Overrides the host name in generated import tags. |
| `CACHE_MAX_AGE` | No | `Cache-Control` header max-age in seconds (defaults to `300`). |
| `IAP_AUDIENCE` | **Yes (Protected)** | Audience string used to validate Google Identity-Aware Proxy (IAP) JWT tokens. |
