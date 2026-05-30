"""Thin shim for the esx agent harness CLI.

The implementation lives in the ``scripts/_agent_harness`` package. This shim
keeps ``python3 scripts/agent_harness.py <command>`` working.
"""

from __future__ import annotations

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from _agent_harness.cli import main  # noqa: E402

if __name__ == "__main__":
    raise SystemExit(main())
