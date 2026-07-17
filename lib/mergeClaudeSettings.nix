{ lib }:
# Pure settings.json merge used by mkClaudeConfig (and directly by the
# checks.eval-settings test). Kept free of pkgs / home-manager so the
# precedence and null-drop rules can be exercised without IFD.
#
# Precedence, low -> high:
#   defaultSettings
#   < extraPermissions            (concatenated into permissions.{allow,ask,deny})
#   < optionSettings              (first-class options; nulls / empty lists dropped)
#   < settings                    (raw escape hatch, wins outright)
#   < statusLineSettings          (statusline command, applied last)
{
  defaultSettings ? { },
  extraPermissions ? {
    allow = [ ];
    ask = [ ];
    deny = [ ];
  },
  # Values from the first-class settings-shaped options (model, effortLevel,
  # hardening.*, mcpControl.*, ...). Any null scalar or empty list is dropped
  # so an *unset* option omits its key entirely rather than emitting `null`.
  optionSettings ? { },
  settings ? { },
  statusLineSettings ? { },
}:
let
  defaultsWithExtra = lib.recursiveUpdate defaultSettings {
    permissions = {
      allow = (defaultSettings.permissions.allow or [ ]) ++ (extraPermissions.allow or [ ]);
      ask = (defaultSettings.permissions.ask or [ ]) ++ (extraPermissions.ask or [ ]);
      deny = (defaultSettings.permissions.deny or [ ]) ++ (extraPermissions.deny or [ ]);
    };
  };

  # Drop unset options: null scalars and empty lists never reach settings.json.
  cleanOptionSettings = lib.filterAttrs (_: v: v != null && v != [ ]) optionSettings;

  defaultsWithOptions = lib.recursiveUpdate defaultsWithExtra cleanOptionSettings;
in
lib.recursiveUpdate (lib.recursiveUpdate defaultsWithOptions settings) statusLineSettings
