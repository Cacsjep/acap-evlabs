#!/usr/bin/env python3
"""Smoke-test the public /playvoice endpoint.

Usage:
    python scripts/test_playvoice.py http://10.0.0.48 "Hello camera"
    python scripts/test_playvoice.py --key SECRET http://10.0.0.48 "Hello"

Designed to be runnable without any cross-compile tooling — point it at a
camera that already has Voicer installed.
"""
from __future__ import annotations

import argparse
import json
import sys
import urllib.error
import urllib.request


def main() -> int:
    p = argparse.ArgumentParser()
    p.add_argument("base", help="camera base URL, e.g. http://10.0.0.48")
    p.add_argument("text", help="text to speak")
    p.add_argument("--voice", default=None, help="voice ID override")
    p.add_argument("--format", default=None, help="output_format override")
    p.add_argument("--key", default=None, help="X-Voicer-Key shared secret if configured")
    p.add_argument("--dry", action="store_true", help="dry_run (synthesise but don't play)")
    args = p.parse_args()

    payload: dict[str, object] = {"text": args.text, "dry_run": args.dry}
    if args.voice:
        payload["voice_id"] = args.voice
    if args.format:
        payload["output_format"] = args.format

    url = args.base.rstrip("/") + "/local/voicer/playvoice"
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(url, data=data, headers={"Content-Type": "application/json"})
    if args.key:
        req.add_header("X-Voicer-Key", args.key)

    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            print(resp.read().decode("utf-8"))
            return 0
    except urllib.error.HTTPError as e:
        print(f"HTTP {e.code}: {e.read().decode('utf-8', 'replace')}", file=sys.stderr)
        return 1
    except urllib.error.URLError as e:
        print(f"network error: {e}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
