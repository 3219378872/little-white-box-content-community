from __future__ import annotations

import argparse
import sys
from pathlib import Path

from _agent_harness import gc, lifecycle
from _agent_harness.checks import check_task
from _agent_harness.model import HarnessError


def _task_dirs(root: Path) -> list[Path]:
    if not root.exists():
        return []
    return sorted(p for p in root.iterdir() if p.is_dir())


def _check(repo_root: Path, include_completed: bool) -> int:
    issues = []
    for task_dir in _task_dirs(lifecycle.active_root(repo_root)):
        issues.extend(check_task(task_dir))
    if include_completed:
        for task_dir in _task_dirs(lifecycle.completed_root(repo_root)):
            issues.extend(check_task(task_dir, historical=True))
    if issues:
        for issue in issues:
            print(f"{issue.task}: [{issue.rule_id}] {issue.file}: {issue.message}")
        return 1
    print("agent harness check passed")
    return 0


def _summary(repo_root: Path) -> int:
    summary = gc.build_summary(repo_root)
    print("Active tasks:")
    for task in summary.active_tasks:
        print(f"- {task}")
    print(f"Completed tasks: {summary.completed_count}")
    print(f"Check issues: {summary.issue_count}")
    print("GC notes:")
    for note in summary.gc_notes:
        print(f"- {note}")
    print(f"Specs without adjacent plans: {len(summary.specs_without_plans)}")
    for path in summary.specs_without_plans[:10]:
        print(f"- {path}")
    print(f"Plans without task references: {len(summary.plans_without_task_refs)}")
    for path in summary.plans_without_task_refs[:10]:
        print(f"- {path}")
    print(f"Next action: {summary.recommended_next_action}")
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Manage esx agent harness task records."
    )
    parser.add_argument("--repo-root", default=".")
    sub = parser.add_subparsers(dest="command", required=True)

    new_p = sub.add_parser("new", help="Create an active task record.")
    new_p.add_argument("slug")
    new_p.add_argument("--date", help="Override date for deterministic task names.")

    check_p = sub.add_parser("check", help="Check task records.")
    check_p.add_argument(
        "--include-completed",
        action="store_true",
        help="Also check completed task records.",
    )

    sub.add_parser("summary", help="Summarize harness task state.")

    complete_p = sub.add_parser("complete", help="Complete a merged task.")
    complete_p.add_argument("slug")

    abandon_p = sub.add_parser("abandon", help="Abandon a task.")
    abandon_p.add_argument("slug")
    abandon_p.add_argument("--reason", required=True)

    args = parser.parse_args(argv)
    repo_root = Path(args.repo_root).resolve()
    try:
        if args.command == "new":
            target = lifecycle.new_task(repo_root, args.slug, date=args.date)
            print(target.relative_to(repo_root))
            return 0
        if args.command == "check":
            return _check(repo_root, args.include_completed)
        if args.command == "summary":
            return _summary(repo_root)
        if args.command == "complete":
            target = lifecycle.complete_task(repo_root, args.slug)
            print(target.relative_to(repo_root))
            return 0
        if args.command == "abandon":
            target = lifecycle.abandon_task(repo_root, args.slug, reason=args.reason)
            print(target.relative_to(repo_root))
            return 0
    except HarnessError as exc:
        print(str(exc), file=sys.stderr)
        return 2
    raise AssertionError(f"unhandled command: {args.command}")
