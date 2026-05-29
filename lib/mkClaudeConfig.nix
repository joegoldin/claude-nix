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
  settings ? { },
  # Optional statusline config — pass attrs to be merged into settings AND a
  # JSON blob to be written to statusline-config.json. Either may be null.
  statusLineSettings ? { },
  statusLineConfigJSON ? null,
  # Markdown written to CLAUDE.md. Empty string omits the file.
  globalClaudeMd ? "",
}:
let
  defaultsWithExtra = lib.recursiveUpdate defaultSettings {
    permissions = {
      allow = (defaultSettings.permissions.allow or [ ]) ++ extraPermissions.allow;
      ask = (defaultSettings.permissions.ask or [ ]) ++ extraPermissions.ask;
      deny = (defaultSettings.permissions.deny or [ ]) ++ extraPermissions.deny;
    };
  };

  mergedSettings = lib.recursiveUpdate (lib.recursiveUpdate defaultsWithExtra settings) statusLineSettings;

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
