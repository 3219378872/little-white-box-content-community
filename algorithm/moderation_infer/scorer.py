"""Placeholder Ranker for the review cascade (DES-review-platform).

stub-v0 returns 0.5 for every requested issue so any task with Router
candidates lands in the gray zone and goes to human review. Its version string
contains "stub", which RVW-018 requires so nobody can claim accuracy,
automation or leakage rates from it.

The fixture mode is only for tests: when enabled, a text marker such as
``[[fixture:CONTENT.SELF_HARM=0.97]]`` sets that issue's p_yes.
"""
from __future__ import annotations

import re
from dataclasses import dataclass

STUB_VERSION = "stub-v0"
FIXTURE_VERSION = "stub-v0+fixture"
_FIXTURE = re.compile(r"\[\[fixture:([A-Z_]+\.[A-Z_]+)=([01](?:\.\d+)?)\]\]")


@dataclass(frozen=True)
class IssueScore:
    issue: str
    p_yes: float
    p_no: float


def score(text: str, issues: list[str], fixture_enabled: bool) -> tuple[str, list[IssueScore]]:
    if len(set(issues)) != len(issues):
        raise ValueError("duplicate issues")
    overrides: dict[str, float] = {}
    if fixture_enabled:
        for issue, value in _FIXTURE.findall(text):
            probability = float(value)
            if 0.0 <= probability <= 1.0:
                overrides[issue] = probability
    version = FIXTURE_VERSION if fixture_enabled else STUB_VERSION
    scores = []
    for issue in issues:
        p_yes = overrides.get(issue, 0.5)
        scores.append(IssueScore(issue=issue, p_yes=p_yes, p_no=round(1.0 - p_yes, 6)))
    return version, scores
