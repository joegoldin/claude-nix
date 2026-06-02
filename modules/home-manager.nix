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
    inherit (cfg) appendSystemPrompt projectSettings;
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

  # Tool-timing hooks: when the statusline is enabled (and toolTiming isn't
  # turned off), point PermissionRequest / PostToolUse / PostToolUseFailure at
  # the same statusline binary (`claude-statusline hook`) so it records real
  # tool start/end times to a per-session sidecar the running-tools row reads.
  # Contributed additively, exactly like extraHooks, so it concatenates with
  # (never clobbers) any user/extraHooks entries for those events.
  toolTimingHooks =
    if statusLine.enable && statusLine.toolTiming then
      let
        entry = {
          matcher = "*";
          hooks = [
            {
              type = "command";
              command = "${statusLine.package}/bin/claude-statusline hook";
            }
          ];
        };
      in
      {
        PermissionRequest = [ entry ];
        PostToolUse = [ entry ];
        PostToolUseFailure = [ entry ];
      }
    else
      { };

  # Per-event union of extraHooks and the tool-timing hooks, so both sets of
  # additive contributions survive into the hooks merge below.
  hookContributions = lib.genAttrs (lib.unique (
    lib.attrNames cfg.extraHooks ++ lib.attrNames toolTimingHooks
  )) (event: (cfg.extraHooks.${event} or [ ]) ++ (toolTimingHooks.${event} or [ ]));

  # Fold extra* list contributions into defaultSettings.* so
  # `programs.claude-nix.settings` still wins via recursiveUpdate but
  # additive callers don't have to worry about list-replacement semantics.
  defaultSettingsWithSandbox = lib.recursiveUpdate cfg.defaultSettings {
    sandbox = {
      filesystem = {
        read = {
          allowWithinDeny =
            (cfg.defaultSettings.sandbox.filesystem.read.allowWithinDeny or [ ])
            ++ cfg.extraSandbox.filesystem.read.allowWithinDeny;
          denyOnly =
            (cfg.defaultSettings.sandbox.filesystem.read.denyOnly or [ ])
            ++ cfg.extraSandbox.filesystem.read.denyOnly;
        };
        write = {
          allowOnly =
            (cfg.defaultSettings.sandbox.filesystem.write.allowOnly or [ ])
            ++ cfg.extraSandbox.filesystem.write.allowOnly;
          denyWithinAllow =
            (cfg.defaultSettings.sandbox.filesystem.write.denyWithinAllow or [ ])
            ++ cfg.extraSandbox.filesystem.write.denyWithinAllow;
        };
      };
      network.allowedHosts =
        (cfg.defaultSettings.sandbox.network.allowedHosts or [ ]) ++ cfg.extraSandbox.network.allowedHosts;
    };
    # Per-event hook lists concatenate: defaults < extraHooks + tool-timing
    # contributions < cfg.settings.hooks. Contributions are event-scoped so
    # each module's entries for a given event accumulate; settings still wins
    # outright for an event if explicitly set there.
    hooks = lib.mapAttrs (
      event: extraEntries: (cfg.defaultSettings.hooks.${event} or [ ]) ++ extraEntries
    ) hookContributions;
  };

  # Render the standard Claude config dir (settings.json, CLAUDE.md,
  # statusline-config.json) as a single derivation. Same merge logic the
  # downstream consumers (claude-container's image build, etc.) need, so
  # we centralize it in claudeLib.
  claudeConfig = claudeLib.mkClaudeConfig {
    defaultSettings = defaultSettingsWithSandbox;
    inherit (cfg) settings globalClaudeMd extraPermissions;
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

    projectSettings = mkOption {
      type = types.attrsOf types.attrs;
      default = { };
      example = lib.literalExpression ''
        {
          claude-container = {
            hooks.PreToolUse = [ {
              matcher = "Bash";
              hooks = [ { type = "command"; command = "''${plugin}/hooks/script.sh"; } ];
            } ];
          };
        }
      '';
      description = ''
        Per-project settings overrides applied at session start via
        `claude --settings <file>`. Keys are project identifiers; the
        wrapper detects the active project from the git origin URL
        basename (stable across worktrees), falling back to the git
        toplevel basename, then the cwd basename.

        Each value is materialized to a Nix-store JSON file. On match,
        Claude Code merges it on top of project, user, and managed
        settings using the standard layering rules — the same as if
        you had a project-level `.claude/settings.json`, but kept out
        of the project tree and reproducible via Nix.
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
        # Auto-mode classifier config. The classifier is a second gate that
        # runs after the permissions system; the rtk PreToolUse hook keeps
        # tripping it because it sees the unfamiliar `rtk <cmd>` wrapper and
        # treats it as untrusted. We only override `allow` (each autoMode
        # section is evaluated independently, so the built-in soft_deny /
        # hard_deny / environment defaults stay intact). The leading
        # "$defaults" keeps the built-in allow rules and inherits new ones as
        # claude-nix tracks Claude Code releases.
        autoMode = {
          allow = [
            "$defaults"
            # rtk is a transparent token-saving CLI proxy installed in this
            # environment. A PreToolUse hook rewrites bare commands into
            # `rtk <command>` form (e.g. `rtk grep ...`, `rtk git show ...`)
            # purely to cut token usage — rtk does NOT change what the command
            # does. Judge `rtk <command>` exactly as you would judge
            # `<command>` run directly: the `rtk` prefix neither adds risk nor
            # grants trust, and it never relaxes a BLOCK rule for the wrapped
            # command. The same applies to the absolute-path form
            # `/run/current-system/sw/bin/rtk <command>`.
            ''rtk is a transparent token-saving CLI proxy installed in this environment. A PreToolUse hook rewrites bare commands into `rtk <command>` form (e.g. `rtk grep ...`, `rtk git show ...`, `rtk ls ...`) purely to reduce token usage — rtk does not change what the command does. Judge `rtk <command>` exactly as you would judge `<command>` run directly: the leading `rtk` (or absolute-path `/run/current-system/sw/bin/rtk`) is a no-op wrapper that neither adds risk nor grants trust, and it never relaxes a BLOCK rule that would apply to the underlying command.''
            # The repo's explicitly-trusted read-only / safe dev commands,
            # mirroring permissions.allow above. Listed so the classifier also
            # clears their rtk-wrapped and absolute-path forms.
            ''The following read-only or otherwise safe development commands are explicitly trusted in this environment, whether run bare, rtk-wrapped, or resolved to an absolute `/run/current-system/sw/bin/` path: find, grep, ls, git show, git rev-parse, mkdir, python -m py_compile, black, isort.''
          ];
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

    extraSandbox = mkOption {
      description = ''
        Sandbox rules appended to `defaultSettings.sandbox`. Use this for
        additive entries so they concatenate with the defaults; use
        `settings.sandbox` only when you want to fully replace a list.

        Mirrors the `extraPermissions` pattern. Common entries:

        - `extraSandbox.filesystem.read.allowWithinDeny` — paths Claude
          may read despite the default deny (e.g. SSH agent socket,
          known_hosts).
        - `extraSandbox.filesystem.write.denyWithinAllow` — paths Claude
          may NOT write despite the default allow.
        - `extraSandbox.network.allowedHosts` — extra hosts Claude may
          reach in addition to the defaults.

        Lists merge via the standard NixOS `listOf` semantics, so
        multiple modules can contribute (use `lib.mkBefore` /
        `lib.mkAfter` to order their entries).
      '';
      default = { };
      type = types.submodule {
        options = {
          filesystem = mkOption {
            default = { };
            type = types.submodule {
              options = {
                read = mkOption {
                  default = { };
                  type = types.submodule {
                    options = {
                      allowWithinDeny = mkOption {
                        type = types.listOf types.str;
                        default = [ ];
                        description = "Extra `sandbox.filesystem.read.allowWithinDeny` paths.";
                      };
                      denyOnly = mkOption {
                        type = types.listOf types.str;
                        default = [ ];
                        description = "Extra `sandbox.filesystem.read.denyOnly` paths.";
                      };
                    };
                  };
                };
                write = mkOption {
                  default = { };
                  type = types.submodule {
                    options = {
                      allowOnly = mkOption {
                        type = types.listOf types.str;
                        default = [ ];
                        description = "Extra `sandbox.filesystem.write.allowOnly` paths.";
                      };
                      denyWithinAllow = mkOption {
                        type = types.listOf types.str;
                        default = [ ];
                        description = "Extra `sandbox.filesystem.write.denyWithinAllow` paths.";
                      };
                    };
                  };
                };
              };
            };
          };
          network = mkOption {
            default = { };
            type = types.submodule {
              options = {
                allowedHosts = mkOption {
                  type = types.listOf types.str;
                  default = [ ];
                  description = "Extra `sandbox.network.allowedHosts` entries.";
                };
              };
            };
          };
        };
      };
    };

    extraHooks = mkOption {
      type = types.attrsOf (types.listOf types.attrs);
      default = { };
      example = lib.literalExpression ''
        {
          PreToolUse = [ {
            matcher = "Bash";
            hooks = [ { type = "command"; command = "''${plugin}/hook.sh"; } ];
          } ];
        }
      '';
      description = ''
        Hook entries appended to `defaultSettings.hooks` per event.
        Multiple modules can contribute; lists per event are
        concatenated rather than replaced.

        For full replacement semantics (e.g. clearing a default event),
        set `settings.hooks` instead — it still wins via recursiveUpdate.
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

          toolTiming = mkOption {
            type = types.bool;
            default = true;
            description = ''
              Register PermissionRequest / PostToolUse / PostToolUseFailure
              hooks (pointing at the same statusline binary, run as
              `claude-statusline hook`) that record each tool's real
              execution start/end to a per-session sidecar.

              The transcript only records when a tool_use is emitted and when
              its result lands — never when it actually starts — and the
              "Waiting…" (queued / awaiting-permission) state is never written
              to disk. With these hooks the running-tools row shows an
              hourglass for a tool that's emitted but not yet started, a
              spinner with elapsed measured from the real start (excluding
              queue + permission wait) once it runs, and a correct final run
              length when it finishes. Without them the row still works,
              falling back to emission-relative elapsed.

              The hooks are additive: they concatenate with any
              `extraHooks` / `settings.hooks` entries for the same events.
            '';
          };
        };
      };
    };
  };

  config = mkIf cfg.enable {
    home.packages = [
      wrappedClaude
    ]
    ++ accountWrappers
    ++ cfg.extraPackages
    ++ lib.optional cfg.statusLine.enable cfg.statusLine.package;

    # NB: settings.json is intentionally NOT placed via home.file. Claude Code
    # rewrites it at runtime (/effort, view mode, theme, etc.), and a home.file
    # symlink points into the read-only Nix store, so those writes fail with
    # EACCES. It is instead copied into place as a writable file by the
    # claudeSettingsCopy activation script below. CLAUDE.md and
    # statusline-config.json are never mutated at runtime, so they stay as
    # immutable store symlinks here.
    home.file = lib.mkMerge (
      lib.optional cfg.statusLine.enable {
        ".claude/statusline-config.json".source = "${claudeConfig}/statusline-config.json";
      }
      ++ lib.optional (cfg.globalClaudeMd != "") {
        ".claude/CLAUDE.md".source = "${claudeConfig}/CLAUDE.md";
      }
      ++ lib.optionals (cfg.globalClaudeMd != "") (
        map (account: {
          "${accountDir account}/CLAUDE.md".source = "${claudeConfig}/CLAUDE.md";
        }) cfg.extraAccounts
      )
    );

    # Place the rendered settings.json as a writable file (not a store symlink)
    # so Claude Code can mutate it at runtime (/effort, view mode, theme, …);
    # a read-only store symlink makes those writes fail with EACCES. On rebuild
    # the Nix-generated settings are deep-merged into the existing file (jq
    # `.[0] * .[1]`, so generated keys win on conflict while runtime-added keys
    # survive). This mirrors how codex-nix / gemini-nix / antigravity-cli-nix
    # reconcile their config files.
    home.activation.claudeSettingsCopy =
      let
        targets = [
          ".claude/settings.json"
        ]
        ++ map (account: "${accountDir account}/settings.json") cfg.extraAccounts;
        mergeOne = rel: ''
          configFile=${config.home.homeDirectory}/${rel}
          generatedConfig=${claudeConfig}/settings.json

          run mkdir -p "$(dirname "$configFile")"

          if [[ -L "$configFile" ]]; then
            run unlink "$configFile"
          fi

          if [[ -f "$configFile" ]]; then
            tmpFile=$(mktemp)
            run ${lib.getExe pkgs.jq} -s '.[0] * .[1]' "$configFile" "$generatedConfig" > "$tmpFile"
            run mv "$tmpFile" "$configFile"
          else
            run cp "$generatedConfig" "$configFile"
          fi

          run chmod 600 "$configFile"
        '';
      in
      lib.hm.dag.entryAfter [ "writeBoundary" ] (lib.concatMapStrings mergeOne targets);
  };
}
