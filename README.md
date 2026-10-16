# Synclet connector registry template

A template for running your own [Synclet](https://github.com/synclet-io/synclet) connector registry:
a repository of Airbyte-protocol connectors whose images are built and pushed to GitHub Container
Registry, plus a `registry.json` catalog that Synclet syncs to offer those connectors in its UI.

Everything runs on GitHub Actions with the default `GITHUB_TOKEN`. No secrets to configure.

```
connectors/<name>/        one connector per directory: Dockerfile, meta.json, spec.json, code
schema/meta.schema.json   JSON Schema every meta.json is validated against
scripts/                  Go tool: validate metadata, generate registry.json, plan releases
.github/workflows/        ci.yml (pull requests), release.yml (main)
Taskfile.yml              local shortcuts
```

## Use this template

1. Click **Use this template → Create a new repository**. Pick the owner whose GHCR namespace should
   hold the images; images are published as `ghcr.io/<owner>/<repo>/<connector>`.
2. **Settings → Pages → Build and deployment → Source: GitHub Actions.** Needed once, before the
   first release run, or the `pages` job fails. Skip this for a private registry (see below).
3. **Settings → Actions → General → Workflow permissions:** leave the default; each job requests the
   permissions it needs (`packages: write`, `contents: write`, `pages: write`).
4. Replace `source-example` with your own connectors, or keep it to try the flow end to end.
5. Push to `main`. The release workflow builds and pushes every connector that has not been released
   yet, tags it, and publishes:

   ```
   https://<owner>.github.io/<repo>/registry.json
   ```

6. The first push of each image creates a **private** GHCR package. For a public registry open
   **Packages → \<connector\> → Package settings → Change visibility → Public** once per connector.
   Synclet executors pull images anonymously unless configured otherwise (see below).

## How it works

```
pull request ──► ci.yml
                 ├─ validate every meta.json against schema/meta.schema.json
                 ├─ fail if a connector's code changed but its version is already released
                 ├─ dry-run registry.json generation
                 └─ per changed connector: go vet/test (Go only), docker build, spec.json == `spec` output

push to main ──► release.yml
                 ├─ plan: connectors whose meta.json version has no <connector>-v<version> tag
                 ├─ release (per pending connector): build linux/amd64 + linux/arm64, push
                 │     ghcr.io/<owner>/<repo>/<connector>:<version> and :latest, push the git tag
                 ├─ registry: generate registry.json from released connectors
                 └─ pages: deploy registry.json to GitHub Pages
```

Versioning is per connector and driven by `meta.json`: bump `version`, merge, and that version is
released and tagged `<connector>-v<version>`. Merging without a bump releases nothing, so the release
workflow is idempotent; re-running it after a failure only retries what is still untagged. The release
tags are pushed with `GITHUB_TOKEN`, which does not trigger further workflow runs. If you protect tags
with rulesets, allow GitHub Actions to create `*-v*` tags.

### How registry.json is produced

`go run ./scripts registry` (or `task registry`) reads every `connectors/*/meta.json` and
`connectors/*/spec.json` and writes the [Airbyte OSS registry
format](https://connectors.airbyte.com/files/registries/v0/oss_registry.json) that Synclet parses:

```json
{
  "sources": [
    {
      "name": "Example",
      "dockerRepository": "ghcr.io/acme/connectors/source-example",
      "dockerImageTag": "0.1.0",
      "releaseStage": "alpha",
      "supportLevel": "community",
      "sourceType": "api",
      "license": "MIT",
      "tombstone": false,
      "spec": { "connectionSpecification": { "type": "object", "properties": { "count": { "type": ["integer"] } } } }
    }
  ],
  "destinations": []
}
```

- `dockerRepository` is `ghcr.io/<owner>/<repo>/<directory>` in lower case (`--image-prefix` overrides it).
- `dockerImageTag` is `meta.json`'s `version`; the other fields map one to one, as documented in the schema.
- `spec` is the checked-in `spec.json`, the exact output of the image's `spec` command. Checking it in
  keeps registry generation fast and independent of registry credentials; CI rebuilds the image and fails
  when the file is stale.
- In the release workflow `--released-only` describes each connector as of its newest release tag: if a
  bumped version failed to build, the registry keeps serving the previous version instead of pointing at
  an image tag that does not exist. Connectors that were never released are left out.

## Local development

Requires Go 1.25+, Docker, [Task](https://taskfile.dev) and `jq`.

```bash
task validate                                   # schema-check every meta.json and spec.json
task test CONNECTOR=source-example              # go vet + go test
task build CONNECTOR=source-example             # docker build for the local platform
task spec CONNECTOR=source-example              # build, run `spec`, write spec.json
task spec:check CONNECTOR=source-example        # fail if spec.json is stale
task registry IMAGE_PREFIX=ghcr.io/acme/connectors   # write dist/registry.json
```

See [CONNECTORS.md](CONNECTORS.md) for adding a connector.

## Register the registry in Synclet

In Synclet open **Settings → Connectors → Add Repository** and enter a name and the registry URL,
for example `https://acme.github.io/connectors/registry.json`. Synclet fetches the file immediately (the
repository is not saved if that fails) and then re-syncs every `PIPELINE_JOB_REPOSITORY_SYNC_INTERVAL`
(default `1h`); **Sync** on the repository refreshes it on demand. Connectors removed from the file, or
marked `tombstone`, disappear from the catalog on the next sync.

## Private registries

There are two independent parts: the catalog (`registry.json`) and the images.

### The catalog

Synclet sends the repository's **Auth Header** value verbatim as the HTTP `Authorization` header on every
registry fetch (it is stored encrypted). Any URL that accepts a static `Authorization` header works; Synclet
cannot follow login redirects, use cookies, or send other headers.

GitHub Pages of a private repository requires a browser login, so it cannot serve a private catalog to
Synclet. Instead:

1. Set the repository variable `REGISTRY_BRANCH` (for example `registry`) and, optionally,
   `PUBLISH_PAGES=false` (**Settings → Secrets and variables → Actions → Variables**). The release workflow
   then force-pushes a single-commit branch containing only `registry.json`.
2. Create a fine-grained personal access token (or one for a machine user) with read-only **Contents**
   access to this repository.
3. In Synclet use
   - URL: `https://raw.githubusercontent.com/<owner>/<repo>/registry/registry.json`
   - Auth Header: `token <PAT>`

Any other host works the same way, e.g. an internal web server or object storage behind a proxy that checks
`Authorization: Bearer <token>`: publish `_site/registry.json` from the `registry` job wherever you need it.

### The images

Synclet never sends credentials when it pulls connector images itself, so private GHCR packages need
credentials on the executor:

- **Docker executor**: Synclet runs the image if it already exists locally and pulls it anonymously only when
  it is missing. Log the Docker host in and pre-pull every released version:

  ```bash
  echo "$PAT" | docker login ghcr.io -u <user> --password-stdin   # PAT with read:packages
  docker pull ghcr.io/<owner>/<repo>/<connector>:<version>
  ```

  Pre-pull again after each release; a new version that is not present locally fails to pull.
- **Kubernetes executor**: create a pull secret in the namespace the executor launches pods in
  (`K8S_EXECUTOR_NAMESPACE`) and set `K8S_EXECUTOR_IMAGE_PULL_SECRET` to its name. Synclet adds it to every
  connector pod.

  ```bash
  kubectl -n <namespace> create secret docker-registry ghcr-pull \
    --docker-server=ghcr.io --docker-username=<user> --docker-password=<PAT with read:packages>
  ```

  Node-level registry credentials (for example a kubelet credential provider) work as well.

## License

The template itself (workflows, scripts, schema, docs and the example connector) is [MIT](LICENSE)
licensed. Each connector declares its own license in the `license` field of its `meta.json`, which is
published in the registry and shown in Synclet; it does not have to match this repository's license.
Synclet hides entries licensed `Airbyte Enterprise`, so the schema rejects that value.
