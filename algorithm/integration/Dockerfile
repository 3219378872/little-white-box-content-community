FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PYTHONPATH=/srv/app

WORKDIR /srv/app
RUN apt-get update \
    && apt-get install --yes --no-install-recommends libgomp1 \
    && rm -rf /var/lib/apt/lists/*
COPY algorithm/offline_train/requirements.txt /tmp/offline-requirements.txt
COPY algorithm/online_infer/requirements.txt /tmp/online-requirements.txt
RUN pip install --no-cache-dir --retries 10 --timeout 120 \
    -r /tmp/offline-requirements.txt -r /tmp/online-requirements.txt
COPY algorithm /srv/app/algorithm

CMD ["python", "-m", "unittest", "discover", "-s", "algorithm", "-p", "test*.py", "-v"]
