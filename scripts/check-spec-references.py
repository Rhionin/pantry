#!/usr/bin/env python3
"""Check that spec cross-references resolve.

No spec format validator ships with Kiro, so this covers the failure mode that
actually bites: a design or task document citing an acceptance criterion or a
correctness property that does not exist, usually after a renumbering.

For every spec under .kiro/specs/:
  - every `Requirement N.M` / `(N.M)` reference in design.md and tasks.md must
    name a criterion that exists in requirements.md
  - every `_Requirements: N.M_` trailer in tasks.md must resolve
  - every `_Properties: N_` trailer in tasks.md must name a property in design.md
  - every `**Validates: Requirements ...**` trailer in design.md must resolve

Usage: scripts/check-spec-references.py [spec-name ...]
Exits non-zero when a reference dangles.
"""

import re
import sys
from pathlib import Path

SPECS = Path(__file__).resolve().parent.parent / ".kiro" / "specs"

REQ_HEADING = re.compile(r"^#{2,4} Requirement (\d+)\b", re.M)
CRITERION = re.compile(r"^(\d+)\. ", re.M)
PROPERTY_HEADING = re.compile(r"^### Property (\d+):", re.M)
# "Requirement 17.4", "Requirements 7.1, 7.2", "(12.5)", "17.8"
REF = re.compile(r"\b(\d{1,2})\.(\d{1,2})\b")
TRAILER_REQ = re.compile(r"_Requirements:([^_]*)_")
TRAILER_PROP = re.compile(r"_Properties:([^_]*)_")
VALIDATES = re.compile(r"\*\*Validates: Requirements?([^*]*)\*\*")


def criteria_index(requirements: str) -> dict[int, int]:
    """Map requirement number -> count of acceptance criteria."""
    blocks, index = [], {}
    matches = list(REQ_HEADING.finditer(requirements))
    for i, m in enumerate(matches):
        end = matches[i + 1].start() if i + 1 < len(matches) else len(requirements)
        blocks.append((int(m.group(1)), requirements[m.start():end]))
    for number, body in blocks:
        nums = [int(n) for n in CRITERION.findall(body)]
        index[number] = max(nums) if nums else 0
    return index


def check(spec_dir: Path) -> list[str]:
    problems: list[str] = []
    req_path = spec_dir / "requirements.md"
    if not req_path.exists():
        return problems  # bugfix-style specs carry no requirements.md

    index = criteria_index(req_path.read_text())
    design = spec_dir / "design.md"
    properties = set()
    if design.exists():
        properties = {int(n) for n in PROPERTY_HEADING.findall(design.read_text())}

    def resolve(req: int, crit: int, where: str, context: str) -> None:
        if req not in index:
            problems.append(f"{where}: cites Requirement {req}.{crit}, but Requirement {req} does not exist  [{context}]")
        elif crit < 1 or crit > index[req]:
            problems.append(
                f"{where}: cites Requirement {req}.{crit}, but Requirement {req} has criteria 1..{index[req]}  [{context}]"
            )

    for name in ("design.md", "tasks.md"):
        path = spec_dir / name
        if not path.exists():
            continue
        for lineno, line in enumerate(path.read_text().splitlines(), 1):
            where = f"{spec_dir.name}/{name}:{lineno}"
            snippet = line.strip()[:70]

            for body in VALIDATES.findall(line) + TRAILER_REQ.findall(line):
                for req, crit in REF.findall(body):
                    resolve(int(req), int(crit), where, snippet)

            for body in TRAILER_PROP.findall(line):
                for n in re.findall(r"\d+", body):
                    if int(n) not in properties:
                        problems.append(f"{where}: _Properties: {n}_ names no property in design.md  [{snippet}]")

            # Prose references, only where the word Requirement makes intent explicit.
            for m in re.finditer(r"Requirements?\s+((?:\d{1,2}\.\d{1,2}[,\s and]*)+)", line):
                for req, crit in REF.findall(m.group(1)):
                    resolve(int(req), int(crit), where, snippet)

    return problems


def main() -> int:
    wanted = sys.argv[1:]
    dirs = sorted(d for d in SPECS.iterdir() if d.is_dir() and (not wanted or d.name in wanted))
    if not dirs:
        print("no matching specs", file=sys.stderr)
        return 1

    failures = 0
    for spec_dir in dirs:
        problems = sorted(set(check(spec_dir)))
        status = "FAIL" if problems else "ok"
        print(f"[{status}] {spec_dir.name}")
        for p in problems:
            print(f"    {p}")
        failures += len(problems)

    print()
    print(f"{failures} dangling reference(s)")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
