# Attribution

## Upstream `claude-nix`

This repository is a fork of
**[arianvp/claude-nix](https://github.com/arianvp/claude-nix)** by
Arian van Putten, Apache-2.0 licensed. The `mkSkill` / `mkCommand` /
`mkAgent` / `mkPlugin` / `mkClaude` library functions, the home-manager
module structure, and the overall "manage Claude Code declaratively
with Nix" approach all originated there. Thanks Arian.

## Statusline references

The `claude-statusline` package in this repository was informed by research
on three existing open-source Claude Code statusline implementations. None
of their source code is vendored here, but specific design and behavior
ideas are borrowed; each project is MIT-licensed and the maintainers are
credited below with thanks.

## Reference projects

- **[jarrodwatts/claude-hud](https://github.com/jarrodwatts/claude-hud)** —
  MIT. Inspired the expanded multi-line layout with on-demand tool / agent /
  todo activity rows, the OSC 8 hyperlink approach for the git branch and
  project path, and the configurable element-order model.

- **[vincent-k2026/codachi](https://github.com/vincent-k2026/codachi)** —
  MIT. Inspired the rate-window pace-delta indicator (`⇡` over-consuming /
  `⇣` headroom against the elapsed fraction of the window) and the
  PostToolExecution event-driven reactivity pattern.

- **[sirmalloc/ccstatusline](https://github.com/sirmalloc/ccstatusline)** —
  MIT. Inspired the two-level git cache (TTL + `.git/HEAD/index` mtime),
  the `--no-optional-locks` and `--porcelain=v2` invocations, streaming
  JSONL dedup by `(id, request_id)`, the cursor-position progress bar
  treatment, the compaction counter (>10% context-drop detection), the
  subagent-aware token-speed walk, the global minimalist toggle concept,
  and the `[1m]` model-id context-size fallback.

## License

`claude-nix` itself is MIT-licensed (see `LICENSE`). Nothing here is a
derivative work of the projects above in the legal sense, but they
deserve credit and thanks for prior art.
