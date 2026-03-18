#!/usr/bin/env python3
import json, os, sys

plugin_name = os.environ.get("PLUGIN", "")
tag = os.environ.get("TAG", "")

if not plugin_name or not tag:
    print("No plugin/tag provided, skipping version update")
    sys.exit(0)

with open(".claude-plugin/marketplace.json") as f:
    data = json.load(f)

updated = False
for p in data["plugins"]:
    if p["name"] == plugin_name:
        p["version"] = tag
        p["source"]["ref"] = tag
        updated = True
        break

if not updated:
    print(f"Plugin '{plugin_name}' not found in marketplace.json, skipping")
    sys.exit(0)

with open(".claude-plugin/marketplace.json", "w") as f:
    json.dump(data, f, indent=2)
    f.write("\n")

print(f"Updated {plugin_name} to {tag}")
