# Adding a connector

Every directory under `connectors/` is one connector. Its name is the image name and must start
with `source-` or `destination-`.

```
connectors/source-acme/
├── Dockerfile     # builds an image whose ENTRYPOINT speaks the Airbyte protocol
├── meta.json      # registry metadata, validated against schema/meta.schema.json
├── spec.json      # output of the image's `spec` command, checked in
└── ...            # connector code (any language)
```

## 1. Write the connector

The image must implement the [Airbyte protocol](https://docs.airbyte.com/understanding-airbyte/airbyte-protocol):
`spec`, `check --config`, `discover --config` and `read --config --catalog [--state]` for sources;
`spec`, `check --config` and `write --config --catalog` (records on stdin) for destinations. Messages go to
stdout as JSON lines.

The quickest start is copying `source-example`, which uses
[airbyte-go-sdk](https://github.com/saturn4er/airbyte-go-sdk):

```bash
cp -r connectors/source-example connectors/source-acme
# edit go.mod (module name), source.go, meta.json
```

Python connectors built with the Airbyte CDK work too: any directory with a `Dockerfile` is built.
Go connectors (with a `go.mod`) also get `go vet` and `go test` in CI.

The image must build for `linux/amd64` and `linux/arm64`. Avoid installing anything that needs network
access at runtime, and keep `ENTRYPOINT` pointing at the connector binary.

## 2. Fill in meta.json

```json
{
  "$schema": "../../schema/meta.schema.json",
  "name": "Acme",
  "connector_type": "source",
  "version": "0.1.0",
  "release_stage": "alpha",
  "support_level": "community",
  "source_type": "api",
  "license": "MIT"
}
```

| Field | Required | Notes |
|---|---|---|
| `name` | yes | Display name in Synclet |
| `connector_type` | yes | `source` or `destination`; must match the directory prefix |
| `version` | yes | Semver. The image tag Synclet will run. Bump it to release |
| `release_stage` | yes | `alpha`, `beta`, `generally_available`, `custom` |
| `support_level` | yes | `community`, `certified` |
| `license` | yes | The connector's own license (SPDX id). Independent of this repository's license |
| `source_type` | no | `api`, `database`, `file`; empty for destinations |
| `documentation_url`, `icon_url` | no | Shown in the connector catalog |
| `breaking_changes` | no | `{"2.0.0": {"message": "...", "upgrade_deadline": "2027-01-31"}}` |
| `tombstone` | no | `true` hides the connector in Synclet without deleting its history |

The schema documents the remaining optional fields (`tags`, `allowed_hosts`, `resource_requirements`, ...)
and which registry field each one becomes. Editors that understand `$schema` autocomplete them.

## 3. Generate spec.json

```bash
task spec CONNECTOR=source-acme
```

This builds the image, runs `spec` and writes the result to `spec.json`. Synclet renders the connector's
configuration form from it without pulling the image. CI rebuilds the image and fails if `spec.json` is
stale, so rerun this whenever the spec changes.

## 4. Check locally and open a PR

```bash
task validate
task test CONNECTOR=source-acme
task registry && jq . dist/registry.json
```

On the pull request CI validates all metadata, builds and tests the changed connectors, compares
`spec.json` with the image, and fails if code changed while `version` still equals an already
released version.

## Releasing a new version

Bump `version` in `meta.json` (plus `spec.json` if the spec changed) and merge. The release workflow
builds `ghcr.io/<owner>/<repo>/<connector>:<version>` (also tagged `latest`), tags the commit
`<connector>-v<version>` and republishes `registry.json`.

Changes that only touch `meta.json` (other than `version`), `spec.json`, `*.md` or `*_test.go` do not need
a version bump; the registry picks them up on the next publish.

## Removing a connector

Set `"tombstone": true` and merge. Synclet skips tombstoned entries, so the connector disappears from
the catalog on the next repository sync while its images stay available. Deleting the directory has the
same effect on the catalog.
