{
  pkgs,
  lib,
  writeShellApplication,
}:
{
  plugins ? [ ],
  # Optional path to a markdown file passed as --append-system-prompt-file.
  # `null` (default) omits the flag entirely.
  appendSystemPromptFile ? null,
}:
let
  pluginDirFlags = lib.cli.toCommandLineShellGNU { } { plugin-dir = plugins; };
  appendFlag =
    if appendSystemPromptFile != null then
      "--append-system-prompt-file ${appendSystemPromptFile}"
    else
      "";
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
