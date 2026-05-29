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
  allFlags = lib.concatStringsSep " " (
    lib.filter (s: s != "") [
      pluginDirFlags
      appendFlag
    ]
  );
in
writeShellApplication {
  name = "claude";
  runtimeInputs = [ pkgs.claude-code ];
  text = ''
    exec claude ${allFlags} "$@"
  '';
}
