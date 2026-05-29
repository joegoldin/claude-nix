{
  pkgs,
  lib,
  writeShellApplication,
}:
{
  plugins ? [ ],
  # Markdown / text appended to Claude Code's default system prompt via
  # --append-system-prompt-file. mkClaude materializes a Nix-store file
  # for the content so callers don't have to. Empty string (default)
  # omits the flag.
  appendSystemPrompt ? "",
  # Per-project settings overrides keyed by project identifier. The
  # wrapper detects the current project (cwd's git origin URL basename,
  # falling back to git toplevel basename, then $PWD basename) and, on
  # exact match, prepends `--settings <path>` so Claude Code merges the
  # value attrs on top of project/user/managed settings.
  #
  # Example:
  #   projectSettings = {
  #     claude-container = {
  #       hooks.PreToolUse = [ { matcher = "Bash"; hooks = [ ... ]; } ];
  #     };
  #   };
  projectSettings ? { },
}:
let
  pluginDirFlags = lib.cli.toCommandLineShellGNU { } { plugin-dir = plugins; };

  appendFile =
    if appendSystemPrompt != "" then
      pkgs.writeText "claude-append-system-prompt" appendSystemPrompt
    else
      null;
  appendFlag =
    if appendFile != null then "--append-system-prompt-file ${appendFile}" else "";

  staticFlags = lib.concatStringsSep " " (
    lib.filter (s: s != "") [
      pluginDirFlags
      appendFlag
    ]
  );

  # Materialize one Nix-store JSON file per projectSettings entry and
  # build the case-arm shell snippet the wrapper uses to map a detected
  # project name to its --settings flag.
  projectNames = builtins.attrNames projectSettings;
  projectCases = lib.concatMapStringsSep "\n  " (name:
    let
      file = pkgs.writeText "claude-${name}-settings.json"
        (builtins.toJSON projectSettings.${name});
    in
    "${lib.escapeShellArg name}) project_args=(--settings ${file}) ;;"
  ) projectNames;

  hasProjectSettings = projectNames != [ ];
in
writeShellApplication {
  name = "claude";
  runtimeInputs = [ pkgs.claude-code ];
  text = ''
    project_args=()
    ${lib.optionalString hasProjectSettings ''
      # Detect the active project. Prefer the git origin URL basename
      # (stable across worktrees), then the git toplevel basename, then
      # the cwd basename. The git commands deliberately tolerate
      # not-a-repo / no-origin / no-git-binary; all paths fall through
      # to a safe "no override" default.
      _cnix_project=""
      _cnix_remote=""
      _cnix_remote="$(git config --get remote.origin.url 2>/dev/null || true)"
      if [ -n "$_cnix_remote" ]; then
        _cnix_project="$(basename "''${_cnix_remote%.git}")"
      fi
      if [ -z "$_cnix_project" ]; then
        _cnix_top="$(git rev-parse --show-toplevel 2>/dev/null || true)"
        if [ -n "$_cnix_top" ]; then
          _cnix_project="$(basename "$_cnix_top")"
        fi
      fi
      if [ -z "$_cnix_project" ]; then
        _cnix_project="$(basename "$PWD")"
      fi
      case "$_cnix_project" in
        ${projectCases}
      esac
    ''}
    exec claude ${staticFlags} "''${project_args[@]}" "$@"
  '';
}
