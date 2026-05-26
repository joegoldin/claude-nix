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

  claudeBase = claudeLib.mkClaude { plugins = cfg.plugins; };

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

  mergedSettings = lib.recursiveUpdate
    (lib.recursiveUpdate cfg.defaultSettings cfg.settings)
    statusLineSettings;

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
        # Enables verbose in-conversation output without adding the duplicate
        # bottom-right token counter that the `--verbose` CLI flag injects.
        # The custom claude-statusline already surfaces token count.
        viewMode = "verbose";
        enabledPlugins = { };
      };
      description = ''
        Sensible default settings for Claude Code. User `settings` are
        merged on top of these via `lib.recursiveUpdate`.
      '';
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
        passing --verbose adds a token counter to the bottom-right of the
        TUI, which duplicates what the custom claude-statusline shows. The
        in-conversation verbose output (tool input/output detail) is
        controlled by `settings.viewMode = "verbose"` instead.
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
            default = 0;
            description = "Seconds between forced re-renders (0 = event-driven only).";
          };

          activityRows = mkOption {
            type = types.ints.between 0 3;
            default = 3;
            description = "Maximum number of activity rows (tools/agents/todos) to render.";
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
                    "usage5h"
                    "usage7d"
                  ];
                  description = ''
                    Top row — identity & account usage. The model widget
                    appends the current effort inline (e.g. "Opus 4.7 xhigh").
                  '';
                };
                row2 = mkOption {
                  type = types.listOf types.str;
                  default = [
                    "context"
                    "duration"
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
            default = 60;
            description = "Rolling window for burn-rate tok/s, in seconds.";
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
            default = "raw";
            description = "Token count format: raw (516987 tokens) or compact (1.2M / 456k).";
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
        { ".claude/settings.json".text = builtins.toJSON mergedSettings; }
      ]
      ++ lib.optional cfg.statusLine.enable {
        ".claude/statusline-config.json".text = statusLineConfigJSON;
      }
      ++ map (account: {
        "${accountDir account}/settings.json".text = builtins.toJSON mergedSettings;
      }) cfg.extraAccounts
    );
  };
}
