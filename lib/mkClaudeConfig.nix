{
  pkgs,
  lib,
}:
{
  # Settings layering — same shape as the home-manager options.
  defaultSettings ? { },
  extraPermissions ? {
    allow = [ ];
    ask = [ ];
    deny = [ ];
  },
  # First-class settings-shaped option values (model, effortLevel,
  # hardening.*, mcpControl.*, ...). Null scalars / empty lists are dropped by
  # the merge, so unset options omit their keys. Sits above defaultSettings but
  # below `settings` (the raw escape hatch).
  optionSettings ? { },
  settings ? { },
  # Optional statusline config — pass attrs to be merged into settings AND a
  # JSON blob to be written to statusline-config.json. Either may be null.
  statusLineSettings ? { },
  statusLineConfigJSON ? null,
  # Markdown written to CLAUDE.md. Empty string omits the file.
  globalClaudeMd ? "",
}:
let
  mergedSettings = import ./mergeClaudeSettings.nix { inherit lib; } {
    inherit
      defaultSettings
      extraPermissions
      optionSettings
      settings
      statusLineSettings
      ;
  };

  settingsFile = pkgs.writeText "claude-settings.json" (builtins.toJSON mergedSettings);
  claudeMdFile =
    if globalClaudeMd != "" then pkgs.writeText "claude-CLAUDE.md" globalClaudeMd else null;
  statusLineFile =
    if statusLineConfigJSON != null then
      pkgs.writeText "claude-statusline-config.json" statusLineConfigJSON
    else
      null;
in
pkgs.runCommand "claude-config" { } ''
  mkdir -p $out
  cp ${settingsFile} $out/settings.json
  ${lib.optionalString (claudeMdFile != null) ''cp ${claudeMdFile} $out/CLAUDE.md''}
  ${lib.optionalString (statusLineFile != null) ''cp ${statusLineFile} $out/statusline-config.json''}
''
