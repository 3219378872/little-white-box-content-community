FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    HF_HOME=/var/lib/embedding/huggingface

WORKDIR /srv/embedding

COPY requirements.txt ./
RUN pip install --no-cache-dir --retries 10 --timeout 120 --requirement requirements.txt \
    && useradd --create-home --uid 10001 embedding \
    && mkdir -p "$HF_HOME" \
    && chown -R embedding:embedding /srv/embedding "$HF_HOME"

COPY --chown=embedding:embedding embedding_pb2.py embedding_pb2_grpc.py server.py ./

USER embedding
EXPOSE 50051
ENTRYPOINT ["python", "server.py"]
