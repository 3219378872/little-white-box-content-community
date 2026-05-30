"""Thin shim for the esx knowledge base checker.

The implementation lives in ``scripts/_knowledge_base``. This shim keeps
``python3 scripts/knowledge_base.py check`` working.
"""

from __future__ import annotations

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from _knowledge_base.cli import main  # noqa: E402

if __name__ == "__main__":
    raise SystemExit(main())
