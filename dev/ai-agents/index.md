# For AI agents

AI agents often write the code that uses a library, and a model trained before tinyconf v2 does not know its API: it writes `Parse(conf)` without the context or imports `github.com/insei/tinyconf` without `/v2`. Give the agent the current documentation.

## The skill

[`skills/tinyconf/SKILL.md`](https://github.com/Insei/tinyconf/blob/main/skills/tinyconf/SKILL.md) is a short guide for agents in the [Agent Skills](https://agentskills.io) format: the rules, complete programs, the drivers with their options and the common mistakes. It is versioned with the code, and CI builds its programs against the code of the repository, so it does not describe the API that does not exist.

Claude Code finds the skill in the `.claude/skills` directory of the project or of the user:

```sh
mkdir -p .claude/skills/tinyconf
curl -fsSL https://raw.githubusercontent.com/Insei/tinyconf/main/skills/tinyconf/SKILL.md \
  -o .claude/skills/tinyconf/SKILL.md
```

Replace `main` with the release tag, `v2.1.0` for example, to get the skill of the version in your `go.mod`. Other agents read it from any path: mention it in `AGENTS.md` or in the instructions of the agent.

The skill describes the library, not your project. Keep the rules of your project — which drivers you use, where the configs live, how they are registered — in your own instructions next to it.

## llms.txt

The documentation is published in the [llms.txt](https://llmstxt.org) format for every version:

- [`llms.txt`](https://insei.github.io/tinyconf/latest/llms.txt) — the index of the pages with descriptions;
- [`llms-full.txt`](https://insei.github.io/tinyconf/latest/llms-full.txt) — the whole documentation as one Markdown file, to put into the context of the agent.

Replace `latest` with the version, `2.1` for example, or with `dev` for the documentation of the `main` branch.
