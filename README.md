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
`~/.claude/statusline-config.json` from your Nix-typed options, and wires
`settings.statusLine.command` to the binary.

Default layout (Nerd Font required):

```
 Opus │  ~/project │  main ↑3 │ ████░░░░ 47%                     5m30s
 5h ███░░░░░ 38% (3h) ⇣2% │  7d █████░░░ 65% (5d) ⇡48% │  high            #42 pending
◐ Edit: home.nix  ·  ✓ Read ×3  ·  ✓ Grep ×2
◐ explore [haiku]: Finding LSP config (2m 15s)
▸ Wire up statusline (2/5)
```

Cost is hidden for Claude Max subscribers inside their plan limits; it only
appears when either rate-limit window has reached 100% (overage territory).

See `docs/plans/2026-05-26-claude-statusline-design.md` for the full spec and
`packages/claude-statusline/` for the source.


## Future-work: Just manage `.claude` directory directly

Ideally I want to avoid the entire plugin ecosystem of Claude as it's generally
terrible. However, Claude Code started gate-keeping new features behind plugins.
For example, you can **only** configure LSPs in plugins. You can't just drop
an LSP config in a `.lsp.json`

Hence, we manage Claude Config with plugins for now.  But the experience is
much nicer without plugins. As e.g. `skills` hot-reload in modern Claude





