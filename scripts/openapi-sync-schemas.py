#!/usr/bin/env python3
"""Merge generated response schemas into backend/internal/openapi/openapi.yaml.

    cd backend && FLEETDOCK_GEN_SCHEMAS=/tmp/schemas.json \\
      go test ./internal/interfaces/httpapi/ -run GenerateSchemas -count=1
    python3 scripts/openapi-sync-schemas.py /tmp/schemas.json

For every schema the Go test pairs with a response type (documentedResponses
in openapi_schema_test.go): properties the handlers no longer return are
dropped, new ones are added from the generated schema, and `required` is
replaced. Properties already documented keep their definition (descriptions,
enums, examples). Only those schema blocks are rewritten; the rest of the
file is left exactly as it is. Needs PyYAML.
"""
import json
import re
import sys
from pathlib import Path

import yaml

SPEC = Path(__file__).resolve().parent.parent / "backend/internal/openapi/openapi.yaml"
BLOCK = re.compile(r"^    ([A-Za-z0-9_]+):\s*$")


KEY_ORDER = ["$ref", "type", "format", "enum", "items", "additionalProperties", "nullable", "default", "description"]


def tidy(value):
    """Canonical key order, and no trailing newlines from folded scalars."""
    if isinstance(value, dict):
        keys = sorted(value, key=lambda k: (KEY_ORDER.index(k) if k in KEY_ORDER else len(KEY_ORDER), k))
        return {k: tidy(value[k]) for k in keys}
    if isinstance(value, list):
        return [tidy(v) for v in value]
    if isinstance(value, str):
        return value.strip()
    return value


def flow(value) -> str:
    return yaml.safe_dump(tidy(value), default_flow_style=True, width=10**9, sort_keys=False, allow_unicode=True).strip()


def emit(name: str, schema: dict) -> list[str]:
    out = [f"    {name}:", "      type: object"]
    for key, value in schema.items():
        if key in ("type", "properties", "required"):
            continue
        text = yaml.safe_dump({key: tidy(value)}, default_flow_style=False, sort_keys=False, width=88, allow_unicode=True)
        out += [("      " + line).rstrip() for line in text.rstrip("\n").split("\n")]
    if schema.get("required"):
        out.append(f"      required: {flow(schema['required'])}")
    out.append("      properties:")
    for prop, value in schema.get("properties", {}).items():
        out.append(f"        {prop}: {flow(value)}")
    return out


def main(generated_path: str) -> None:
    generated = json.loads(Path(generated_path).read_text())
    lines = SPEC.read_text().split("\n")
    start = lines.index("  schemas:") + 1
    end = start
    while end < len(lines) and (lines[end] == "" or lines[end].startswith("    ")):
        end += 1
    # Split components.schemas into named blocks.
    blocks, order, current = {}, [], None
    for line in lines[start:end]:
        m = BLOCK.match(line)
        if m:
            current = m.group(1)
            order.append(current)
            blocks[current] = [line]
        elif current:
            blocks[current].append(line)
    for name in sorted(generated):
        gen = generated[name]
        if name in blocks:
            body = "\n".join(l[4:] for l in blocks[name])
            cur = yaml.safe_load(body)[name] or {}
        else:
            cur = {}
            order.append(name)
        props = cur.get("properties") or {}
        merged = {p: props.get(p, gen["properties"][p]) for p in props if p in gen["properties"]}
        for p, s in gen["properties"].items():
            merged.setdefault(p, s)
        dropped = [p for p in props if p not in gen["properties"]]
        added = [p for p in gen["properties"] if p not in props]
        if dropped or added or name not in blocks:
            print(f"{name}: +{added} -{dropped}")
        cur["properties"] = merged
        if gen.get("required"):
            cur["required"] = gen["required"]
        else:
            cur.pop("required", None)
        trailing = [""] if blocks.get(name, [""])[-1] == "" else []
        blocks[name] = emit(name, cur) + trailing
    section = []
    for name in order:
        section += blocks[name]
    SPEC.write_text("\n".join(lines[:start] + section + lines[end:]))


if __name__ == "__main__":
    main(sys.argv[1])
