#!/usr/bin/env python3
"""Print expectancy by method from closes.jsonl. Never writes weights.json."""

from __future__ import annotations

import json
import sys
from collections import defaultdict
from pathlib import Path


def method_of(strategy_id: str) -> str:
    parts = (strategy_id or "").split("_")
    if len(parts) >= 2:
        return "_".join(parts[:-1]) if parts[-1] in {"1d", "1w", "1h", "5m"} or parts[-1].isdigit() else "_".join(parts[:-1])
    return strategy_id or "unknown"


def load(path: Path) -> list[dict]:
    if not path.exists():
        return []
    rows = []
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line:
            continue
        rows.append(json.loads(line))
    return rows


def table(closes: list[dict]) -> str:
    agg: dict[str, dict[str, float]] = defaultdict(
        lambda: {"n": 0, "wins": 0, "pnl": 0.0, "excess": 0.0, "hold": 0.0, "mae": 0.0, "mfe": 0.0}
    )
    for c in closes:
        m = c.get("method") or method_of(c.get("strategyId", ""))
        a = agg[m]
        a["n"] += 1
        pnl = float(c.get("pnl") or 0)
        if pnl > 0:
            a["wins"] += 1
        a["pnl"] += pnl
        a["excess"] += float(c.get("excess") or 0)
        a["hold"] += float(c.get("holdMs") or 0)
        a["mae"] += float(c.get("mae") or 0)
        a["mfe"] += float(c.get("mfe") or 0)
    lines = [
        "method               n   expectancy     excess  win_rate  hold_h      mae      mfe",
    ]
    if not agg:
        lines.append("(no closes)")
        return "\n".join(lines) + "\n"
    for name in sorted(agg):
        a = agg[name]
        n = a["n"]
        lines.append(
            f"{name[:18]:<18} {int(n):5d} {a['pnl']/n:12.2f} {a['excess']/n:10.2f} "
            f"{a['wins']/n:9.2f} {(a['hold']/n)/3_600_000:7.1f} {a['mae']/n:8.4f} {a['mfe']/n:8.4f}"
        )
    return "\n".join(lines) + "\n"


def main() -> int:
    if len(sys.argv) > 1:
        path = Path(sys.argv[1])
    else:
        path = Path("data/learning/closes.jsonl")
    sys.stdout.write(table(load(path)))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
