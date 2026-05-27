# claude-nix

Manage Claude Code with Nix!

## Usage

This will drop you into a claude code session with
a few plugins managed by this repo loaded.

```
nix run .
```

To create a plugin:

```nix

example-plugin = claudeLib.mkPlugin {
  name = "example";
  description = "an example plugin";
  skills = [
    (claudeLib.mkSkill {
      name = "cowsay";
      description = "When you wanna say something like a cow";
      allowed-tools =  ["Bash(${pkgs.cowsay}/bin/cowsay)"];
    } ''
    You are MOOOOOdy . Use `${pkgs.cowsay}/bin/cowsay MSG` to say things
    like acow would.
    '')
  ];
}
```



```nix
claudeLib.mkClaude {
  plugins = [ plugin1 plugin2 ... ];
}
```

### Examples:

See [./flake.nix](./flake.nix) for full example.

* `plugin-nix` ships `nixd` lsp, and a skill to run `statix` and `nixfmt`. It shows off how to install LSPs using nix, and have skills refer to binaries from nixpkgs.
* `plugin-chromium` ships the Chromium Devtools MCP,, and a skill to do webpage audits. It shows off how to install MCPs using nix.
* `plugin-pandoc` ships a skill to convert to PDF using `pandoc` and `texlive`. IT shows off how to have skills refer to binaries from nixpkgs.

## Why?

Claude-code is amazing, but they somehow invented the worst package manager on
earth, stealing the crown from [Github
Actions](https://nesbitt.io/2025/12/06/github-actions-package-manager.html).

The "Claude Marketplace" has no pinning, no dependency resolution, and worst of
all, a lot of plugins in turn just call out to `npx` or `uvx` to install
unpinned versions of nodejs and python packages or don't help you install the
binaries at all! In the light of the series of Shai Hulud worms pwning several
major companies, running agents with unpinned dependencies from the web is not
only annoying for reproducibility, it *will* get you hacked.

Instead, we use Nix to manage Claude Code. This allows us to pin exactly
which MCPs and LSPs we want to use, but also what binaries our skills,
agents, hooks, and commands have access to!

In the end, claude configs are just files. But we ship some smart helpers to
make generating those files easy.


## Managing external plugins  using Nix:

TODO: Write tutorial


## Statusline

`claude-nix` ships a Go-based custom Claude Code statusline (Nerd Font, ANSI 16
colors, reactive layout). Enable it via the home-manager module:

```nix
{
  programs.claude-nix = {
    enable = true;
    statusLine.enable = true;
  };
}
```

This installs the `claude-statusline` binary, writes
`~/.claude/statusline-config.json` from your Nix-typed options, wires
`settings.statusLine.command` to the binary, and sets `refreshInterval = 1`
so the session clock / burn-rate ETA / reset countdowns tick live.

### Layout

Up to **6 lines**, each hidden when empty (Nerd Font required):

```
 Opus 4.7 1M xhigh │  ~/project │  main* ↑1 │  42m18s        ████▒░░░ 81% │ 810.0k tokens │  0.3%/m ETA 1h7m │  1c
◐ Bash: nix build … · ◐ Edit: home.nix
✓ Bash ×273 · ✓ Edit ×196 · ✓ Write ×92 · ✓ Read ×75
✓ Explore: Audit statusline docs (28s) · ✓ Explore: Investigate ccstatusline (4m0s)
▸ Sync README statusline section (24/27)
```

- **Row 1 — identity & budget**: model (with effort + `1M` for the 1M-context
  variant inline), cwd, git branch + dirty/ahead-behind + worktree, session
  duration, then the 5h / 7d account-usage windows.
- **Row 2 — conversation state**: context-window bar, token count, burn rate
  (% of context per minute, EMA-smoothed) + ETA-to-full, voice mode,
  compaction counter, PR badge, cost.
- **Rows 3–6 — activity** (each appears only when populated): running tools,
  completed-tool counts (session totals), subagents, current todo.

On a **wide** terminal rows 1 and 2 merge onto a single line (row 1 left,
row 2 right). As the terminal narrows the dashboard **wraps** across more
lines rather than truncating; the 6-line budget is given to the dashboard
first, so activity rows are dropped before any dashboard content is lost.

### Bars & colors

- Context bar: shaded blocks `░▒▓█`. 5h usage: dotted braille `⣀…⣿`. 7d
  usage: triangle ticks `▷▶`. All sub-cell granular.
- ANSI 16 (your terminal theme supplies the actual RGB) plus one 256-color
  orange for the token gradient: gray <20% · green <50% · yellow <70% ·
  orange <85% · red ≥85% of the context window.

### Notes

- **Cost** is hidden for Claude Max subscribers inside plan limits — it only
  appears once a rate-limit window hits 100% (overage). Hidden entirely when
  `rate_limits` is absent (e.g. early in a resumed session).
- **Verbose** is off by default (no duplicate bottom-right token counter);
  set `programs.claude-nix.verbose = true` if you want Claude's verbose UI.
- The transcript is parsed **incrementally** and cached under
  `~/.cache/claude-statusline/`, so a 1s refresh never re-parses the whole
  (often multi-MB) transcript; git runs at most once per `gitCacheTtlSeconds`.

All defaults are overridable under `programs.claude-nix.statusLine`
(`widgets.row1` / `widgets.row2` / `widgets.hide`, `activityRows`,
`refreshInterval`, `barWidth`, `transcriptWindowSeconds`, `sevenDayThreshold`,
`gitCacheTtlSeconds`, `tokenFormat`).

See `docs/plans/2026-05-26-claude-statusline-design.md` for the full spec and
`packages/claude-statusline/` for the source.


## Future-work: Just manage `.claude` directory directly

Ideally I want to avoid the entire plugin ecosystem of Claude as it's generally
terrible. However, Claude Code started gate-keeping new features behind plugins.
For example, you can **only** configure LSPs in plugins. You can't just drop
an LSP config in a `.lsp.json`

Hence, we manage Claude Config with plugins for now.  But the experience is
much nicer without plugins. As e.g. `skills` hot-reload in modern Claude





