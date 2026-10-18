# Synclet connectors template

A template for your own [Synclet](https://github.com/synclet-io/synclet) connectors repository: connectors written
in Go on [synclet-go-sdk](https://github.com/synclet-io/synclet-go-sdk), shipped as multi-arch Docker images on
GHCR and listed in a connector registry (`registry.json`) on GitHub Pages that Synclet reads from. It has the same
layout, scripts, CI and release flow as [synclet-io/connectors](https://github.com/synclet-io/connectors).

```
connectors/<name>/        one connector per directory: Dockerfile, meta.json, spec.json, Go code
schema/meta.schema.json   JSON Schema every meta.json is validated against
scripts/                  Go tool: validate metadata, resolve release tags, build registry.json
.github/workflows/        ci.yml (pull requests, main), release-please.yml (main), release.yml (release tags),
                          registry.yml (after each release)
release-please-config.json, .release-please-manifest.json   one release-please package per connector
Taskfile.yml              local shortcuts
```

Images are pushed as `ghcr.io/<owner>/<repo>/<connector>` and the registry is published at
`https://<owner>.github.io/<repo>/registry.json`; both follow from the repository's name, so nothing needs
editing after a rename. The release, release-please and registry workflows check `is_template` and do nothing in
the template repository (synclet-io/synclet-connectors-template) itself.

## Setup

### 1. Create your repository

Click **Use this template → Create a new repository** on
[synclet-io/synclet-connectors-template](https://github.com/synclet-io/synclet-connectors-template). Make the
repository **public**: Synclet fetches `registry.json` and pulls the images anonymously. GitHub Pages of a private
repository needs a paid plan, and on Enterprise Cloud it can be restricted to organization members, which
Synclet cannot fetch; a private repository works only with Pages visibility set to public (and the images made
public, step 6).

The first Release Please run, started by the repository's creation, fails because the App secrets do not exist
yet; re-run it from **Actions → Release Please** after step 2.

### 2. Create a GitHub App for releases

Tags and pull requests created with the default `GITHUB_TOKEN` start no workflows, so release-please runs with a
token minted from a GitHub App:

1. **Settings → Developer settings → GitHub Apps → New GitHub App** (under your organization's settings for an
   organization repository). Any name and homepage URL; untick **Webhook → Active**.
2. **Repository permissions**: **Contents**, **Pull requests** and **Issues**: **Read and write**.
3. Create the App, note its **App ID**, then **Generate a private key** (a `.pem` file is downloaded).
4. **Install App** → choose your account or organization → **Only select repositories** → your new repository.
5. In your repository, **Settings → Secrets and variables → Actions → New repository secret**:

| Secret | Value |
|---|---|
| `RELEASE_APP_ID` | The App ID |
| `RELEASE_APP_PRIVATE_KEY` | The full contents of the `.pem` file |

### 3. Enable GitHub Pages

The registry is deployed by a workflow, so Pages must use the **GitHub Actions** source: **Settings → Pages →
Build and deployment → Source: GitHub Actions**, or

```bash
gh api -X POST repos/<owner>/<repo>/pages -f build_type=workflow
```

The `github-pages` environment this creates lets only `main` deploy, which is where `registry.yml` runs.

### 4. Add a connector

Copy `connectors/source-example`, write the connector, fill `meta.json` and generate `spec.json`;
[CONTRIBUTING.md](CONTRIBUTING.md) walks through it. Add a package for it to `release-please-config.json` and keep
`version` in `meta.json` at `0.1.0`, or `task validate` fails; leave it out of `.release-please-manifest.json`
until its first release.
Point `documentation_url` in `meta.json` and `DocumentationURL` in the spec at your repository, or delete
`source-example` (its directory and its release-please entries) once you have your own connector.

Open a pull request; CI validates the metadata and, for the changed connectors, runs vet, golangci-lint and
`go test -race`, builds the image and checks `spec.json` against it. After merging, release-please opens the
connector's first release pull request at `0.1.0`.

### 5. Release with release-please

Later changes are released from [Conventional Commits](https://www.conventionalcommits.org) scoped to the
connector directory, for example `fix(source-acme): retry on HTTP 429`. After each push to `main`, release-please
opens or updates one pull request per connector with releasable changes, `chore: release <dir> <version>`, which
bumps `meta.json`, the manifest and the connector's `CHANGELOG.md`. Dependabot opens weekly `fix` pull requests per
connector directory, so dependency bumps are released too.

Merging the release pull request creates the tag `<dir>-v<version>`, and the release workflow builds and pushes
`ghcr.io/<owner>/<repo>/<dir>:<version>` (and `:latest`) for linux/amd64 and linux/arm64, then runs `spec` on the
pushed image for both platforms and checks that the image can be pulled anonymously. Once it succeeds, the
Registry workflow rebuilds `registry.json` from every connector's newest published release and deploys it to
Pages. A failed release can be re-run from **Actions → Release → Run workflow** with the same tag;
**Actions → Registry → Run workflow** republishes the registry by hand.

### 6. Make each image public

The first push of a connector creates a **private** GHCR package. Synclet pulls images anonymously, so the
release fails at "Image is publicly pullable" until the package is public: open the package (your profile or
organization → **Packages** → `<repo>/<connector>`) → **Package settings → Change visibility → Public**, once
per connector, then re-run the release for the same tag.

### 7. Add the registry to Synclet

In Synclet open **Settings → Connectors → Add Repository** and enter a name and the registry URL:

```
https://<owner>.github.io/<repo>/registry.json
```

The Registry workflow run links the deployed file from its `github-pages` environment. Synclet fetches the file
immediately (the repository is not saved if that fails) and re-syncs it every
`PIPELINE_JOB_REPOSITORY_SYNC_INTERVAL` (default `1h`); **Sync** on the repository refreshes it on demand.
Connectors removed from the file, or marked `tombstone`, disappear from the catalog on the next sync.

## How it works

```
pull request / main ──► ci.yml
                        ├─ test the scripts, validate every meta.json and spec.json
                        ├─ check release-please lists every connector at its meta.json version
                        ├─ dry-run registry entry generation
                        └─ per changed connector: go vet, golangci-lint, go test -race (Docker available
                           for testcontainers), docker build, spec.json == `spec` output

main ─────────────────► release-please.yml
                        └─ one release pull request per connector with releasable commits; merging it
                           bumps meta.json and CHANGELOG.md and creates the tag <dir>-v<version>

tag <dir>-v<version> ──► release.yml
                        ├─ plan: the tag must match connectors/<dir>/meta.json's version
                        └─ image: spec check, build linux/amd64 + linux/arm64, push
                              ghcr.io/<owner>/<repo>/<dir>:<version> and :latest,
                              run `spec` on the pushed image for both platforms, check it is public

Release succeeded ────► registry.yml (on main)
                        └─ rebuild registry.json from every connector's newest published release and
                           deploy it to GitHub Pages
```

`registry.yml` builds the file with `go run ./scripts registry --released`: for every connector directory it
takes the newest `<dir>-v<version>` tag whose image can be pulled anonymously and reads `meta.json` and
`spec.json` as of that tag. A release that is still running or failed is never listed, tags of deleted
connector directories are ignored, and two releases finishing in either order publish the same file. Entries use
the [Airbyte OSS registry format](https://connectors.airbyte.com/files/registries/v0/oss_registry.json).

## Local development

Requires Go 1.26+, Docker, [Task](https://taskfile.dev), `jq` and golangci-lint v2.

```bash
task validate                                   # schema-check every meta.json and spec.json
task test CONNECTOR=source-example              # go vet + go test
task lint CONNECTOR=source-example              # golangci-lint
task build CONNECTOR=source-example             # docker build for the local platform
task spec CONNECTOR=source-example              # build, run `spec`, write spec.json
task spec:check CONNECTOR=source-example        # fail if spec.json is stale
task entry CONNECTOR=source-example             # print a connector's registry entry from the working tree
task registry                                   # write dist/registry.json with every connector (working tree)
go test ./scripts                               # the scripts' own tests
```

Image names default to `ghcr.io/<owner>/<repo>` of the `origin` remote; pass `IMAGE_PREFIX=...` to override.
`go test -short ./...` in a connector skips the tests that need Docker.

### Building against a local synclet-go-sdk

```bash
task sdk:local SDK_DIR=../synclet-go-sdk                              # writes connectors/go.work (git-ignored)
task spec CONNECTOR=source-example SDK_DIR=../synclet-go-sdk          # docker build --build-context sdk=../synclet-go-sdk
```

Each connector's Dockerfile has an empty `sdk` stage; `--build-context sdk=<dir>` replaces it, and the build then
adds `replace github.com/synclet-io/synclet-go-sdk => <dir>` to the image's copy of `go.mod`. Nothing in the
repository changes, so CI always builds against the published module.

## License

[MIT](LICENSE). Each connector also declares its license in the `license` field of its `meta.json`, which is
published in the registry and shown in Synclet.

Airbyte is a trademark of Airbyte, Inc.; this project is not affiliated with or endorsed by Airbyte, Inc.
