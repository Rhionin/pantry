#!/usr/bin/env python3

from __future__ import annotations

import os
import sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

SKIP_DIRS = {
    ".git",
    "node_modules",
    "dist",
    "coverage",
    ".bin",
    "evidence",
}

SKIP_WORD_FILES = {
    "frontend/src/components/shopping/ShoppingListPage.test.tsx",
    "frontend/src/components/inventory/AddToGroupSheet.test.tsx",
    "scripts/rename-to-stockbot.py",
    "scripts/check-stockbot-names.sh",
}

TEXT_EXT = {
    ".go", ".md", ".html", ".ts", ".tsx", ".js", ".mjs", ".sh", ".yml", ".yaml",
    ".json", ".svg", ".txt", ".mod", ".css", ".toml", ".env",
}

BARE_NAMES = {
    "Dockerfile", "Dockerfile.grocy-sync", "Caddyfile", "Caddyfile.tunnel",
    "AGENTS.md", ".env.example", ".gitignore",
}

PROTECTED = (
    "X-Pantry-",
    "bcrypt Pantry",
    "named Pantry",
    "the Pantry location",
    "other than Pantry",
    "wiki/Pantry",
    "Pantry item",
    "Pantry beans",
)


def rel(path: str) -> str:
    return os.path.relpath(path, ROOT).replace(os.sep, "/")


def mechanical(text: str) -> str:
    text = text.replace("github.com/Rhionin/pantry", "github.com/Rhionin/stockbot")
    text = text.replace("Rhionin/pantry", "Rhionin/stockbot")
    text = text.replace("com.rhionin.pantry", "com.rhionin.stockbot")
    text = text.replace(".agents/skills/verify-pantry", ".agents/skills/verify-stockbot")
    text = text.replace("pantry-verify.sh", "stockbot-verify.sh")
    text = text.replace("pantry-server", "stockbot-server")
    return text


def product_copy(text: str) -> str:
    tokens = []
    for phrase in PROTECTED:
        token = f"\x00PROT{len(tokens)}\x00"
        if phrase in text:
            text = text.replace(phrase, token)
        tokens.append((token, phrase))
    text = text.replace("Pantry", "Stockbot")
    for token, phrase in tokens:
        text = text.replace(token, phrase)
    return text


def should_word(path: str) -> bool:
    r = rel(path)
    if r.endswith(".go") or r.startswith(".kiro/") or r == "AGENTS.md":
        return False
    if r in SKIP_WORD_FILES:
        return False
    return True


def walk():
    for dirpath, dirnames, filenames in os.walk(ROOT):
        dirnames[:] = [d for d in dirnames if d not in SKIP_DIRS and d != ".git"]
        for name in filenames:
            path = os.path.join(dirpath, name)
            ext = os.path.splitext(name)[1]
            if ext in TEXT_EXT or name in BARE_NAMES or name.startswith("Dockerfile"):
                yield path


def main() -> int:
    mode = sys.argv[1] if len(sys.argv) > 1 else "all"
    changed = 0
    for path in walk():
        try:
            with open(path, "r", encoding="utf-8") as fh:
                original = fh.read()
        except (UnicodeDecodeError, OSError):
            continue
        updated = mechanical(original)
        if mode == "all" and should_word(path):
            updated = product_copy(updated)
        if updated != original:
            with open(path, "w", encoding="utf-8", newline="") as fh:
                fh.write(updated)
            changed += 1
            print(rel(path))
    print(f"updated {changed} files")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
