{
  description = "Claude Nix";
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable-small";
    flake-utils.url = "github:numtide/flake-utils";
    # The statusline binary and its shared config schema, extracted out of this
    # repo so every harness renders the same status line from the same options.
    agent-statusline = {
      url = "github:joegoldin/agent-statusline";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };
  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
      agent-statusline,
    }:
    {
      homeManagerModules.default = import ./modules/home-manager.nix;
      homeManagerModules.claude-nix = import ./modules/home-manager.nix;
    }
    // flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs {
          inherit system;
          config.allowUnfreePredicate = pkg: builtins.elem (lib.getName pkg) [ "claude-code" ];
        };
        inherit (nixpkgs) lib;

        # Import library functions
        claudeLib = import ./lib { inherit pkgs; };
      in
      {
        # Export library functions for use by other flakes
        lib = claudeLib;

        packages.node-mcp-servers = pkgs.callPackage ./node-mcp-servers { };

        packages.agent-statusline = agent-statusline.packages.${pkgs.system}.agent-statusline;

        packages.plugin-chromium = claudeLib.mkPlugin {
          name = "chromium";
          description = "Chromium Devtools MCP";
          mcpServers.chromium = {
            command = "${self.packages.${system}.node-mcp-servers}/node_modules/.bin/chrome-devtools-mcp";
            args = [
              "--executablePath"
              (lib.getExe pkgs.chromium)
            ];
          };
          skills = [
            (claudeLib.mkSkill {
              name = "webpage-to-screenshot";
              description = "Turns a webpage into a screenshot";
              allowed-tools = [
                "mcp__plugin_chromium_chromium__navigate_page"
                "mcp__plugin_chromium_chromium__take_screenshot"
              ];
            } ''
              Navigates to a page using the mcp__plugin_chromium_chromium__navigate_page tool and then takes a screenshot using mcp__plugin_chromium_chromium__take_screenshot
            '')
            (claudeLib.mkSkill {
              name = "web-performance-audit";
              description = "Comprehensive performance audit of a webpage with Core Web Vitals, network analysis, and error detection";
              allowed-tools = [
                "mcp__plugin_chromium_chromium__navigate_page"
                "mcp__plugin_chromium_chromium__new_page"
                "mcp__plugin_chromium_chromium__performance_start_trace"
                "mcp__plugin_chromium_chromium__performance_stop_trace"
                "mcp__plugin_chromium_chromium__performance_analyze_insight"
                "mcp__plugin_chromium_chromium__list_network_requests"
                "mcp__plugin_chromium_chromium__get_network_request"
                "mcp__plugin_chromium_chromium__list_console_messages"
                "mcp__plugin_chromium_chromium__get_console_message"
                "mcp__plugin_chromium_chromium__take_screenshot"
                "mcp__plugin_chromium_chromium__take_snapshot"
              ];
            } ''
                You are a web performance expert. When asked to audit a webpage, follow this comprehensive workflow:

                ## Audit Workflow

                1. **Navigate & Start Tracing**
                   - Navigate to the target URL
                   - Start a performance trace with automatic page reload
                   - This captures Core Web Vitals and performance metrics

                2. **Stop Trace & Analyze Results**
                   - Stop the performance trace
                   - Review Core Web Vitals (LCP, FID, CLS, etc.)
                   - Identify performance insights and bottlenecks
                   - For any critical insights, use performance_analyze_insight to get detailed information

                3. **Network Analysis**
                   - List all network requests from the page load
                   - Identify slow requests (>1s response time)
                   - Find large resources (>500KB)
                   - Check for failed requests (status 4xx/5xx)
                   - Get detailed information on problematic requests

                4. **Console Error Detection**
                   - List all console messages
                   - Filter for errors and warnings
                   - Get full details of any critical errors
                   - Report JavaScript errors, CSP violations, etc.

                5. **Visual Capture**
                   - Take a screenshot of the page in its final state
                   - This helps visualize layout issues or rendering problems

                6. **Generate Comprehensive Report**
                   Provide a structured report with:
                   - **Executive Summary**: Overall performance grade (A/B/C/D/F)
                   - **Core Web Vitals**: LCP, FID, CLS scores with pass/fail
                   - **Performance Insights**: Top 3-5 issues affecting speed
                   - **Network Issues**: Slow/failed/large requests
                   - **JavaScript Errors**: Critical errors found
                   - **Recommendations**: Prioritized action items
                   - **Screenshot**: Visual reference

                ## Best Practices

                - Always wait for traces to complete fully
                - Prioritize insights by impact (high/medium/low)
                - Be specific in recommendations (not just "optimize images")
                - Compare metrics against Web Vitals thresholds:
                  - LCP: Good <2.5s, Needs Improvement 2.5-4s, Poor >4s
                  - FID: Good <100ms, Needs Improvement 100-300ms, Poor >300ms
                  - CLS: Good <0.1, Needs Improvement 0.1-0.25, Poor >0.25
                - Group related issues together (e.g., all render-blocking resources)


                ## Example Usage

                User: "Audit https://example.com"

                You should:
                1. Navigate and trace the page
                2. Analyze all performance data
                3. Check network and console
                4. Screenshot the result
                5. Deliver a clear, actionable report
            '')
          ];
        };

        packages.plugin-nix = claudeLib.mkPlugin {
          name = "nix";
          description = "Nix development tools and helpers";
          skills = [
            (claudeLib.mkSkill {
              name = "nix-helper";
              description = "Helps with Nix development and formatting";
              allowed-tools = [
                "Bash(${pkgs.statix}/bin/statix)"
                "Bash(${pkgs.nixfmt}/bin/nixfmt)"
              ];
            } ''
              You are a Nix expert. When working with Nix files:

              1. ALWAYS run `${pkgs.statix}/bin/statix check .` to find anti-patterns
              2. ADDRESS all issues found
              3. ALWAYS format files with `${pkgs.nixfmt}/bin/nixfmt`

              Be pedantic about best practices and code quality.
            '')
          ];
          lspServers = {
            nix = {
              command = lib.getExe pkgs.nixd;
              extensionToLanguage = {
                ".nix" = "nix";
              };
            };
          };
          commands = [
            (claudeLib.mkCommand {
              name = "format-nix";
              description = "Format all Nix files in the project";
              allowed-tools = [
                "Bash(${pkgs.nixfmt}/bin/nixfmt)"
                "Bash(${pkgs.fd}/bin/fd)"
              ];
              argument-hint = "[directory]";
            } ''
              Format all Nix files using nixfmt.

              If an argument is provided, format files in that directory.
              Otherwise, format all .nix files in the current directory.

              Use: ${pkgs.fd}/bin/fd -e nix -x ${pkgs.nixfmt}/bin/nixfmt
            '')
          ];
          agents = [
            (claudeLib.mkAgent {
              name = "nix-analyzer";
              description = "Specialized agent for analyzing Nix code";
              tools = [
                "Read"
                "Glob"
                "Grep"
                "Bash(${pkgs.statix}/bin/statix)"
              ];
            } ''
              You are an expert Nix code analyzer. When asked to analyze Nix code:

              1. Search for all .nix files in the project
              2. Run statix to identify anti-patterns
              3. Analyze the flake structure and dependencies
              4. Provide recommendations for improvements
              5. Explain any complex Nix patterns found

              Be thorough and educational in your analysis.
            '')
          ];
        };

        packages.plugin-pandoc = claudeLib.mkPlugin {
          name = "pandoc";
          description = "Document conversion with pandoc";
          skills = [
            (pkgs.callPackage ./skills/pandoc.nix { inherit claudeLib; })
          ];
        };

        packages.default = claudeLib.mkClaude {
          plugins = [
            self.packages.${system}.plugin-chromium
            self.packages.${system}.plugin-nix
            self.packages.${system}.plugin-pandoc
          ];
        };

        # Pure eval test for the settings.json merge: precedence (settings >
        # optionSettings > extraPermissions > defaultSettings) and the null /
        # empty-list drop that makes unset options omit their keys.
        checks.eval-settings =
          let
            s = claudeLib.mergeClaudeSettings {
              defaultSettings = {
                cleanupPeriodDays = 14;
                permissions.allow = [ "Bash(ls:*)" ];
                autoMode.allow = [ "$defaults" ];
              };
              extraPermissions.allow = [ "Bash(fd:*)" ];
              extraPermissions.additionalDirectories = [ "/srv/notes" ];
              extraAutoMode.allow = [ "trust the widget CLI" ];
              optionSettings = {
                model = "opus";
                effortLevel = null; # unset -> dropped
                fallbackModel = [ ]; # empty -> dropped
                editorMode = "vim";
                enabledMcpjsonServers = [ "nixos" ];
                disableWorkflows = true;
                disableArtifact = null; # unset -> dropped
                tui = "fullscreen";
                # Grouped submodules: pruning is recursive, so a group whose
                # members are all unset must vanish rather than emit nulls.
                worktree = { baseRef = "head"; bgIsolation = null; sparsePaths = [ ]; };
                voice = { enabled = null; mode = null; autoSubmit = null; };
              };
              settings.model = "claude-sonnet-5"; # escape hatch wins
            };
            assertions = [
              { name = "settings.model wins over optionSettings.model"; cond = s.model == "claude-sonnet-5"; }
              { name = "editorMode emitted"; cond = s.editorMode == "vim"; }
              { name = "enabledMcpjsonServers emitted"; cond = s.enabledMcpjsonServers == [ "nixos" ]; }
              { name = "disableWorkflows emitted"; cond = s.disableWorkflows == true; }
              { name = "null effortLevel dropped"; cond = !(s ? effortLevel); }
              { name = "empty fallbackModel dropped"; cond = !(s ? fallbackModel); }
              { name = "null disableArtifact dropped"; cond = !(s ? disableArtifact); }
              { name = "defaultSettings preserved"; cond = s.cleanupPeriodDays == 14; }
              { name = "extraPermissions concatenated"; cond = s.permissions.allow == [ "Bash(ls:*)" "Bash(fd:*)" ]; }
              { name = "additionalDirectories concatenated"; cond = s.permissions.additionalDirectories == [ "/srv/notes" ]; }
              # The classifier's built-in rules ride in on the literal
              # "$defaults"; an added rule must extend that list, not replace it.
              { name = "extraAutoMode appends after defaults"; cond = s.autoMode.allow == [ "$defaults" "trust the widget CLI" ]; }
              { name = "untouched autoMode section omitted"; cond = !(s.autoMode ? hard_deny); }
              { name = "tui emitted"; cond = s.tui == "fullscreen"; }
              { name = "set group member survives"; cond = s.worktree.baseRef == "head"; }
              { name = "null group member dropped"; cond = !(s.worktree ? bgIsolation); }
              { name = "empty list in group dropped"; cond = !(s.worktree ? sparsePaths); }
              { name = "all-unset group dropped entirely"; cond = !(s ? voice); }
            ];
            failures = builtins.filter (a: !a.cond) assertions;
          in
          if failures == [ ] then
            pkgs.runCommand "eval-settings-pass" { } "touch $out"
          else
            throw "eval-settings failed: ${lib.concatMapStringsSep ", " (a: a.name) failures}";

        # The statusline schema now lives in the agent-statusline flake, so the
        # regression that matters here is no longer "does the Go build pass"
        # (that check moved with the source) but "does mounting the shared
        # schema still render the exact config file existing installs already
        # have on disk". Pins the two answers claude-nix owns — barWidth 8,
        # where the shared default is Go's 10, and the package — plus the fact
        # that the harness-only keys stay out of the binary's config.
        checks.eval-statusline =
          let
            statusline = import ./lib/agentStatusline.nix { inherit pkgs; };
            sl =
              (lib.evalModules {
                modules = [
                  { options.statusLine = statusline.statusLineOption; }
                  { statusLine = { enable = true; hideVimModeIndicator = true; }; }
                ];
              }).config.statusLine;
            rendered = builtins.fromJSON (statusline.renderConfig sl).text;
            expected = {
              activityRows = 4;
              barWidth = 8;
              gitCacheTtlSeconds = 5;
              hideWhenIdle = true;
              padding = 0;
              refreshInterval = 1;
              sevenDayThreshold = 50;
              tokenFormat = "compact";
              transcriptWindowSeconds = 300;
              widgets = {
                hide = [ ];
                row1 = [ "model" "cwd" "git" "duration" "usage5h" "usage7d" ];
                row2 = [ "context" "tokens" "burnRate" "voice" "compaction" "pr" "cost" ];
              };
            };
            assertions = [
              { name = "rendered config unchanged"; cond = rendered == expected; }
              { name = "barWidth stays 8, not the shared default"; cond = sl.barWidth == 8; }
              { name = "package defaults to the agent-statusline binary"; cond = lib.hasSuffix "/bin/agent-statusline" (lib.getExe sl.package); }
              # enable / package / hideVimModeIndicator / toolTiming are
              # harness-level: they drive settings.json, and the binary must
              # never see them in its own config.
              { name = "enable excluded from the config"; cond = !(rendered ? enable); }
              { name = "package excluded from the config"; cond = !(rendered ? package); }
              { name = "hideVimModeIndicator excluded from the config"; cond = !(rendered ? hideVimModeIndicator); }
              { name = "toolTiming excluded from the config"; cond = !(rendered ? toolTiming); }
              # ... but both must still reach the settings.json side.
              { name = "hideVimModeIndicator readable for settings.json"; cond = sl.hideVimModeIndicator == true; }
              { name = "toolTiming defaults on for the timing hooks"; cond = sl.toolTiming; }
            ];
            failures = builtins.filter (a: !a.cond) assertions;
          in
          if failures == [ ] then
            pkgs.runCommand "eval-statusline-pass" { } "touch $out"
          else
            throw "eval-statusline failed: ${lib.concatMapStringsSep ", " (a: a.name) failures}";
      }
    );
}
