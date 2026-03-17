# Sam's Claude Menagerie

A collection of Claude Code plugins by [Sam Phinizy](https://github.com/sam-phinizy).

## Plugins

| Plugin | Description | Repo |
|--------|-------------|------|
| **joan** | Local code review gate for AI agents via Forgejo. Agents push work to a local Forgejo instance, a human reviews it, and only approved work gets pushed upstream. | [sam-phinizy/joan](https://github.com/sam-phinizy/joan) |
| **remindctl** | Apple Reminders integration via remindctl-mcp. Full CRUD on reminders, plus a text-to-reminders skill that parses emails, Slack threads, and meeting notes into structured reminders with smart due dates. | [sam-phinizy/remindctl-mcp](https://github.com/sam-phinizy/remindctl-mcp) |
| **claude-obsidian-thinker** | Lightweight session context management via Obsidian. Writes session summaries, project notes, daily logs, and a searchable knowledge base. Commands: `/closeout`, `/recall`, `/wakeup`, `/pickup`. | [sam-phinizy/claude-obsidian-thinker](https://github.com/sam-phinizy/claude-obsidian-thinker) |

## Installation

Inside Claude Code, add this marketplace:

```
/plugin marketplace add sam-phinizy/sams-claude-menagerie
```

Then install individual plugins:

```
/plugin install joan@sams-claude-menagerie
/plugin install remindctl@sams-claude-menagerie
/plugin install claude-obsidian-thinker@sams-claude-menagerie
```
