import io
import json
import tempfile
import unittest
from pathlib import Path

from algorithm.model_registry import load_manifest
from algorithm.offline_train.registry import S3ModelRegistry
from algorithm.online_infer.model_loader import load_registered_model


class FakeS3Client:
    def __init__(self) -> None:
        self.objects: dict[tuple[str, str], bytes] = {}

    def upload_file(self, filename, bucket, key):
        self.objects[(bucket, key)] = Path(filename).read_bytes()

    def put_object(self, *, Bucket, Key, Body, **_kwargs):
        self.objects[(Bucket, Key)] = bytes(Body)

    def get_object(self, *, Bucket, Key):
        return {"Body": io.BytesIO(self.objects[(Bucket, Key)])}

    def download_file(self, bucket, key, filename):
        Path(filename).write_bytes(self.objects[(bucket, key)])


class ModelRegistryTest(unittest.TestCase):
    def create_registered_model(self, directory: str, client: FakeS3Client):
        model_path = Path(directory) / "ranker.json"
        model_path.write_text(
            json.dumps({"weights": [2.0, 1.0], "bias": 0.25}), encoding="utf-8"
        )
        metadata_path = Path(str(model_path) + ".meta.json")
        metadata_path.write_text(
            json.dumps(
                {
                    "model_type": "linear-json",
                    "model_version": "rank-v2",
                    "feature_version": "v2",
                    "feature_names": ["quality", "coarse_score"],
                }
            ),
            encoding="utf-8",
        )
        registry = S3ModelRegistry(bucket="models", client=client)
        return registry.register(
            version="rank-v2",
            model_path=model_path,
            metadata_path=metadata_path,
            metrics={"auc": 0.8, "ndcg_at_k": 0.7},
            training_window={
                "feature_start": "2026-01-01T00:00:00+00:00",
                "sample_start": "2026-01-08T00:00:00+00:00",
                "sample_end": "2026-01-15T00:00:00+00:00",
            },
            feature_version="v2",
            status="candidate",
        )

    def test_registry_manifest_loads_verified_model(self):
        client = FakeS3Client()
        with tempfile.TemporaryDirectory() as directory:
            registered = self.create_registered_model(directory, client)
            manifest = load_manifest(registered.manifest_uri, s3_client=client)
            loaded = load_registered_model("rank-v2", registered.manifest_uri, s3_client=client)

        self.assertEqual("rank-v2", manifest.model_version)
        self.assertEqual("candidate", manifest.status)
        self.assertEqual(("quality", "coarse_score"), loaded.feature_names)
        self.assertEqual([1.75], loaded.model.predict([{"quality": 0.5, "coarse_score": 0.5}]))

    def test_tampered_registry_artifact_is_rejected(self):
        client = FakeS3Client()
        with tempfile.TemporaryDirectory() as directory:
            registered = self.create_registered_model(directory, client)
            manifest = load_manifest(registered.manifest_uri, s3_client=client)
            _, model_key = manifest.model_uri.removeprefix("s3://").split("/", 1)
            client.objects[("models", model_key)] += b"tampered"

            with self.assertRaisesRegex(ValueError, "SHA-256 mismatch"):
                load_registered_model("rank-v2", registered.manifest_uri, s3_client=client)

    def test_manifest_version_mismatch_is_rejected(self):
        client = FakeS3Client()
        with tempfile.TemporaryDirectory() as directory:
            registered = self.create_registered_model(directory, client)
            with self.assertRaisesRegex(ValueError, "does not match requested version"):
                load_registered_model("rank-v1", registered.manifest_uri, s3_client=client)


if __name__ == "__main__":
    unittest.main()
