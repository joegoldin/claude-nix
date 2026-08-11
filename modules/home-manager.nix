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

  # Shared by askUserQuestionTimeout / dialogExpiry — Claude Code validates
  # both against this exact set and silently drops anything else.
  timeoutEnum = types.enum [
    "60s"
    "5m"
    "10m"
    "never"
  ];

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

  statusLineSettings =
    lib.optionalAttrs statusLine.enable {
      statusLine = {
        type = "command";
        command = "${statusLine.package}/bin/claude-statusline";
        padding = statusLine.padding;
        refreshInterval = statusLine.refreshInterval;
      }
      // lib.optionalAttrs (statusLine.hideVimModeIndicator != null) {
        inherit (statusLine) hideVimModeIndicator;
      };
    }
    // lib.optionalAttrs (cfg.subagentStatusLine != null) {
      subagentStatusLine = {
        type = "command";
        command = cfg.subagentStatusLine;
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
  # Additive sandbox lists, in the shape Claude Code's *settings* schema
  # actually accepts (sandbox.filesystem.{allowRead,denyRead,allowWrite,
  # denyWrite} / sandbox.network.{allowedDomains,deniedDomains,...}). Each
  # entry is emitted only when non-empty, so an untouched sandbox block
  # collapses away rather than writing `{ }` placeholders.
  concatSandbox =
    path: extra: (lib.attrByPath path [ ] (cfg.defaultSettings.sandbox or { })) ++ extra;

  sandboxListContributions = {
    filesystem = {
      allowRead = concatSandbox [ "filesystem" "allowRead" ] cfg.extraSandbox.filesystem.allowRead;
      denyRead = concatSandbox [ "filesystem" "denyRead" ] cfg.extraSandbox.filesystem.denyRead;
      allowWrite = concatSandbox [ "filesystem" "allowWrite" ] cfg.extraSandbox.filesystem.allowWrite;
      denyWrite = concatSandbox [ "filesystem" "denyWrite" ] cfg.extraSandbox.filesystem.denyWrite;
    };
    network = {
      allowedDomains = concatSandbox [
        "network"
        "allowedDomains"
      ] cfg.extraSandbox.network.allowedDomains;
      deniedDomains = concatSandbox [ "network" "deniedDomains" ] cfg.extraSandbox.network.deniedDomains;
      allowUnixSockets = concatSandbox [
        "network"
        "allowUnixSockets"
      ] cfg.extraSandbox.network.allowUnixSockets;
      allowMachLookup = concatSandbox [
        "network"
        "allowMachLookup"
      ] cfg.extraSandbox.network.allowMachLookup;
    };
  };

  sandboxLists = lib.filterAttrs (_: v: v != { }) (
    lib.mapAttrs (_: lib.filterAttrs (_: v: v != [ ])) sandboxListContributions
  );

  # Auto-mode classifier rules are concatenated onto the defaults by
  # mergeClaudeSettings (like extraPermissions), not replaced.
  extraAutoMode = {
    inherit (cfg.autoMode)
      allow
      soft_deny
      hard_deny
      environment
      ;
  };

  defaultSettingsWithSandbox = lib.recursiveUpdate cfg.defaultSettings (
    {
      # Per-event hook lists concatenate: defaults < extraHooks + tool-timing
      # contributions < cfg.settings.hooks. Contributions are event-scoped so
      # each module's entries for a given event accumulate; settings still wins
      # outright for an event if explicitly set there.
      hooks = lib.mapAttrs (
        event: extraEntries: (cfg.defaultSettings.hooks.${event} or [ ]) ++ extraEntries
      ) hookContributions;
    }
    // lib.optionalAttrs (sandboxLists != { }) { sandbox = sandboxLists; }
  );

  # First-class settings-shaped options, gathered as a flat attrset in the
  # exact settings.json key shape. mkClaudeConfig drops null scalars and empty
  # lists, so unset options omit their keys. Keys are selected explicitly (not
  # via `// cfg.mcpControl`) so submodule internals like `_module` never leak
  # into settings.json.
  rawOptionSettings = {
    inherit (cfg)
      model
      effortLevel
      fallbackModel
      outputStyle
      editorMode
      askUserQuestionTimeout
      ;
    inherit (cfg.mcpControl)
      enableAllProjectMcpServers
      enabledMcpjsonServers
      disabledMcpjsonServers
      disableClaudeAiConnectors
      ;
    inherit (cfg.hardening)
      disableAllHooks
      disableSkillShellExecution
      disableWorkflows
      disableRemoteControl
      disableArtifact
      disableBundledSkills
      disableAgentView
      ;
    inherit (cfg)
      tui
      viewMode
      theme
      language
      plansDirectory
      agent
      autoUpdatesChannel
      feedbackDrafts
      respectGitignore
      defaultShell
      respondToBashCommands
      includeGitInstructions
      dialogExpiry
      ;

    # ── Grouped submodules → their settings.json keys ──
    # mergeClaudeSettings prunes recursively, so a group with everything
    # unset collapses away instead of writing an empty object.
    inherit (cfg.autoMode)
      skipAutoPermissionPrompt
      useAutoModeDuringPlan
      ;
    # Only the scalar: the four rule lists are concatenated onto the defaults
    # above rather than replacing them, so they never reach this layer.
    autoMode = {
      inherit (cfg.autoMode) classifyAllShell;
    };

    inherit (cfg.workflows) workflowSizeGuideline workflowKeywordTriggerEnabled;
    enableWorkflows = cfg.workflows.enable;
    skipWorkflowUsageWarning = cfg.workflows.skipUsageWarning;

    enableArtifact = cfg.artifacts.enable;

    worktree = {
      inherit (cfg.worktree)
        symlinkDirectories
        sparsePaths
        baseRef
        bgIsolation
        ;
    };

    inherit (cfg.memory)
      autoMemoryEnabled
      autoMemoryDirectory
      autoDreamEnabled
      ;

    inherit (cfg.skills)
      skillListingMaxDescChars
      skillListingBudgetFraction
      skillOverrides
      ;

    inherit (cfg.compaction)
      autoCompactEnabled
      autoCompactWindow
      precomputeCompactionEnabled
      ;

    inherit (cfg.ui)
      fileCheckpointingEnabled
      showThinkingSummaries
      showMessageTimestamps
      terminalProgressBarEnabled
      todoFeatureEnabled
      autoScrollEnabled
      wheelScrollAccelerationEnabled
      prefersReducedMotion
      emojiCompletionEnabled
      promptSuggestionEnabled
      syntaxHighlightingDisabled
      terminalTitleFromRename
      showClearContextOnPlanAccept
      ;

    voice = {
      inherit (cfg.voice) enabled mode autoSubmit;
    };
    # Claude Code carries two keys for the same switch — the flat
    # `voiceEnabled` from its feature-gated block and `voice.enabled` from the
    # settings object — and writes both when you toggle voice in the UI. Mirror
    # that, so a declared value doesn't half-apply.
    voiceEnabled = cfg.voice.enabled;

    inherit (cfg.remoteControl)
      remoteControlAtStartup
      isolatePeerMachines
      crossSessionInbound
      autoUploadSessions
      daemonColdStart
      teammateMode
      inputNeededNotifEnabled
      agentPushNotifEnabled
      ;

    sandbox = {
      inherit (cfg.sandboxControl)
        enabled
        failIfUnavailable
        autoAllowBashIfSandboxed
        allowUnsandboxedCommands
        excludedCommands
        ;
      filesystem.disabled = cfg.sandboxControl.filesystemDisabled;
      network.strictAllowlist = cfg.sandboxControl.strictNetworkAllowlist;
    };
  };

  # Render the standard Claude config dir (settings.json, CLAUDE.md,
  # statusline-config.json) as a single derivation. Same merge logic the
  # downstream consumers (claude-container's image build, etc.) need, so
  # we centralize it in claudeLib.
  claudeConfig = claudeLib.mkClaudeConfig {
    defaultSettings = defaultSettingsWithSandbox;
    inherit (cfg) settings globalClaudeMd extraPermissions;
    inherit extraAutoMode;
    optionSettings = rawOptionSettings;
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

    mcpServers = mkOption {
      type = types.attrsOf types.attrs;
      default = { };
      example = lib.literalExpression ''
        {
          context7 = {
            command = "npx";
            args = [ "-y" "@upstash/context7-mcp" ];
          };
          stripe = {
            type = "http";
            url = "https://mcp.stripe.com";
          };
        }
      '';
      description = ''
        User-scope MCP servers, written to the top-level `mcpServers` key of
        `~/.claude.json` (and each `~/.claude-<account>/.claude.json` for
        `extraAccounts`). Values use Claude Code's native MCP shape: stdio
        servers `{ command; args?; env?; }` or remote servers
        `{ type = "http"; url; headers?; }`.

        Merged into the existing file via a jq deep-merge on activation, so
        runtime/manually-added servers and every other `.claude.json` key are
        preserved; declared servers win on a name conflict. settings.json is
        NOT used for MCP — Claude Code only reads server definitions from
        `~/.claude.json` at user scope.
      '';
    };

    # ---- First-class settings.json options -------------------------------
    # Thin, validated wrappers over individual settings.json keys. Each folds
    # into the rendered settings.json only when set: null scalars and empty
    # lists are dropped, so an unset option omits its key entirely. They sit
    # above `defaultSettings` but below `settings`, so the dedicated option
    # overrides the module default while a raw `settings.<key>` stays the
    # escape hatch. Scalar keys with no validation or default value are left to
    # the `settings` passthrough on purpose (see README) rather than
    # first-classed, to keep the option surface small.

    model = mkOption {
      type = types.nullOr types.str;
      default = null;
      example = "opus";
      description = ''
        Default model written to `settings.model` — a model alias
        (`"opus"`, `"sonnet"`, ...) or a full model ID. Null (default) omits
        the key, leaving Claude Code's own default.

        Caveat: settings.json is deep-merged on rebuild with the generated
        config winning, so a declared `model` is re-asserted on every
        rebuild — clobbering an in-session `/model` switch. Leave this null
        if you change models per session; set it only for a hard default.
      '';
    };

    effortLevel = mkOption {
      type = types.nullOr (
        types.enum [
          "low"
          "medium"
          "high"
          "xhigh"
        ]
      );
      default = null;
      example = "high";
      description = ''
        Default reasoning effort written to `settings.effortLevel`. Same
        rebuild-clobber caveat as `model`: a declared value re-asserts on
        rebuild over an in-session `/effort` change.
      '';
    };

    fallbackModel = mkOption {
      type = types.listOf types.str;
      default = [ ];
      example = [ "claude-sonnet-5" ];
      description = ''
        Fallback model chain written to `settings.fallbackModel`; Claude
        Code falls through the list when the primary model is unavailable.
        Additive — multiple modules' entries concatenate (like
        `extraPermissions`). Empty (default) omits the key. Use
        `settings.fallbackModel` to replace rather than append.
      '';
    };

    outputStyle = mkOption {
      type = types.nullOr types.str;
      default = null;
      example = "default";
      description = ''
        Output style written to `settings.outputStyle`. Null (default) omits
        the key.
      '';
    };

    editorMode = mkOption {
      type = types.nullOr (
        types.enum [
          "normal"
          "vim"
        ]
      );
      default = null;
      example = "vim";
      description = ''
        Editor key-binding mode written to `settings.editorMode`. Null
        (default) omits the key (Claude Code defaults to `"normal"`).
      '';
    };

    askUserQuestionTimeout = mkOption {
      type = types.nullOr timeoutEnum;
      default = null;
      example = "10m";
      description = ''
        Idle time before an unanswered AskUserQuestion auto-continues with
        whatever answers are selected so far, written to
        `settings.askUserQuestionTimeout`. Null (default) omits the key
        (Claude Code defaults to `"never"`). Set a finite value only for
        unattended runs.
      '';
    };

    dialogExpiry = mkOption {
      type = types.nullOr timeoutEnum;
      default = null;
      example = "5m";
      description = ''
        How long a permission/user dialog forwarded to a remote client stays
        parked awaiting an answer — and how long a HELD cross-session message
        awaits approval — before resolving to its safe no-action default
        (`settings.dialogExpiry`). Null (default) omits the key; Claude Code
        then uses `"5m"`. Local-only prompts are unaffected.
      '';
    };

    tui = mkOption {
      type = types.nullOr (
        types.enum [
          "default"
          "fullscreen"
        ]
      );
      default = "fullscreen";
      description = ''
        Terminal UI renderer written to `settings.tui`. `"fullscreen"`
        (the default here) uses the flicker-free alt-screen renderer with
        virtualized scrollback — the same thing `CLAUDE_CODE_NO_FLICKER=1`
        selects — and is what enables `ui.autoScrollEnabled` /
        `ui.wheelScrollAccelerationEnabled`. `"default"` is the classic
        main-screen renderer. Null omits the key, leaving Claude Code's own
        (server-gated) choice.

        Claude Code disables fullscreen regardless on a few terminals it
        knows re-render badly (tmux control mode, Windows-over-SSH ConPTY).
      '';
    };

    viewMode = mkOption {
      type = types.nullOr (
        types.enum [
          "default"
          "verbose"
          "focus"
        ]
      );
      default = null;
      example = "verbose";
      description = ''
        Default transcript view mode on startup (`settings.viewMode`).
        `"verbose"` is the settings-level equivalent of the `verbose` option
        below / the `--verbose` flag. Null (default) omits the key.
      '';
    };

    theme = mkOption {
      type = types.nullOr types.str;
      default = null;
      example = "dark";
      description = ''
        Colour theme written to `settings.theme`. Built-ins: `"auto"`,
        `"dark"`, `"light"`, `"light-daltonized"`, `"dark-daltonized"`,
        `"light-ansi"`, `"dark-ansi"`; a plugin-provided theme uses the
        `"custom:<name>"` form. Left as a free string so plugin themes work.
        Null (default) omits the key.

        Same rebuild-clobber caveat as `model`: a declared value re-asserts
        on every rebuild over an in-session `/theme` switch.
      '';
    };

    language = mkOption {
      type = types.nullOr types.str;
      default = null;
      example = "japanese";
      description = ''
        Preferred language for Claude's responses and voice dictation
        (`settings.language`). Null (default) omits the key (English).
      '';
    };

    plansDirectory = mkOption {
      type = types.nullOr types.str;
      default = null;
      example = "docs/plans";
      description = ''
        Directory for plan files, relative to the project root
        (`settings.plansDirectory`). Null (default) omits the key; Claude
        Code then writes plans to `~/.claude/plans/`.
      '';
    };

    agent = mkOption {
      type = types.nullOr types.str;
      default = null;
      description = ''
        Name of an agent (built-in or plugin-provided) to drive the main
        thread (`settings.agent`), applying that agent's system prompt, tool
        restrictions and model. Null (default) omits the key.
      '';
    };

    autoUpdatesChannel = mkOption {
      type = types.nullOr (
        types.enum [
          "latest"
          "stable"
          "rc"
        ]
      );
      default = null;
      description = ''
        Release channel for Claude Code's own auto-updates
        (`settings.autoUpdatesChannel`). Mostly moot here — the module's
        `defaultSettings` set `DISABLE_AUTOUPDATER=1` because the package
        comes from Nix — so this is only useful if you re-enable updating.
        Null (default) omits the key.
      '';
    };

    feedbackDrafts = mkOption {
      type = types.nullOr (
        types.enum [
          "notify"
          "quiet"
          "off"
        ]
      );
      default = null;
      description = ''
        Model-drafted feedback via the SendFeedback tool
        (`settings.feedbackDrafts`). `"notify"` (Claude Code's default)
        shows a one-line notice when a draft is queued, `"quiet"` shows only
        the footer counter, `"off"` disables the tool. Null omits the key.
      '';
    };

    respectGitignore = mkOption {
      type = types.nullOr types.bool;
      default = null;
      description = ''
        Whether the `@`-mention file picker respects `.gitignore`
        (`settings.respectGitignore`; Claude Code defaults to true).
        `.ignore` files are always respected either way. Null omits the key.
      '';
    };

    defaultShell = mkOption {
      type = types.nullOr (
        types.enum [
          "bash"
          "powershell"
        ]
      );
      default = null;
      description = ''
        Shell used for input-box `!` commands (`settings.defaultShell`).
        Claude Code defaults to bash on every platform. Null omits the key.
      '';
    };

    respondToBashCommands = mkOption {
      type = types.nullOr types.bool;
      default = null;
      description = ''
        Whether Claude responds after an input-box `!` command runs
        (`settings.respondToBashCommands`; Claude Code defaults to true).
        Set false to drop the output into context without a reply. Null
        omits the key.
      '';
    };

    includeGitInstructions = mkOption {
      type = types.nullOr types.bool;
      default = null;
      description = ''
        Include Claude Code's built-in commit / PR workflow instructions in
        the system prompt (`settings.includeGitInstructions`; defaults to
        true). Set false if `globalClaudeMd` or a plugin supplies its own
        git workflow. Null omits the key.
      '';
    };

    autoMode = mkOption {
      description = ''
        Auto permission mode: a model classifier decides each permission
        prompt instead of stopping to ask. `defaultSettings.permissions
        .defaultMode` is `"auto"` out of the box, so these tune a mode that
        is already on.

        The four rule lists are *additive* — they concatenate with whatever
        `defaultSettings.autoMode.<section>` holds, so several modules can
        contribute. Include the literal string `"$defaults"` in a list to
        inherit Claude Code's built-in rules at that position (the shipped
        `allow` list already does).

        Note: Claude Code reads `autoMode` rules only from trusted sources
        (user / policy / flag settings), never from a checked-in project
        settings file.
      '';
      default = { };
      type = types.submodule {
        options = {
          skipAutoPermissionPrompt = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = ''
              Records that the auto-mode opt-in dialog has been accepted
              (`settings.skipAutoPermissionPrompt`). Setting true suppresses
              that first-run dialog in a fresh config dir — useful for
              containers and `extraAccounts`. Null (default) omits the key,
              so the dialog is shown once per config dir as usual.
            '';
          };
          useAutoModeDuringPlan = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = ''
              Whether plan mode uses auto-mode semantics when auto mode is
              available (`settings.useAutoModeDuringPlan`; Claude Code
              defaults to true). Null omits the key.
            '';
          };
          allow = mkOption {
            type = types.listOf types.str;
            default = [ ];
            description = "Extra `autoMode.allow` classifier rules (additive).";
          };
          soft_deny = mkOption {
            type = types.listOf types.str;
            default = [ ];
            description = ''
              Extra `autoMode.soft_deny` rules (additive) — destructive or
              irreversible actions that explicit user intent can clear.
            '';
          };
          hard_deny = mkOption {
            type = types.listOf types.str;
            default = [ ];
            description = ''
              Extra `autoMode.hard_deny` rules (additive) — security
              boundaries that user intent does NOT clear.
            '';
          };
          environment = mkOption {
            type = types.listOf types.str;
            default = [ ];
            description = ''
              Extra `autoMode.environment` entries (additive) — facts about
              this machine the classifier should assume.
            '';
          };
          classifyAllShell = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = ''
              When true, every Bash/PowerShell `permissions.allow` rule is
              suspended while auto mode is active and all shell commands go
              through the classifier (`autoMode.classifyAllShell`): safer,
              but a classifier call per command. Claude Code defaults to
              false. Null omits the key.
            '';
          };
        };
      };
    };

    workflows = mkOption {
      description = "The Workflows (multi-agent orchestration) feature.";
      default = { };
      type = types.submodule {
        options = {
          enable = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = ''
              Enable or disable Workflows for this user
              (`settings.enableWorkflows`). Null (default) omits the key,
              leaving the per-plan default. To hard-disable regardless, use
              `hardening.disableWorkflows`.
            '';
          };
          workflowSizeGuideline = mkOption {
            type = types.nullOr (
              types.enum [
                "unrestricted"
                "small"
                "medium"
                "large"
              ]
            );
            default = null;
            description = ''
              Advisory ceiling on the size of workflows Claude writes
              (`settings.workflowSizeGuideline`): `small` aims for under 5
              agents, `medium` (Claude Code's default) under 15, `large`
              under 50, `unrestricted` sends no guideline. Setting this here
              hides the matching `/config` row. Null omits the key.
            '';
          };
          workflowKeywordTriggerEnabled = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = ''
              Whether the `ultracode` keyword in a prompt opts that turn into
              the Workflow tool (`settings.workflowKeywordTriggerEnabled`;
              defaults to true). Null omits the key.
            '';
          };
          skipUsageWarning = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = ''
              Records that the multi-agent usage warning has been accepted
              (`settings.skipWorkflowUsageWarning`). Until it is set, auto
              permission mode prompts before running a workflow. Null omits
              the key.
            '';
          };
        };
      };
    };

    artifacts = mkOption {
      description = "The Artifact tool (publishing pages to claude.ai).";
      default = { };
      type = types.submodule {
        options = {
          enable = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = ''
              Enable or disable the Artifact tool for this user
              (`settings.enableArtifact`). Null (default) omits the key,
              leaving it enabled once the feature is available. To
              hard-disable, use `hardening.disableArtifact`.
            '';
          };
        };
      };
    };

    worktree = mkOption {
      description = ''
        Git worktree behaviour for `--worktree`, `EnterWorktree`, and agent
        isolation (`settings.worktree`).
      '';
      default = { };
      type = types.submodule {
        options = {
          symlinkDirectories = mkOption {
            type = types.listOf types.str;
            default = [ ];
            example = [
              "node_modules"
              ".cache"
            ];
            description = ''
              Directories symlinked from the main checkout into each new
              worktree, to avoid duplicating them on disk. Nothing is
              symlinked by default. Empty omits the key.
            '';
          };
          sparsePaths = mkOption {
            type = types.listOf types.str;
            default = [ ];
            description = ''
              Paths to include when creating a worktree, via git
              sparse-checkout (cone mode) — only these are written to disk.
              Worth setting in large monorepos. Empty omits the key.
            '';
          };
          baseRef = mkOption {
            type = types.nullOr (
              types.enum [
                "fresh"
                "head"
              ]
            );
            default = null;
            description = ''
              Which ref new worktrees branch from. `"fresh"` (Claude Code's
              default) uses `origin/<default-branch>`; `"head"` uses your
              current local HEAD so unpushed commits come along. Null omits
              the key.
            '';
          };
          bgIsolation = mkOption {
            type = types.nullOr (
              types.enum [
                "worktree"
                "none"
              ]
            );
            default = null;
            description = ''
              Isolation for background sessions in a repo. `"worktree"`
              (Claude Code's default) blocks Edit/Write in the main checkout
              until `EnterWorktree` runs; `"none"` lets background jobs edit
              the working copy directly. Null omits the key.
            '';
          };
        };
      };
    };

    memory = mkOption {
      description = ''
        Auto-memory: the store Claude reads from and writes to across
        sessions for a project.

        Note the interaction with `defaultSettings.env`, which sets
        `CLAUDE_CODE_DISABLE_AUTO_MEMORY=1` — the env var wins, so flip that
        off in `settings.env` if you set `autoMemoryEnabled = true`.
      '';
      default = { };
      type = types.submodule {
        options = {
          autoMemoryEnabled = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = ''
              Enable auto-memory (`settings.autoMemoryEnabled`). When false,
              Claude neither reads nor writes the auto-memory directory.
              Null omits the key.
            '';
          };
          autoMemoryDirectory = mkOption {
            type = types.nullOr types.str;
            default = null;
            description = ''
              Auto-memory storage directory, `~/`-expanded
              (`settings.autoMemoryDirectory`). Claude Code ignores this key
              in a checked-in project settings file for security. Null omits
              it; the default is
              `~/.claude/projects/<sanitized-cwd>/memory/`.
            '';
          };
          autoDreamEnabled = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = ''
              Background memory consolidation, a.k.a. auto-dream
              (`settings.autoDreamEnabled`). Overrides the server-side
              default when set. Null omits the key.
            '';
          };
        };
      };
    };

    skills = mkOption {
      description = ''
        How the skill listing is budgeted and which skills are exposed.
        Skills themselves ship through `plugins`.
      '';
      default = { };
      type = types.submodule {
        options = {
          skillListingMaxDescChars = mkOption {
            type = types.nullOr types.ints.positive;
            default = null;
            description = ''
              Per-skill description cap in the listing sent to Claude
              (`settings.skillListingMaxDescChars`; default 1536).
              Descriptions past this are truncated. Raising it costs
              per-turn context. Null omits the key.
            '';
          };
          skillListingBudgetFraction = mkOption {
            type = types.nullOr (types.numbers.between 0.0 1.0);
            default = null;
            example = 0.04;
            description = ''
              Fraction of the context window reserved for the whole skill
              listing (`settings.skillListingBudgetFraction`; default 0.01 =
              1%). When the listing overflows, descriptions are shortened to
              fit. Worth raising if you ship many skills. Null omits the key.
            '';
          };
          skillOverrides = mkOption {
            type = types.attrsOf (
              types.enum [
                "on"
                "name-only"
                "user-invocable-only"
                "off"
              ]
            );
            default = { };
            example = {
              some-noisy-skill = "name-only";
            };
            description = ''
              Per-skill listing overrides keyed by skill name
              (`settings.skillOverrides`). `"name-only"` lists the skill
              without its description, `"user-invocable-only"` hides it from
              the model but keeps `/name`, `"off"` hides it from both.
              Absent means on. Empty omits the key.
            '';
          };
        };
      };
    };

    compaction = mkOption {
      description = "Automatic context compaction.";
      default = { };
      type = types.submodule {
        options = {
          autoCompactEnabled = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = ''
              Automatically compact the conversation when context fills
              (`settings.autoCompactEnabled`). Null omits the key; Claude
              Code's own default is on.
            '';
          };
          autoCompactWindow = mkOption {
            type = types.nullOr (types.ints.between 100000 1000000);
            default = null;
            description = ''
              Auto-compact window size in tokens (`settings.autoCompactWindow`),
              between 100000 and 1000000. Null omits the key.
            '';
          };
          precomputeCompactionEnabled = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = ''
              Build the compaction summary in the background before it is
              needed (`settings.precomputeCompactionEnabled`). Only applies
              while auto-compact is on. Null omits the key — but note the
              module's `defaultSettings` turn this on.
            '';
          };
        };
      };
    };

    ui = mkOption {
      description = ''
        Interface toggles that map one-to-one onto their same-named
        `settings.*` keys. All null by default (Claude Code's own default
        applies) except where `defaultSettings` states otherwise.
      '';
      default = { };
      type = types.submodule {
        options =
          let
            toggle =
              key: text:
              mkOption {
                type = types.nullOr types.bool;
                default = null;
                description = "${text} (`settings.${key}`). Null omits the key.";
              };
          in
          {
            fileCheckpointingEnabled = toggle "fileCheckpointingEnabled" "Snapshot files before edits so `/rewind` can restore them";
            showThinkingSummaries = toggle "showThinkingSummaries" "Request API-side thinking summaries and show them inline and in the transcript view";
            showMessageTimestamps = toggle "showMessageTimestamps" "Stamp each message with its arrival time";
            terminalProgressBarEnabled = toggle "terminalProgressBarEnabled" "Emit OSC 9;4 progress sequences during long operations";
            todoFeatureEnabled = toggle "todoFeatureEnabled" "Enable the todo / task tracking panel";
            autoScrollEnabled = toggle "autoScrollEnabled" "Auto-scroll the conversation to the bottom (fullscreen renderer only)";
            wheelScrollAccelerationEnabled = toggle "wheelScrollAccelerationEnabled" "Ramp mouse-wheel scroll speed during fast scrolls (fullscreen renderer only)";
            prefersReducedMotion = toggle "prefersReducedMotion" "Reduce or disable animations (spinner shimmer, flash effects)";
            emojiCompletionEnabled = toggle "emojiCompletionEnabled" "Enable the `:emoji:` shortcode typeahead";
            promptSuggestionEnabled = toggle "promptSuggestionEnabled" "Enable prompt suggestions";
            syntaxHighlightingDisabled = toggle "syntaxHighlightingDisabled" "Disable syntax highlighting in diffs";
            terminalTitleFromRename = toggle "terminalTitleFromRename" "Let `/rename` update the terminal tab title";
            showClearContextOnPlanAccept = toggle "showClearContextOnPlanAccept" ''Offer a "clear context" option in the plan-approval dialog'';
          };
      };
    };

    voice = mkOption {
      description = "Voice dictation (`settings.voice` / `settings.voiceEnabled`).";
      default = { };
      type = types.submodule {
        options = {
          enabled = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = "Enable voice mode. Null omits the key.";
          };
          mode = mkOption {
            type = types.nullOr (
              types.enum [
                "hold"
                "tap"
              ]
            );
            default = null;
            description = ''
              `"hold"` (Claude Code's default) is hold-to-talk; `"tap"` taps
              to start and taps again to stop and submit. Null omits the key.
            '';
          };
          autoSubmit = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = ''
              Submit the prompt when hold-to-talk is released (hold mode
              only). Null omits the key.
            '';
          };
        };
      };
    };

    remoteControl = mkOption {
      description = ''
        Remote Control, cross-session messaging, and how spawned teammates
        run. To disable Remote Control outright, use
        `hardening.disableRemoteControl`.
      '';
      default = { };
      type = types.submodule {
        options = {
          remoteControlAtStartup = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = "Start the Remote Control bridge automatically each session. Null omits the key.";
          };
          isolatePeerMachines = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = ''
              Require explicit approval before `SendMessage` can reach a peer
              session on another machine. Null omits the key.
            '';
          };
          crossSessionInbound = mkOption {
            type = types.nullOr (
              types.enum [
                "accept"
                "hold"
                "refuse"
              ]
            );
            default = null;
            description = ''
              Inbound peer messages from your other sessions: `"accept"`
              delivers them, `"hold"` parks them for review without letting
              Claude act, `"refuse"` opts this session out. Null omits the
              key, which selects Claude Code's permission-mode-parity
              behaviour.
            '';
          };
          autoUploadSessions = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = "Mirror local sessions to claude.ai as view-only. Null omits the key.";
          };
          daemonColdStart = mkOption {
            type = types.nullOr (
              types.enum [
                "transient"
                "ask"
              ]
            );
            default = null;
            description = ''
              With no background service running: `"transient"` spawns one
              for this login session, `"ask"` offers to install it
              persistently. Null omits the key.
            '';
          };
          teammateMode = mkOption {
            type = types.nullOr (
              types.enum [
                "auto"
                "tmux"
                "iterm2"
                "in-process"
              ]
            );
            default = null;
            description = "How spawned teammates execute. Null omits the key.";
          };
          inputNeededNotifEnabled = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = "Push to mobile when a permission prompt or question is waiting. Null omits the key.";
          };
          agentPushNotifEnabled = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = "Allow Claude to push proactive mobile notifications. Null omits the key.";
          };
        };
      };
    };

    sandboxControl = mkOption {
      description = ''
        Scalar `settings.sandbox` knobs. The additive path lists live in
        `extraSandbox`; this is for the on/off switches around them.
      '';
      default = { };
      type = types.submodule {
        options = {
          enabled = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = "Run Bash tool commands inside the sandbox (`sandbox.enabled`). Null omits the key.";
          };
          failIfUnavailable = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = ''
              Exit at startup if `sandbox.enabled` is true but the sandbox
              cannot start (missing bwrap, unsupported platform). When false
              — Claude Code's default — you get a warning and commands run
              unsandboxed. Null omits the key.
            '';
          };
          autoAllowBashIfSandboxed = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = ''
              Skip the Bash permission prompt for commands that are running
              sandboxed (`sandbox.autoAllowBashIfSandboxed`; defaults to
              true). Null omits the key.
            '';
          };
          allowUnsandboxedCommands = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = ''
              Allow the Bash tool's `dangerouslyDisableSandbox` parameter to
              take effect (`sandbox.allowUnsandboxedCommands`; defaults to
              true). When false the parameter is ignored outright. Null
              omits the key.
            '';
          };
          excludedCommands = mkOption {
            type = types.listOf types.str;
            default = [ ];
            description = "Commands that never run sandboxed (`sandbox.excludedCommands`). Empty omits the key.";
          };
          filesystemDisabled = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = ''
              macOS and Linux/WSL only: skip filesystem isolation while
              keeping network and seccomp isolation
              (`sandbox.filesystem.disabled`). Sandboxed commands then get
              unrestricted host read/write, and `filesystem.denyRead` stops
              being enforced — for deployments whose goal is egress control
              only. Null omits the key.
            '';
          };
          strictNetworkAllowlist = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = ''
              Deterministically deny hosts outside
              `extraSandbox.network.allowedDomains` instead of prompting
              (`sandbox.network.strictAllowlist`). Applies to sandboxed
              commands only — in-process tools like WebFetch are not gated
              by it. Null omits the key.
            '';
          };
        };
      };
    };

    mcpControl = mkOption {
      description = ''
        Gating for how MCP servers *defined elsewhere* are approved — as
        opposed to `mcpServers`, which *defines* user-scope servers. Each
        sub-option is written to its same-named `settings.*` key when set.
      '';
      default = { };
      type = types.submodule {
        options = {
          enableAllProjectMcpServers = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = ''
              Auto-approve every server from a project's `.mcp.json`
              (`settings.enableAllProjectMcpServers`). Null (default) omits
              the key. Leaving this off is recommended: blanket-approving
              arbitrary project MCP servers runs unpinned code from whatever
              repo you open — prefer `enabledMcpjsonServers` for a trusted
              allowlist.
            '';
          };
          enabledMcpjsonServers = mkOption {
            type = types.listOf types.str;
            default = [ ];
            description = ''
              Allowlist of `.mcp.json` server names to approve
              (`settings.enabledMcpjsonServers`). Additive; empty omits.
            '';
          };
          disabledMcpjsonServers = mkOption {
            type = types.listOf types.str;
            default = [ ];
            description = ''
              Denylist of `.mcp.json` server names to reject
              (`settings.disabledMcpjsonServers`). Additive; empty omits.
            '';
          };
          disableClaudeAiConnectors = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = ''
              Disable claude.ai MCP connectors
              (`settings.disableClaudeAiConnectors`). Null (default) omits
              the key.
            '';
          };
        };
      };
    };

    hardening = mkOption {
      description = ''
        User-scope-effective lockdown toggles, each written to its
        same-named `settings.*` key when non-null. All null by default, so
        nothing is emitted unless explicitly set. Intended mainly for
        hardened downstreams (e.g. a container image); a normal interactive
        setup typically leaves these off.

        NOTE: managed-settings-only keys (`allowManagedHooksOnly`,
        `disableSideloadFlags`, `allowAllClaudeAiMcps`,
        `allowManagedMcpServersOnly`, ...) are deliberately NOT exposed here —
        Claude Code ignores them outside a system `managed-settings.json`,
        which this module does not write. A dedicated managed-settings target
        is future work.
      '';
      default = { };
      type = types.submodule {
        options =
          let
            toggle =
              key:
              mkOption {
                type = types.nullOr types.bool;
                default = null;
                description = "Sets `settings.${key}` when non-null.";
              };
          in
          {
            disableAllHooks = toggle "disableAllHooks";
            disableSkillShellExecution = toggle "disableSkillShellExecution";
            disableWorkflows = toggle "disableWorkflows";
            disableRemoteControl = toggle "disableRemoteControl";
            disableArtifact = toggle "disableArtifact";
            disableBundledSkills = toggle "disableBundledSkills";
            disableAgentView = toggle "disableAgentView";
          };
      };
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
          # Auto mode: a model classifier adjudicates each permission prompt
          # rather than stopping to ask, with the soft/hard deny sections
          # below it as the backstop. Only grantable at user scope — Claude
          # Code ignores `defaultMode: auto` from a repo settings file.
          defaultMode = "auto";
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
            "rtk is a transparent token-saving CLI proxy installed in this environment. A PreToolUse hook rewrites bare commands into `rtk <command>` form (e.g. `rtk grep ...`, `rtk git show ...`, `rtk ls ...`) purely to reduce token usage — rtk does not change what the command does. Judge `rtk <command>` exactly as you would judge `<command>` run directly: the leading `rtk` (or absolute-path `/run/current-system/sw/bin/rtk`) is a no-op wrapper that neither adds risk nor grants trust, and it never relaxes a BLOCK rule that would apply to the underlying command."
            # The repo's explicitly-trusted read-only / safe dev commands,
            # mirroring permissions.allow above. Listed so the classifier also
            # clears their rtk-wrapped and absolute-path forms.
            "The following read-only or otherwise safe development commands are explicitly trusted in this environment, whether run bare, rtk-wrapped, or resolved to an absolute `/run/current-system/sw/bin/` path: find, grep, ls, git show, git rev-parse, mkdir, python -m py_compile, black, isort."
          ];
        };
        alwaysThinkingEnabled = true;
        showThinkingSummaries = true;
        showTurnDuration = true;
        spinnerTipsEnabled = false;
        # Snapshot files before edits so /rewind can restore them, and build
        # the compaction summary ahead of the moment it is needed.
        fileCheckpointingEnabled = true;
        precomputeCompactionEnabled = true;
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
          additionalDirectories = mkOption {
            type = types.listOf types.str;
            default = [ ];
            example = [ "/home/me/notes" ];
            description = ''
              Extra `permissions.additionalDirectories` entries — directories
              outside the cwd that are in scope for file tools without a
              per-session `/add-dir`.
            '';
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

        - `extraSandbox.filesystem.allowRead` — paths Claude may read back
          despite a `denyRead` region covering them (e.g. an SSH agent
          socket, `known_hosts`).
        - `extraSandbox.filesystem.denyWrite` — paths Claude may NOT write
          despite the surrounding allow.
        - `extraSandbox.network.allowedDomains` — extra domains sandboxed
          commands may reach.

        Key names follow Claude Code's *settings* schema, which is not the
        same as the internal runtime shape the `/sandbox` view prints:
        `allowRead`/`denyRead`/`allowWrite`/`denyWrite` and
        `allowedDomains`/`deniedDomains`, not
        `allowWithinDeny`/`denyOnly`/`allowOnly`/`denyWithinAllow`/`allowedHosts`.
        Claude Code parses `sandbox.filesystem` and `sandbox.network` with
        closed schemas, so a stale key is dropped on load with no error —
        the rule simply never applies.

        Lists merge via the standard NixOS `listOf` semantics, so
        multiple modules can contribute (use `lib.mkBefore` /
        `lib.mkAfter` to order their entries). Scalar sandbox switches live
        in `sandboxControl`.
      '';
      default = { };
      type = types.submodule {
        options = {
          filesystem = mkOption {
            default = { };
            type = types.submodule {
              options = {
                allowRead = mkOption {
                  type = types.listOf types.str;
                  default = [ ];
                  description = ''
                    Extra `sandbox.filesystem.allowRead` paths — re-allowed
                    for reading inside a `denyRead` region, taking precedence
                    over it.
                  '';
                };
                denyRead = mkOption {
                  type = types.listOf types.str;
                  default = [ ];
                  description = ''
                    Extra `sandbox.filesystem.denyRead` paths, merged with
                    the paths from `Read(...)` deny permission rules.
                  '';
                };
                allowWrite = mkOption {
                  type = types.listOf types.str;
                  default = [ ];
                  description = ''
                    Extra `sandbox.filesystem.allowWrite` paths, merged with
                    the paths from `Edit(...)` allow permission rules.
                  '';
                };
                denyWrite = mkOption {
                  type = types.listOf types.str;
                  default = [ ];
                  description = ''
                    Extra `sandbox.filesystem.denyWrite` paths, merged with
                    the paths from `Edit(...)` deny permission rules.
                  '';
                };
              };
            };
          };
          network = mkOption {
            default = { };
            type = types.submodule {
              options = {
                allowedDomains = mkOption {
                  type = types.listOf types.str;
                  default = [ ];
                  description = "Extra `sandbox.network.allowedDomains` entries.";
                };
                deniedDomains = mkOption {
                  type = types.listOf types.str;
                  default = [ ];
                  description = ''
                    Extra `sandbox.network.deniedDomains` entries. Always
                    blocked, even when `allowedDomains` would match.
                  '';
                };
                allowUnixSockets = mkOption {
                  type = types.listOf types.str;
                  default = [ ];
                  description = ''
                    Extra `sandbox.network.allowUnixSockets` paths. macOS
                    only — Linux seccomp cannot filter sockets by path, so
                    entries are ignored there.
                  '';
                };
                allowMachLookup = mkOption {
                  type = types.listOf types.str;
                  default = [ ];
                  description = ''
                    Extra `sandbox.network.allowMachLookup` XPC/Mach service
                    names (macOS only). A single trailing `*` is allowed.
                  '';
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

    subagentStatusLine = mkOption {
      type = types.nullOr types.str;
      default = null;
      example = lib.literalExpression ''"''${pkgs.my-tool}/bin/subagent-status"'';
      description = ''
        Command for the per-subagent status line shown in the agent panel
        (`settings.subagentStatusLine`). It receives that row's context as
        JSON on stdin, and is independent of the main `statusLine`. Null
        (default) omits the key.
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

          hideVimModeIndicator = mkOption {
            type = types.nullOr types.bool;
            default = null;
            description = ''
              Hide Claude Code's built-in `-- INSERT --` / `-- VISUAL --`
              line below the prompt (`statusLine.hideVimModeIndicator`).
              Only worth setting when `editorMode = "vim"` and your status
              line renders the mode itself. Null (default) omits the key.
            '';
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

    # Merge user-scope MCP servers into ~/.claude.json (and each per-account
    # ~/.claude-<account>/.claude.json). Claude Code stores user-scope MCP
    # servers in .claude.json (NOT settings.json) and rewrites the file at
    # runtime, so we jq deep-merge (generated wins per-key, runtime keys
    # survive) rather than overwrite. Inert unless servers are set.
    home.activation.claudeMcpServersMerge = lib.mkIf (cfg.mcpServers != { }) (
      let
        generatedMcp = pkgs.writeText "claude-mcp-servers.json" (
          builtins.toJSON { mcpServers = cfg.mcpServers; }
        );
        targets = [
          ".claude.json"
        ]
        ++ map (account: "${accountDir account}/.claude.json") cfg.extraAccounts;
        mergeOne = rel: ''
          mcpFile=${config.home.homeDirectory}/${rel}

          run mkdir -p "$(dirname "$mcpFile")"

          if [[ -L "$mcpFile" ]]; then
            run unlink "$mcpFile"
          fi

          if [[ -f "$mcpFile" ]]; then
            tmpFile=$(mktemp)
            run ${lib.getExe pkgs.jq} -s '.[0] * .[1]' "$mcpFile" ${generatedMcp} > "$tmpFile"
            run mv "$tmpFile" "$mcpFile"
          else
            run cp ${generatedMcp} "$mcpFile"
          fi

          run chmod 600 "$mcpFile"
        '';
      in
      lib.hm.dag.entryAfter [ "writeBoundary" ] (lib.concatMapStrings mergeOne targets)
    );
  };
}
