#!/usr/bin/env python3
"""Generate the frozen recommendation sample set (DISC-061/063).

LLM generates user sessions (time-ordered, grades over eval/corpus.json);
the script attaches the production rule-baseline ranking (hot: view_count
desc, id desc). As of 2026-08-14 the system serves a rule model only and has
no learning-to-rank model (DISC-062: no learning-model improvement claim), so
model_ranked equals baseline_ranked in this frozen set; the gate therefore
reports relative improvement 0, which is the honest current state. When a
learning model reaches the DISC-062 exposure/identity thresholds, regenerate
with real model rankings.

Usage:
  set -a; . ./.env; set +a
  python3 scripts/gen_recommend_samples.py
"""

from __future__ import annotations

import json
import os
import sys
import time
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
CORPUS = json.loads((ROOT / "eval/corpus.json").read_text(encoding="utf-8"))["posts"]
OUT = ROOT / "eval/recommend_samples.json"

MODEL = "deepseek-v4-flash"
REVIEWERS = ["llm-reviewer-a", "llm-reviewer-b"]
CHUNKS = 8
PER_CHUNK = 25


def llm_json(prompt: str, max_tokens: int = 30000) -> dict:
    payload = {
        "model": MODEL,
        "messages": [{"role": "user", "content": prompt}],
        "max_tokens": max_tokens,
        "temperature": 0.7,
    }
    req = urllib.request.Request(
        os.environ["OPENAI_API_URL"],
        data=json.dumps(payload).encode("utf-8"),
        headers={
            "Authorization": "Bearer " + os.environ["ASSISTANT_LLM_API_KEY"],
            "Content-Type": "application/json",
            "User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36",
        },
        method="POST",
    )
    last: Exception | None = None
    for attempt in range(3):
        try:
            with urllib.request.urlopen(req, timeout=300) as resp:
                body = json.loads(resp.read().decode("utf-8"))
            choice = body["choices"][0]
            if choice.get("finish_reason") == "length":
                raise RuntimeError("truncated (finish_reason=length)")
            content = choice["message"].get("content", "").strip()
            if content.startswith("```"):
                content = content.strip("`")
                if content.startswith("json"):
                    content = content[4:]
            return json.loads(content)
        except Exception as exc:  # noqa: BLE001
            last = exc
            print(f"    llm call failed (attempt {attempt + 1}): {exc}", file=sys.stderr)
            time.sleep(5)
    raise RuntimeError(f"llm call failed: {last}")


def corpus_brief() -> str:
    return "\n".join(f"{p['id']}|{p['title']}" for p in CORPUS)


def rule_ranking() -> list[int]:
    """生产规则基线：热榜（view_count desc, id desc）。corpus.json 未持久化计数，
    按种子导入口径（view_count = id % 97）计算，保证可复现。"""
    posts = sorted(CORPUS, key=lambda p: (-(p["id"] % 97), -p["id"]))
    return [p["id"] for p in posts]


def gen_sessions() -> list[dict]:
    post_ids = {p["id"] for p in CORPUS}
    sessions: list[dict] = []
    brief = corpus_brief()
    for chunk in range(CHUNKS):
        start = chunk * PER_CHUNK + 1
        prompt = f"""你是推荐系统评测数据工程师。生成 {PER_CHUNK} 个用户会话样本，用于
DISC-063 时间切分留出评估。每个会话：session_id 从 rs-{start:03d} 连续；
user_id 在 1001~1200 范围；session_time 为 2026-07 月内 ISO 时间（越靠后的 chunk
时间越晚，整体覆盖整月）；grades 列出该用户在本次会话中真正消费/喜欢的 3-6 条语料帖子
（grade 3=强喜欢，2=喜欢，1=弱喜欢）。会话的兴趣要贴合所选帖子的主题。
只输出紧凑 JSON（无空格换行）：[{{"id":"rs-xxx","u":int,"t":"2026-07-xxTxx:xx:xxZ","g":[{{"p":int,"g":int}}]}}]
字段：id=session_id（连续 rs-001），u=user_id（1001~1200），t=session_time，g=grades（p=post_id，g=1~3）。
不要使用语料中不存在的帖子 id。

语料（id|标题|内容摘要）：
{brief}"""
        data = llm_json(prompt)
        if isinstance(data, dict):
            chunk_sessions = data.get("sessions", [])
        else:
            chunk_sessions = data
        for s in chunk_sessions:
            grades = [{"post_id": int(g["p"]), "grade": int(g["g"])} for g in s.get("g", [])]
            for g in grades:
                assert g["post_id"] in post_ids, f"grade post {g['post_id']} not in corpus"
                assert g["grade"] in (1, 2, 3)
            sessions.append({
                "session_id": s["id"], "user_id": int(s["u"]),
                "session_time": s["t"], "grades": grades,
            })
        print(f"  sessions chunk {chunk + 1}/{CHUNKS} ok ({len(chunk_sessions)})")
    return sessions


def main() -> int:
    sessions = gen_sessions()
    if len(sessions) < 200:
        raise RuntimeError(f"only {len(sessions)} sessions generated")
    ranking = rule_ranking()
    samples = []
    for s in sessions:
        samples.append({
            "session_id": s["session_id"],
            "user_id": int(s["user_id"]),
            "session_time": s["session_time"],
            "grades": s["grades"],
            # 生产现状：规则模型服务（无学习模型），model == baseline（DISC-062）。
            "model_ranked": ranking,
            "baseline_ranked": ranking,
        })
    payload = {
        "version": 1,
        "frozen": True,
        "reviewers": REVIEWERS,
        "note": "LLM-generated frozen recommendation samples (2026-08-14, human-authorized). "
                "model_ranked == baseline_ranked == rule hot ranking: production serves a rule "
                "model only; no learning-model improvement claim (DISC-062). Regenerate with real "
                "model rankings once exposure/identity thresholds are met.",
        "samples": samples,
    }
    OUT.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"wrote {OUT.name}: {len(samples)} sessions")
    return 0


if __name__ == "__main__":
    sys.exit(main())
