FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PYTHONPATH=/srv/app

WORKDIR /srv/app
RUN apt-get update \
    && apt-get install --yes --no-install-recommends libgomp1 \
    && rm -rf /var/lib/apt/lists/*
COPY algorithm/online_infer/requirements.txt /tmp/requirements.txt
RUN pip install --no-cache-dir --retries 10 --timeout 120 -r /tmp/requirements.txt
COPY algorithm /srv/app/algorithm

EXPOSE 9025
CMD ["python", "-m", "algorithm.online_infer.server"]
