{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.programs.claude-nix;
  inherit (lib)
    mkOption
    mkEnableOption
    mkIf
    types
    ;

  claudeLib = import ../lib {
    pkgs = pkgs.extend (final: prev: { claude-code = cfg.package; });
  };

  claudeBase = claudeLib.mkClaude {
    plugins = cfg.plugins;
    inherit (cfg) appendSystemPrompt;
  };

  # mkClaude already bakes in plugin-dir and append-system-prompt-file
  # flags. Only re-wrap when --verbose is requested.
  wrappedClaude =
    if cfg.verbose then
      pkgs.writeShellScriptBin "claude" ''exec ${claudeBase}/bin/claude --verbose "$@"''
    else
      claudeBase;

  statusLine = cfg.statusLine;

  statusLineConfigJSON = builtins.toJSON {
    padding = statusLine.padding;
    refreshInterval = statusLine.refreshInterval;
    activityRows = statusLine.activityRows;
    hideWhenIdle = statusLine.hideWhenIdle;
    widgets = {
      row1 = statusLine.widgets.row1;
      row2 = statusLine.widgets.row2;
      hide = statusLine.widgets.hide;
    };
    gitCacheTtlSeconds = statusLine.gitCacheTtlSeconds;
    transcriptWindowSeconds = statusLine.transcriptWindowSeconds;
    barWidth = statusLine.barWidth;
    sevenDayThreshold = statusLine.sevenDayThreshold;
    tokenFormat = statusLine.tokenFormat;
  };

  statusLineSettings = lib.optionalAttrs statusLine.enable {
    statusLine = {
      type = "command";
      command = "${statusLine.package}/bin/claude-statusline";
      padding = statusLine.padding;
      refreshInterval = statusLine.refreshInterval;
    };
  };

  # Render the standard Claude config dir (settings.json, CLAUDE.md,
  # statusline-config.json) as a single derivation. Same merge logic the
  # downstream consumers (claude-container's image build, etc.) need, so
  # we centralize it in claudeLib.
  claudeConfig = claudeLib.mkClaudeConfig {
    inherit (cfg) defaultSettings settings globalClaudeMd extraPermissions;
    inherit statusLineSettings;
    statusLineConfigJSON = if cfg.statusLine.enable then statusLineConfigJSON else null;
  };

  # For each extra account (e.g. "work"), build a wrapper binary
  # (e.g. "claude-work") that sets CLAUDE_CONFIG_DIR (e.g. ~/.claude-work)
  # before exec'ing the wrapped claude binary. Plugins are baked into the
  # binary via --plugin-dir so they apply regardless of CLAUDE_CONFIG_DIR.
  accountDir = account: ".claude-${account}";
  accountBin = account: "claude-${account}";

  mkAccountWrapper =
    account:
    pkgs.writeShellScriptBin (accountBin account) ''
      mkdir -p "$HOME/${accountDir account}"
      export CLAUDE_CONFIG_DIR="$HOME/${accountDir account}"
      exec ${wrappedClaude}/bin/claude "$@"
    '';

  accountWrappers = map mkAccountWrapper cfg.extraAccounts;
in
{
  options.programs.claude-nix = {
    enable = mkEnableOption "Claude Code managed declaratively by claude-nix";

    package = mkOption {
      type = types.package;
      default = pkgs.claude-code;
      defaultText = lib.literalExpression "pkgs.claude-code";
      description = "The Claude Code package to install.";
    };

    plugins = mkOption {
      type = types.listOf types.package;
      default = [ ];
      description = ''
        List of plugin derivations produced by `claude-nix.lib.mkPlugin`.
        Each plugin is passed via `--plugin-dir` to the Claude CLI.
      '';
    };

    settings = mkOption {
      type = types.attrs;
      default = { };
      description = ''
        User overrides merged on top of `defaultSettings` via
        `lib.recursiveUpdate`. The result is written to
        `~/.claude/settings.json`.
      '';
    };

    globalClaudeMd = mkOption {
      type = types.lines;
      default = "";
      example = ''
        # Operating context

        Prefer terse, direct prose. Use TDD for bugfixes. Default to
        small focused commits.
      '';
      description = ''
        Markdown content written to `~/.claude/CLAUDE.md` (and to each
        per-account variant from `extraAccounts`). Claude Code auto-loads
        this on every session as user-level context.

        Uses `types.lines`, so multiple modules can contribute and they
        get newline-concatenated. Empty (default) means no file is
        written.
      '';
    };

    appendSystemPrompt = mkOption {
      type = types.lines;
      default = "";
      example = "You are operating inside a sandboxed Docker container.";
      description = ''
        Text materialized into a Nix-store file and passed to the
        `claude` wrapper as `--append-system-prompt-file <path>` on
        every invocation. Appended to Claude Code's default system
        prompt rather than replacing it. Multi-line / special-char
        content is safe because the value reaches claude as a file, not
        a shell arg.

        Empty (default) means the flag is not added. Uses `types.lines`
        so multiple modules can contribute.
      '';
    };

    defaultSettings = mkOption {
      type = types.attrs;
      default = {
        attribution = {
          commit = "";
          pr = "";
        };
        cleanupPeriodDays = 14;
        feedbackSurveyRate = 0;
        env = {
          DISABLE_AUTOUPDATER = "1";
          CLAUDE_CODE_DISABLE_AUTO_MEMORY = "1";
          CLAUDE_CODE_DISABLE_FEEDBACK_SURVEY = "1";
          CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC = "1";
        };
        permissions = {
          allow = [
            "Bash(find:*)"
            "Bash(grep:*)"
            "Bash(ls:*)"
            "Bash(git show:*)"
            "Bash(git rev-parse:*)"
            "Bash(mkdir:*)"
            "Bash(python -m py_compile:*)"
            "Bash(black:*)"
            "Bash(isort:*)"
            # rtk-wrapped variants: the rtk PreToolUse hook rewrites bare
            # commands to `rtk <cmd>` for token savings. Mirror every Bash
            # allow above so the rewritten form is also pre-approved.
            "Bash(rtk find:*)"
            "Bash(rtk grep:*)"
            "Bash(rtk ls:*)"
            "Bash(rtk git show:*)"
            "Bash(rtk git rev-parse:*)"
            "Bash(rtk mkdir:*)"
            "Bash(rtk python -m py_compile:*)"
            "Bash(rtk black:*)"
            "Bash(rtk isort:*)"
            # NixOS absolute-path variants: sandboxed claude resolves bare
            # commands to /run/current-system/sw/bin/<cmd>. Mirror every
            # bare and rtk-wrapped allow above so the resolved path is
            # also pre-approved.
            "Bash(/run/current-system/sw/bin/find:*)"
            "Bash(/run/current-system/sw/bin/grep:*)"
            "Bash(/run/current-system/sw/bin/ls:*)"
            "Bash(/run/current-system/sw/bin/git show:*)"
            "Bash(/run/current-system/sw/bin/git rev-parse:*)"
            "Bash(/run/current-system/sw/bin/mkdir:*)"
            "Bash(/run/current-system/sw/bin/python -m py_compile:*)"
            "Bash(/run/current-system/sw/bin/black:*)"
            "Bash(/run/current-system/sw/bin/isort:*)"
            "Bash(/run/current-system/sw/bin/rtk find:*)"
            "Bash(/run/current-system/sw/bin/rtk grep:*)"
            "Bash(/run/current-system/sw/bin/rtk ls:*)"
            "Bash(/run/current-system/sw/bin/rtk git show:*)"
            "Bash(/run/current-system/sw/bin/rtk git rev-parse:*)"
            "Bash(/run/current-system/sw/bin/rtk mkdir:*)"
            "Bash(/run/current-system/sw/bin/rtk python -m py_compile:*)"
            "Bash(/run/current-system/sw/bin/rtk black:*)"
            "Bash(/run/current-system/sw/bin/rtk isort:*)"
          ];
          deny = [
            "Read(./.env)"
            "Read(./.env.*)"
          ];
          defaultMode = "acceptEdits";
        };
        alwaysThinkingEnabled = true;
        showTurnDuration = true;
        spinnerTipsEnabled = false;
        enabledPlugins = { };
      };
      description = ''
        Sensible default settings for Claude Code. User `settings` are
        merged on top of these via `lib.recursiveUpdate`.
      '';
    };

    extraPermissions = mkOption {
      description = ''
        Permission rules appended to `defaultSettings.permissions`. Use this
        for additive rules so they concatenate with the defaults; use
        `settings.permissions` only when you want to fully replace a list.

        Lists merge via the standard NixOS `listOf` semantics, so multiple
        modules can each contribute (and use `lib.mkBefore` / `lib.mkAfter`
        to order their entries).
      '';
      default = { };
      type = types.submodule {
        options = {
          allow = mkOption {
            type = types.listOf types.str;
            default = [ ];
            description = "Extra `permissions.allow` rules.";
          };
          ask = mkOption {
            type = types.listOf types.str;
            default = [ ];
            description = "Extra `permissions.ask` rules.";
          };
          deny = mkOption {
            type = types.listOf types.str;
            default = [ ];
            description = "Extra `permissions.deny` rules.";
          };
        };
      };
    };

    extraPackages = mkOption {
      type = types.listOf types.package;
      default = [ ];
      description = "Extra packages to install alongside Claude Code.";
    };

    extraAccounts = mkOption {
      type = types.listOf types.str;
      default = [ ];
      description = ''
        Names of parallel Claude accounts to install. Each entry creates:

        - `~/.claude-<name>/settings.json` (copy of the merged settings)
        - A `claude-<name>` command in `$PATH` that runs claude with
          `CLAUDE_CONFIG_DIR=~/.claude-<name>`.

        For example, `[ "work" ]` creates `~/.claude-work/settings.json`
        and a `claude-work` command. Plugins are baked into every wrapper.
      '';
    };

    verbose = mkOption {
      type = types.bool;
      default = false;
      description = ''
        Whether to pass --verbose to the Claude CLI. Off by default —
        passing --verbose adds a token counter to the bottom-right of
        the TUI, which duplicates what the custom claude-statusline
        already shows. Set this to true (or `settings.viewMode = "verbose"`)
        if you also want the in-conversation verbose tool input/output
        detail.
      '';
    };

    statusLine = mkOption {
      description = "Custom claude-statusline integration.";
      default = { };
      type = types.submodule {
        options = {
          enable = mkEnableOption "the custom claude-statusline binary";

          package = mkOption {
            type = types.package;
            default = pkgs.callPackage ../packages/claude-statusline { };
            defaultText = lib.literalExpression "claude-nix.packages.<system>.claude-statusline";
            description = "The claude-statusline binary to install.";
          };

          padding = mkOption {
            type = types.int;
            default = 0;
            description = "Horizontal padding cells, passed to Claude Code's statusLine.padding.";
          };

          refreshInterval = mkOption {
            type = types.int;
            default = 1;
            description = ''
              Seconds between forced re-renders, in addition to Claude
              Code's event-driven updates (0 = event-driven only). Defaults
              to 1 so time-based segments — session clock, burn-rate ETA,
              rate-limit reset countdowns, agent elapsed — tick live. The
              binary caches the expensive work (git porcelain via TTL, the
              parsed transcript via mtime), so a 1s cadence is cheap.
            '';
          };

          activityRows = mkOption {
            type = types.ints.between 0 4;
            default = 4;
            description = ''
              Maximum number of activity rows to render. The activity stack
              (in order) is: running tools, recent-tool counts, agents, todos.
              Each row hides when empty.
            '';
          };

          hideWhenIdle = mkOption {
            type = types.bool;
            default = true;
            description = "Hide activity rows entirely when there is no recent activity.";
          };

          widgets = mkOption {
            description = "Ordered widget lists per row plus a universal hide list.";
            default = { };
            type = types.submodule {
              options = {
                row1 = mkOption {
                  type = types.listOf types.str;
                  default = [
                    "model"
                    "cwd"
                    "git"
                    "duration"
                    "usage5h"
                    "usage7d"
                  ];
                  description = ''
                    Top row — identity, session clock & account usage. The
                    model widget appends the current effort inline (e.g.
                    "Opus 4.7 xhigh"); duration sits right after git.
                  '';
                };
                row2 = mkOption {
                  type = types.listOf types.str;
                  default = [
                    "context"
                    "tokens"
                    "burnRate"
                    "voice"
                    "compaction"
                    "pr"
                    "cost"
                  ];
                  description = "Bottom row — this conversation's state.";
                };
                hide = mkOption {
                  type = types.listOf types.str;
                  default = [ ];
                  description = "Widgets to suppress everywhere.";
                };
              };
            };
          };

          gitCacheTtlSeconds = mkOption {
            type = types.int;
            default = 5;
            description = "Git porcelain cache TTL in seconds.";
          };

          transcriptWindowSeconds = mkOption {
            type = types.int;
            default = 300;
            description = ''
              Time constant (τ) for the burn-rate EMA, in seconds. Larger
              values produce a more stable display that's less reactive to
              individual file-read spikes, at the cost of taking longer
              (~3τ) to converge on a sustained rate change. Default 300s
              (5 min) is a smooth-but-still-responsive middle ground.
            '';
          };

          barWidth = mkOption {
            type = types.int;
            default = 8;
            description = "Width in cells of progress bars.";
          };

          sevenDayThreshold = mkOption {
            type = types.int;
            default = 50;
            description = "Only render usage7d once usage crosses this percent.";
          };

          tokenFormat = mkOption {
            type = types.enum [
              "compact"
              "raw"
            ];
            default = "compact";
            description = "Token count format: compact (516.9k / 1.2M tokens) or raw (516987 tokens).";
          };
        };
      };
    };
  };

  config = mkIf cfg.enable {
    home.packages =
      [ wrappedClaude ]
      ++ accountWrappers
      ++ cfg.extraPackages
      ++ lib.optional cfg.statusLine.enable cfg.statusLine.package;

    home.file = lib.mkMerge (
      [
        { ".claude/settings.json".source = "${claudeConfig}/settings.json"; }
      ]
      ++ lib.optional cfg.statusLine.enable {
        ".claude/statusline-config.json".source = "${claudeConfig}/statusline-config.json";
      }
      ++ lib.optional (cfg.globalClaudeMd != "") {
        ".claude/CLAUDE.md".source = "${claudeConfig}/CLAUDE.md";
      }
      ++ map (account: {
        "${accountDir account}/settings.json".source = "${claudeConfig}/settings.json";
      }) cfg.extraAccounts
      ++ lib.optionals (cfg.globalClaudeMd != "") (
        map (account: {
          "${accountDir account}/CLAUDE.md".source = "${claudeConfig}/CLAUDE.md";
        }) cfg.extraAccounts
      )
    );
  };
}
