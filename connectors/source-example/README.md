# source-example

Minimal Go source built on [airbyte-go-sdk](https://github.com/saturn4er/airbyte-go-sdk).
Emits `count` rows (default 10) into a single `numbers` stream with `id` and `label` columns.

Copy this directory to start a new connector; see [CONNECTORS.md](../../CONNECTORS.md).

```bash
task test CONNECTOR=source-example
task build CONNECTOR=source-example
echo '{"count": 3}' > /tmp/config.json
docker run --rm -v /tmp:/data ghcr.io/your-org/your-registry/source-example:dev check --config /data/config.json
```
