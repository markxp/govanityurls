# Go Vanity URLs

Go Vanity URLs is a simple Go server that allows you
to set custom import paths for your Go packages.
It also can run on Google App Engine and Google Cloud Run.

## Quickstart

1. Download the source code:

```
$ git clone https://github.com/markxp/govanityurls
$ cd govanityurls/cmd
```

2. Build the container image: 

```
# The `location` is the region of your docker artifact registry, e.g. `us-central1`, `europe-west1` for regional registry, or `asia` for multi-regional registry.

# In this example, we use buildpacks to build the container.

# a. build locally with buildpacks
$ pack build ${location}-docker.pkg.dev/<your-project>/<repository-name>/<image-name>:<tag> --builder gcr.io/buildpacks/builder:latest

# ... and push it to the artifact registry
$ docker push ${location}-docker.pkg.dev/<your-project>/<repository-name>/<image-name>:<tag>


# or
# b. you prefer build image remotely on cloud build
$ gcloud builds submit --pack="builder=gcr.io/buildpacks/builder:latest,image=${location}-docker.pkg.dev/<your-project>/<repository-name>/<image-name>:<tag>"
```



3. Deploy the application:
```
# If you use App Engine(standard environment), you do not need to build the container image.
a. Deploy to App Engine(standard environment)
# write a app.yaml file....
$ gcloud app deploy .

b. Deploy to Clou Run
$ gcloud run deploy --image {image-from-step-2} --platform managed
...OR deploy without building the image (directly from source code)
$ cd ../
$ gcloud run deploy {service-name} --source . --platform managed --region {region} --allow-unauthenticated
```

4. Prepare the storage and Cloud Tasks queue (optional):
```
# We use firestore as the storage. Create the database.

# (optional) Create the queue. Noting that the service account of the queue should have permission to hit the server's endpoint `POST {host}/_internal/createRepo`.
```



(deprecated) View `vanity.yaml` to know how to add repos. E.g., `custom-domain.com/portmidi` will
serve the [https://github.com/rakyll/portmidi](https://github.com/rakyll/portmidi) repo.

```
paths:
  /portmidi:
    repo: https://github.com/rakyll/portmidi
```

And use the module with `go get`:
```
$ go get customdomain.com/portmidi
```

## App Configuration

```
host: example.com
cache_max_age: 3600
```

<table>
  <thead>
    <tr>
      <th scope="col">Key</th>
      <th scope="col">Required</th>
      <th scope="col">Description</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <th scope="row"><code>cache_max_age</code></th>
      <td>optional</td>
      <td>The amount of time to cache package pages in seconds.  Controls the <code>max-age</code> directive sent in the <a href="https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Cache-Control"><code>Cache-Control</code></a> HTTP header.</td>
    </tr>
    <tr>
      <th scope="row"><code>host</code></th>
      <td>optional</td>
      <td>Host name to use in meta tags.  If omitted, uses the request "host" haeder to derive host name.  You can use this option to fix the host when using this service behind a reverse proxy or a <a href="https://cloud.google.com/appengine/docs/standard/go/how-requests-are-routed#routing_with_a_dispatch_file">custom dispatch file</a>.</td>
    </tr>
  </tbody>
</table>

### Path Configuration

<table>
  <thead>
    <tr>
      <th scope="col">Key</th>
      <th scope="col">Required</th>
      <th scope="col">Description</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <th scope="row"><code>display</code></th>
      <td>optional</td>
      <td>The last three fields of the <a href="https://github.com/golang/gddo/wiki/Source-Code-Links"><code>go-source</code> meta tag</a>.  If omitted, it is inferred from the code hosting service if possible.</td>
    </tr>
    <tr>
      <th scope="row"><code>repo</code></th>
      <td>required</td>
      <td>Root URL of the repository as it would appear in <a href="https://pkg.go.dev/cmd/go#hdr-Remote_import_paths"><code>go-import</code> meta tag</a>.</td>
    </tr>
    <tr>
      <th scope="row"><code>vcs</code></th>
      <td><b>required</b></td>
      <td>If the version control system cannot be inferred (e.g. for Bitbucket or a custom domain), then this specifies the version control system as it would appear in <a href="https://pkg.go.dev/cmd/go#hdr-Remote_import_paths"><code>go-import</code> meta tag</a>.  This can be one of <code>git</code>, <code>hg</code>, <code>svn</code>, or <code>bzr</code>.A special value <code>mod</code> can be used to specify the module uses Go Module Proxy to serve bundled source code directly instead of using version control system.</td>
    </tr>
  </tbody>
</table>
