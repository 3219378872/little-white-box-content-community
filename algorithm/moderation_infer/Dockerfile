FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PYTHONPATH=/srv/app

WORKDIR /srv/app
COPY algorithm/moderation_infer/requirements.txt /tmp/requirements.txt
RUN pip install --no-cache-dir --retries 10 --timeout 120 -r /tmp/requirements.txt
COPY algorithm/__init__.py /srv/app/algorithm/__init__.py
COPY algorithm/moderation_infer /srv/app/algorithm/moderation_infer

EXPOSE 9026
CMD ["python", "-m", "algorithm.moderation_infer.server"]
