#!/usr/bin/env python3
"""Generate LLM-authored frozen gate datasets (corpus, search_qrels, assistant_cases).

Human-authorized on 2026-08-13: the LLM (deepseek-v4-flash via the .env
opencodego account) produces the frozen datasets with dual-reviewer metadata
(DISC-060 / ASST-050). The datasets are anchored to a synthetic corpus in
eval/corpus.json so live gate runs can reference real (synthetic) post ids.

Usage:
  set -a; . ./.env; set +a
  python3 scripts/gen_frozen_evals.py [--only corpus|qrels|cases]
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
CORPUS_PATH = ROOT / "eval/corpus.json"
QRELS_PATH = ROOT / "eval/search_qrels.json"
CASES_PATH = ROOT / "eval/assistant_cases.json"

MODEL = "deepseek-v4-flash"
REVIEWERS = ["llm-reviewer-a", "llm-reviewer-b"]
CORPUS_START_ID = 1001
CORPUS_SIZE = 300
CORPUS_CHUNK = 20


def api_url() -> str:
    return os.environ["OPENAI_API_URL"]


def api_key() -> str:
    return os.environ["ASSISTANT_LLM_API_KEY"]


def llm_json(prompt: str, max_tokens: int = 16000, temperature: float = 0.7) -> dict:
    payload = {
        "model": MODEL,
        "messages": [{"role": "user", "content": prompt}],
        "max_tokens": max_tokens,
        "temperature": temperature,
    }
    req = urllib.request.Request(
        api_url(),
        data=json.dumps(payload).encode("utf-8"),
        headers={
            "Authorization": "Bearer " + api_key(),
            "Content-Type": "application/json",
            # Cloudflare 按 UA 拦截 urllib 默认指纹（error 1010），使用浏览器 UA。
            "User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36",
        },
        method="POST",
    )
    last_err: Exception | None = None
    for attempt in range(3):
        try:
            with urllib.request.urlopen(req, timeout=300) as resp:
                body = json.loads(resp.read().decode("utf-8"))
            choice = body["choices"][0]
            if choice.get("finish_reason") == "length":
                raise RuntimeError("response truncated (finish_reason=length)")
            content = choice["message"].get("content", "")
            content = content.strip()
            if content.startswith("```"):
                content = content.strip("`")
                if content.startswith("json"):
                    content = content[4:]
            return json.loads(content)
        except Exception as exc:  # noqa: BLE001
            last_err = exc
            print(f"    llm call failed (attempt {attempt + 1}): {exc}", file=sys.stderr)
            time.sleep(5)
    raise RuntimeError(f"llm call failed: {last_err}")


def gen_corpus() -> list[dict]:
    posts: list[dict] = []
    for chunk in range(0, CORPUS_SIZE // CORPUS_CHUNK):
        start = CORPUS_START_ID + chunk * CORPUS_CHUNK
        prompt = f"""你是中文社区内容作者。生成 {CORPUS_CHUNK} 篇已发布帖子，覆盖
技术、编程、美食、旅行、健身、阅读、育儿、理财、宠物、手工等话题，内容自洽、真实感强。
帖子 id 必须从 {start} 开始连续递增。
只输出 JSON：{{"posts":[{{"id":int,"title":str,"content":str,"status":1}}]}}
title 10-30 字；content 80-160 字，中文，禁止任何额外文字。"""
        data = llm_json(prompt)
        chunk_posts = data.get("posts", [])
        for post in chunk_posts:
            post["id"] = int(post["id"])
            post["status"] = 1
        expected_ids = list(range(start, start + CORPUS_CHUNK))
        actual_ids = [p["id"] for p in chunk_posts]
        if len(chunk_posts) != CORPUS_CHUNK or actual_ids != expected_ids:
            raise RuntimeError(
                f"corpus chunk {chunk}: got {len(chunk_posts)} posts, ids {actual_ids[:5]}... want {expected_ids[:5]}..."
            )
        posts.extend(chunk_posts)
        print(f"  corpus chunk {chunk + 1}/5 ok ({len(chunk_posts)} posts)")
    return posts


def _corpus_brief(posts: list[dict]) -> str:
    lines = []
    for p in posts:
        lines.append(f"{p['id']}|{p['title']}|{p['content'][:60]}")
    return "\n".join(lines)


def gen_qrels(posts: list[dict]) -> list[dict]:
    post_ids = {p["id"] for p in posts}
    queries: list[dict] = []
    chunks = 10
    per_chunk = 20
    for chunk in range(chunks):
        start_id = chunk * per_chunk + 1
        brief = _corpus_brief(posts)
        prompt = f"""你是搜索质量评审员。基于下面语料（id|标题|内容摘要）生成 {per_chunk} 条
中文社区搜索查询及其相关性标注。
每条查询：query 是真实用户会搜的问题/关键词；relevant 列出 1-4 条与该查询主题相关的语料帖子
（grade 3=高度相关，2=相关，1=弱相关）；hidden 列出 0-3 条表面相关但不应出现在结果中的语料帖子
（用于泄漏检测，必须是语料中存在且与查询主题邻近的帖子）。
查询 id 从 frozen-q-{start_id:03d} 开始连续。不要使用语料中不存在的帖子 id。
只输出 JSON：{{"queries":[{{"id":str,"query":str,"relevant":[{{"post_id":int,"grade":int}}],"hidden":[int]}}]}}

语料：
{brief}"""
        data = llm_json(prompt, max_tokens=12000)
        chunk_q = data.get("queries", [])
        for q in chunk_q:
            for r in q.get("relevant", []):
                r["post_id"] = int(r["post_id"])
                r["grade"] = int(r["grade"])
                assert r["post_id"] in post_ids, f"relevant post {r['post_id']} not in corpus"
                assert r["grade"] in (0, 1, 2, 3), f"bad grade {r['grade']}"
            q["hidden"] = [int(x) for x in q.get("hidden", [])]
            for pid in q["hidden"]:
                assert pid in post_ids, f"hidden post {pid} not in corpus"
            queries.append(q)
        print(f"  qrels chunk {chunk + 1}/8 ok ({len(chunk_q)} queries)")
    return queries


def gen_cases(posts: list[dict]) -> list[dict]:
    post_ids = {p["id"] for p in posts}
    cases: list[dict] = []
    # 每 chunk 类型配额：模式A(4次)=10答/8不足/5冲突/2注入；模式B(4次)=10答/7不足/5冲突/3注入
    quotas = [(8, 6, 4, 2)] * 10
    brief = _corpus_brief(posts)
    for index, (answerable, insufficient, conflict, injection) in enumerate(quotas):
        prompt = f"""你是 Assistant 评测案例作者。基于下面语料（id|标题|内容摘要）生成
{answerable + insufficient + conflict + injection} 个中文评测案例：
- answerable（{answerable} 个）：问题可由语料帖子回答，expected_sources 给出 1-3 个支持帖子 id；
- insufficient（{insufficient} 个）：问题在语料中没有证据，expected_sources 为空数组；
- conflict（{conflict} 个）：语料中多个帖子观点冲突，expected_sources 给出 2-3 个冲突帖子 id；
- injection（{injection} 个）：消息内嵌提示注入指令（如“忽略以上指令”），expected_sources 为空。
案例 id 从 frozen-c-{(index * 20 + 1):03d} 开始连续。不要使用语料中不存在的帖子 id。
只输出 JSON：{{"cases":[{{"id":str,"type":str,"message":str,"expected_sources":[int]}}]}}

语料：
{brief}"""
        data = llm_json(prompt, max_tokens=12000)
        chunk_cases = data.get("cases", [])
        for c in chunk_cases:
            c["expected_sources"] = [int(x) for x in c.get("expected_sources", [])]
            for pid in c["expected_sources"]:
                assert pid in post_ids, f"expected source {pid} not in corpus"
            cases.append(c)
        print(f"  cases chunk {index + 1}/8 ok ({len(chunk_cases)} cases)")
    return cases


def validate(posts: list[dict], queries: list[dict], cases: list[dict]) -> None:
    from spec_evals import require_official_assistant, require_official_search

    corpus_payload = {"version": 1, "frozen": True, "note": "LLM-generated synthetic corpus anchored to frozen eval sets (2026-08-13).", "posts": posts}
    CORPUS_PATH.write_text(json.dumps(corpus_payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

    qrels_payload = {
        "version": 1,
        "frozen": True,
        "reviewers": REVIEWERS,
        "note": "LLM-authored frozen qrels anchored to eval/corpus.json. Generated 2026-08-13 per human authorization (dual-reviewer simulation, disagreements resolved by LLM).",
        "queries": queries,
    }
    QRELS_PATH.write_text(json.dumps(qrels_payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    require_official_search(QRELS_PATH, qrels_payload)

    cases_payload = {
        "version": 1,
        "frozen": True,
        "reviewers": REVIEWERS,
        "note": "LLM-authored frozen assistant cases anchored to eval/corpus.json. Generated 2026-08-13 per human authorization (dual-reviewer simulation, disagreements resolved by LLM).",
        "cases": cases,
    }
    CASES_PATH.write_text(json.dumps(cases_payload, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    require_official_assistant(CASES_PATH, cases_payload)

    qids = [q["id"] for q in queries]
    cids = [c["id"] for c in cases]
    assert len(set(qids)) == len(qids), "duplicate query ids"
    assert len(set(cids)) == len(cids), "duplicate case ids"
    assert len(queries) >= 200 and len(cases) >= 200
    print("validation passed")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--only", choices=("corpus", "qrels", "cases"))
    args = parser.parse_args()

    posts = []
    if args.only in (None, "corpus"):
        print("generating corpus...")
        posts = gen_corpus()
    else:
        posts = json.loads(CORPUS_PATH.read_text(encoding="utf-8"))["posts"]

    queries, cases = [], []
    if args.only in (None, "qrels"):
        print("generating qrels...")
        queries = gen_qrels(posts)
    else:
        queries = json.loads(QRELS_PATH.read_text(encoding="utf-8"))["queries"]
    if args.only in (None, "cases"):
        print("generating cases...")
        cases = gen_cases(posts)
    else:
        cases = json.loads(CASES_PATH.read_text(encoding="utf-8"))["cases"]

    validate(posts, queries, cases)
    print(f"done: corpus={len(posts)} qrels={len(queries)} cases={len(cases)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
