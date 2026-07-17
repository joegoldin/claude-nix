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
  # Plugins can now ship hook scripts alongside the plugin metadata.
  # hooks    : serialized to <plugin>/hooks/hooks.json
  # hooksDir : derivation whose contents are copied to <plugin>/hooks/
  # Use ${CLAUDE_PLUGIN_ROOT} inside command strings so paths resolve at runtime.
  hooks = {
    PreToolUse = [{
      matcher = "Bash";
      hooks = [{ type = "command"; command = "\${CLAUDE_PLUGIN_ROOT}/hooks/my-guard.sh"; }];
    }];
  };
  hooksDir = pkgs.runCommand "example-hooks" {} ''
    mkdir -p $out
    cp ${./my-guard.sh} $out/my-guard.sh
    chmod +x $out/my-guard.sh
  '';
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
  completed-tool counts (session totals), subagents, current todo. A tool
  that's been emitted but is still queued shows an hourglass with a wait timer;
  once it actually starts it switches to the spinner with a fresh timer
  measured from its **real** execution start — see *Tool timing* below.

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

### Tool timing

The JSONL transcript records only when a tool is *emitted* and when its result
*lands* — never when it actually starts running, and the "Waiting…" state
(queued behind another tool, or sitting on a permission prompt) is never
written to disk. So from the transcript alone a queued tool and a tool running
in parallel are indistinguishable, and elapsed time can only be measured from
emission, which over-counts queue + permission wait.

When the statusline is enabled, claude-nix therefore also registers
`PermissionRequest` / `PostToolUse` / `PostToolUseFailure` hooks pointing at the
same binary (`claude-statusline hook`). They record each tool's real start
(`PermissionRequest`, which fires right before the tool executes — so a
still-queued tool, which hasn't reached it, isn't counted as started) and end to
a per-session sidecar under `~/.cache/claude-statusline/tool-timing/`, keyed by
`tool_use_id`. The running-tools row joins that to the transcript to show:

- an **hourglass + wait timer** (counting from emission) for a tool that's
  emitted but still queued;
- the **spinner + a fresh run timer from the real start** once it runs (queue
  and permission wait excluded);
- the **true run length** when it finishes.

The hooks are additive — they concatenate with any `extraHooks` /
`settings.hooks` entries for those events — and degrade gracefully: with the
hooks absent the row falls back to emission-relative elapsed, and a tool whose
hook is ever missed can never get stranded as a permanent hourglass (with no
running tool to be queued behind, it reverts to the spinner). Set
`statusLine.toolTiming = false` to opt out.

All defaults are overridable under `programs.claude-nix.statusLine`
(`widgets.row1` / `widgets.row2` / `widgets.hide`, `activityRows`,
`refreshInterval`, `barWidth`, `transcriptWindowSeconds`, `sevenDayThreshold`,
`gitCacheTtlSeconds`, `tokenFormat`, `toolTiming`).

See `docs/plans/2026-05-26-claude-statusline-design.md` for the full spec and
`packages/claude-statusline/` for the source.


## Home-manager module options

The `programs.claude-nix` home-manager module grew several new options this cycle. All additive options concatenate with the defaults so multiple modules can contribute without clobbering each other.

### `globalClaudeMd :: lines`

Markdown written to `~/.claude/CLAUDE.md` (and to each per-account variant created by `extraAccounts`). Claude Code auto-loads it on every session as user-level context. Uses `types.lines` so multiple modules can append.

```nix
programs.claude-nix.globalClaudeMd = ''
  # Operating context
  Prefer terse prose. Use TDD for bugfixes.
'';
```

### `appendSystemPrompt :: lines`

Text materialized into a Nix-store file and passed as `--append-system-prompt-file <path>` on every invocation. Appended to Claude Code's default system prompt rather than replacing it. Multi-line content is safe because the value reaches claude as a file. Empty by default; uses `types.lines`.

```nix
programs.claude-nix.appendSystemPrompt = "You are operating inside a sandboxed Docker container.";
```

### `projectSettings :: attrsOf attrs`

Per-project settings overrides applied at session start via `claude --settings <file>`. The wrapper detects the active project from the git origin URL basename (stable across worktrees), falling back to git toplevel basename, then cwd basename.

```nix
programs.claude-nix.projectSettings = {
  my-project = {
    hooks.PreToolUse = [{
      matcher = "Bash";
      hooks = [{ type = "command"; command = "${plugin}/hooks/script.sh"; }];
    }];
  };
};
```

### `extraSandbox`

Additive sandbox rules that concatenate with `defaultSettings.sandbox`. Mirrors the `extraPermissions` pattern.

```nix
programs.claude-nix.extraSandbox = {
  filesystem.read.allowWithinDeny = [ "/run/user/1000/gnupg" ];
  filesystem.write.denyWithinAllow = [ "/home/me/.ssh" ];
  network.allowedHosts = [ "internal.corp" ];
};
```

Sub-options: `filesystem.read.{allowWithinDeny,denyOnly}`, `filesystem.write.{allowOnly,denyWithinAllow}`, `network.allowedHosts`.

### `extraHooks :: attrsOf (listOf attrs)`

Additive hook entries per event. Each module's contributions for a given event are concatenated rather than replaced. Use `settings.hooks` instead only when you need to fully replace an event's list.

```nix
programs.claude-nix.extraHooks = {
  PreToolUse = [{
    matcher = "Bash";
    hooks = [{ type = "command"; command = "${myPlugin}/hooks/guard.sh"; }];
  }];
};
```

### `mcpServers :: attrsOf attrs`

User-scope MCP servers, written to the top-level `mcpServers` key of
`~/.claude.json` (and each `~/.claude-<account>/.claude.json` created by
`extraAccounts`). settings.json is not used for MCP — Claude Code only reads
server definitions from `~/.claude.json` at user scope.

```nix
programs.claude-nix.mcpServers = {
  context7 = { command = "npx"; args = [ "-y" "@upstash/context7-mcp" ]; };
  stripe = { type = "http"; url = "https://mcp.stripe.com"; };
};
```

Merged on activation via `jq -s '.[0] * .[1]'`, so runtime-added servers and
other keys survive; declared servers win on a name conflict.

### First-class `settings.json` options

Thin, validated wrappers over individual `settings.json` keys. Each folds into
the rendered `settings.json` **only when set** — a null scalar or empty list
drops the key entirely, so an unset option leaves Claude Code's own default in
place. In precedence they sit **above `defaultSettings` but below `settings`**,
so the dedicated option overrides the module default while a raw
`settings.<key>` remains the escape hatch.

| Option | Type | `settings.json` key |
|---|---|---|
| `model` | `nullOr str` | `model` |
| `effortLevel` | `nullOr (enum [low medium high xhigh])` | `effortLevel` |
| `fallbackModel` | `listOf str` (additive) | `fallbackModel` |
| `outputStyle` | `nullOr str` | `outputStyle` |
| `editorMode` | `nullOr (enum [normal vim])` | `editorMode` |
| `askUserQuestionTimeout` | `nullOr str` (`"60s"`/`"5m"`/`"never"`) | `askUserQuestionTimeout` |
| `mcpControl.enableAllProjectMcpServers` | `nullOr bool` | `enableAllProjectMcpServers` |
| `mcpControl.enabledMcpjsonServers` | `listOf str` (additive) | `enabledMcpjsonServers` |
| `mcpControl.disabledMcpjsonServers` | `listOf str` (additive) | `disabledMcpjsonServers` |
| `mcpControl.disableClaudeAiConnectors` | `nullOr bool` | `disableClaudeAiConnectors` |
| `hardening.{disableAllHooks,disableSkillShellExecution,disableWorkflows,disableRemoteControl,disableArtifact,disableBundledSkills,disableAgentView}` | `nullOr bool` each | same-named key |

```nix
programs.claude-nix = {
  fallbackModel = [ "claude-sonnet-5" ];          # additive
  effortLevel = "high";                            # enum-validated
  mcpControl.enabledMcpjsonServers = [ "nixos" ];  # allowlist a project MCP
};
```

**Rebuild-clobber caveat.** `settings.json` is deep-merged on rebuild with the
generated config winning (`jq -s '.[0] * .[1]'`), so any key you *declare* is
re-asserted every rebuild — clobbering an in-session `/model`, `/effort`, or
theme change. Conversely, a key you *don't* declare lets the runtime value
persist. Leave `model` / `effortLevel` unset if you switch them per session;
set them only for a hard default.

**`enableAllProjectMcpServers` is off for a reason.** Blanket-approving every
server in a repo's `.mcp.json` runs unpinned code from whatever project you
open — at odds with this repo's whole thesis. Prefer
`mcpControl.enabledMcpjsonServers` as a trusted allowlist.

**Managed-only keys are not exposed.** `allowManagedHooksOnly`,
`disableSideloadFlags`, `allowAllClaudeAiMcps`, `allowManagedMcpServersOnly`,
and friends are ignored by Claude Code outside a system `managed-settings.json`,
which this module does not write. A dedicated managed-settings target is future
work; setting them via `settings` here would be a silent no-op.

#### Passthrough for the long tail

Every other `settings.json` key is settable today via the `settings` attrset
(deep-merged over everything above) — no dedicated option needed. Keys left to
passthrough on purpose (no validation or default to add): `fastMode`,
`fastModePerSessionOptIn`, `advisorModel`, `fileCheckpointingEnabled` (already
`true` upstream), `autoCompactEnabled` (already `true`), `autoMemoryEnabled`,
`autoMemoryDirectory`, `autoUpdatesChannel`, `showClearContextOnPlanAccept`.

```nix
programs.claude-nix.settings.fileCheckpointingEnabled = false;  # e.g. disable /rewind
```

### `lib.mkClaudeConfig`

New derivation builder (also available in `claudeLib`) that renders a complete Claude config directory — `settings.json`, `CLAUDE.md`, and `statusline-config.json` — as a single Nix derivation. The home-manager module uses it internally; pass it to `claude-container`'s `mkClaudeContainer` or any other consumer that needs the same config layout.

```nix
claudeConfig = claudeLib.mkClaudeConfig {
  inherit settings globalClaudeMd;
  extraPermissions = { allow = [ "Bash(ripgrep:*)" ]; };
};
# claudeConfig/settings.json, claudeConfig/CLAUDE.md, etc.
```

### NixOS path pre-approval in `defaultSettings`

`defaultSettings.permissions.allow` now contains three variants for every entry: the bare command name (e.g. `Bash(find:*)`), the `rtk`-wrapped form (`Bash(rtk find:*)`), and the `/run/current-system/sw/bin/` absolute-path form (`Bash(/run/current-system/sw/bin/find:*)`). This ensures NixOS-resolved paths are pre-approved without extra user configuration.

## Future-work: Just manage `.claude` directory directly

Ideally I want to avoid the entire plugin ecosystem of Claude as it's generally
terrible. However, Claude Code started gate-keeping new features behind plugins.
For example, you can **only** configure LSPs in plugins. You can't just drop
an LSP config in a `.lsp.json`

Hence, we manage Claude Config with plugins for now.  But the experience is
much nicer without plugins. As e.g. `skills` hot-reload in modern Claude





