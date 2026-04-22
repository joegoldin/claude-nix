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

  mergedSettings = lib.recursiveUpdate cfg.defaultSettings cfg.settings;
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

    extraSettingsDirs = mkOption {
      type = types.listOf types.str;
      default = [ ];
      description = ''
        Additional directories (relative to home) that should receive a
        copy of the merged settings.json. For example, `[".claude-work"]`
        creates `~/.claude-work/settings.json`.
      '';
    };

    verbose = mkOption {
      type = types.bool;
      default = true;
      description = "Whether to pass --verbose to the Claude CLI.";
    };
  };

  config = mkIf cfg.enable {
    home.packages = [ wrappedClaude ] ++ cfg.extraPackages;

    home.file = lib.mkMerge (
      [
        { ".claude/settings.json".text = builtins.toJSON mergedSettings; }
      ]
      ++ map (dir: {
        "${dir}/settings.json".text = builtins.toJSON mergedSettings;
      }) cfg.extraSettingsDirs
    );
  };
}
