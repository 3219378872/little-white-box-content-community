from __future__ import annotations

import argparse
from pathlib import Path

from _knowledge_base.checks import (
    check_knowledge_base,
    render_sync_report,
    stale_pages,
)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Validate the esx knowledge base."
    )
    parser.add_argument("--repo-root", default=".")
    sub = parser.add_subparsers(dest="command", required=True)
    check_p = sub.add_parser("check", help="Validate knowledge base pages.")
    check_p.add_argument(
        "--base",
        default=None,
        help="Git base ref for the K005 co-change check (skipped when absent).",
    )
    report_p = sub.add_parser(
        "sync-report",
        help="Write the staleness report consumed by the weekly sync workflow.",
    )
    report_p.add_argument(
        "--output",
        required=True,
        help="Path to write the markdown staleness report to.",
    )
    args = parser.parse_args(argv)
    repo_root = Path(args.repo_root).resolve()
    if args.command == "sync-report":
        report = render_sync_report(stale_pages(repo_root))
        output = Path(args.output)
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(report, encoding="utf-8")
        print(report, end="")
        return 0
    if args.command == "check":
        issues = check_knowledge_base(repo_root, base_ref=args.base)
        if issues:
            for issue in issues:
                print(f"[{issue.rule_id}] {issue.path}: {issue.message}")
            return 1
        if args.base is None:
            print("knowledge base check passed (K005 skipped: no --base)")
        else:
            print("knowledge base check passed")
        return 0
    raise AssertionError(f"unhandled command: {args.command}")
