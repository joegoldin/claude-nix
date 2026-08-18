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
  # Auto-mode classifier rules, concatenated onto
  # `defaultSettings.autoMode.<section>` rather than replacing it.
  extraAutoMode ? { },
  # First-class settings-shaped option values (model, effortLevel,
  # hardening.*, mcpControl.*, ...). Null scalars / empty lists are dropped by
  # the merge, so unset options omit their keys. Sits above defaultSettings but
  # below `settings` (the raw escape hatch).
  optionSettings ? { },
  settings ? { },
  # Optional statusline config — pass attrs to be merged into settings AND an
  # already-rendered file to be copied to statusline-config.json (that is what
  # agent-statusline's `renderConfig` hands back). Either may be null.
  statusLineSettings ? { },
  statusLineConfigFile ? null,
  # Markdown written to CLAUDE.md. Empty string omits the file.
  globalClaudeMd ? "",
}:
let
  mergedSettings = import ./mergeClaudeSettings.nix { inherit lib; } {
    inherit
      defaultSettings
      extraPermissions
      extraAutoMode
      optionSettings
      settings
      statusLineSettings
      ;
  };

  settingsFile = pkgs.writeText "claude-settings.json" (builtins.toJSON mergedSettings);
  claudeMdFile =
    if globalClaudeMd != "" then pkgs.writeText "claude-CLAUDE.md" globalClaudeMd else null;
in
pkgs.runCommand "claude-config" { } ''
  mkdir -p $out
  cp ${settingsFile} $out/settings.json
  ${lib.optionalString (claudeMdFile != null) "cp ${claudeMdFile} $out/CLAUDE.md"}
  ${lib.optionalString (
    statusLineConfigFile != null
  ) "cp ${statusLineConfigFile} $out/statusline-config.json"}
''
