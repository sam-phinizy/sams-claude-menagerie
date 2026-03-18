#!/usr/bin/env python3
import json

with open(".claude-plugin/marketplace.json") as f:
    data = json.load(f)

plugins = data["plugins"]

lines = []
lines.append("# Sam's Claude Menagerie\n")
lines.append("A collection of Claude Code plugins by [Sam Phinizy](https://github.com/sam-phinizy).\n")
lines.append("## Plugins\n")
lines.append("| Plugin | Version | Description | Repo |")
lines.append("|--------|---------|-------------|------|")

for p in plugins:
    name = p["name"]
    desc = p["description"]
    repo = p["source"]["repo"]
    version = p.get("version", "latest")
    lines.append(f"| **{name}** | {version} | {desc} | [{repo}](https://github.com/{repo}) |")

lines.append("")
lines.append("## Installation\n")
lines.append("Inside Claude Code, add this marketplace:\n")
lines.append("```")
lines.append("/plugin marketplace add sam-phinizy/sams-claude-menagerie")
lines.append("```\n")
lines.append("Then install individual plugins:\n")
lines.append("```")
for p in plugins:
    lines.append(f"/plugin install {p['name']}@sams-claude-menagerie")
lines.append("```")

with open("README.md", "w") as f:
    f.write("\n".join(lines) + "\n")

print("README.md generated")
