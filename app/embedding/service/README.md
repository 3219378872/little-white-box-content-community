# Embedding gRPC service

This service implements `proto/embedding/embedding.proto` with a pinned
sentence-transformers model revision. Startup fails when the model cannot load
or its dimension differs from `EMBEDDING_DIMENSION`.

Required environment variables:

- `EMBEDDING_MODEL_NAME`: Hugging Face model name or local model path.
- `EMBEDDING_MODEL_REVISION`: immutable model commit or local artifact revision.
- `EMBEDDING_MODEL_VERSION`: must equal `<model-name>@<model-revision>` and is
  stored with every Milvus vector.

Default limits are 16,384 UTF-8 bytes per text, 64 texts and 262,144 bytes per
batch. The default dimension is 384. All limits are enforced by both the Python
service and the Go client.

Regenerate Python stubs after changing the proto:

```bash
python3 -m pip install --requirement app/embedding/service/requirements-dev.txt
app/embedding/service/generate_proto.sh
```

Run the service and rebuild a versioned Milvus collection before starting the
MQ consumer:

```bash
python3 app/embedding/service/server.py
go run ./app/embedding/mq/cmd/rebuild -f app/embedding/mq/etc/embedding-consumer.yaml
go run ./app/embedding/mq -f app/embedding/mq/etc/embedding-consumer.yaml
```

The rebuild command reads published posts through Content RPC, retries bounded
transient failures, verifies the new collection row count, then atomically
promotes `xbh_post_embeddings_current`. It does not delete the previous target,
so rollback is an alias switch.
