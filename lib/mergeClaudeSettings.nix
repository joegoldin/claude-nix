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
  # Auto-mode classifier rules, concatenated onto
  # `defaultSettings.autoMode.<section>` rather than replacing it: the shipped
  # `allow` list leads with the literal "$defaults", and a caller adding one
  # rule must not silently drop it (and with it every built-in allow rule).
  extraAutoMode ? { },
  # Values from the first-class settings-shaped options (model, effortLevel,
  # hardening.*, mcpControl.*, ...). Any null scalar or empty list is dropped
  # so an *unset* option omits its key entirely rather than emitting `null`.
  optionSettings ? { },
  settings ? { },
  statusLineSettings ? { },
}:
let
  additionalDirectories =
    (defaultSettings.permissions.additionalDirectories or [ ])
    ++ (extraPermissions.additionalDirectories or [ ]);

  autoMode = lib.filterAttrs (_: v: v != [ ]) (
    lib.genAttrs [
      "allow"
      "soft_deny"
      "hard_deny"
      "environment"
    ] (section: (defaultSettings.autoMode.${section} or [ ]) ++ (extraAutoMode.${section} or [ ]))
  );

  defaultsWithExtra = lib.recursiveUpdate defaultSettings (
    {
      permissions = {
        allow = (defaultSettings.permissions.allow or [ ]) ++ (extraPermissions.allow or [ ]);
        ask = (defaultSettings.permissions.ask or [ ]) ++ (extraPermissions.ask or [ ]);
        deny = (defaultSettings.permissions.deny or [ ]) ++ (extraPermissions.deny or [ ]);
      }
      # Omitted entirely when nothing contributes, so the common case doesn't
      # write an empty list into settings.json.
      // lib.optionalAttrs (additionalDirectories != [ ]) { inherit additionalDirectories; };
    }
    // lib.optionalAttrs (autoMode != { }) { inherit autoMode; }
  );

  # Drop unset options: null scalars and empty lists never reach settings.json.
  # Applied recursively so a grouped option submodule (e.g. `worktree`,
  # `autoMode`) with every member unset collapses away entirely instead of
  # emitting `{ baseRef = null; ... }`. Only `optionSettings` is pruned —
  # `settings` is the raw escape hatch, where an explicit `{ }` or `null` is
  # taken at face value.
  prune =
    v:
    if builtins.isAttrs v then
      let
        kept = lib.filterAttrs (_: x: x != null && x != [ ] && !(builtins.isAttrs x && x == { })) (
          lib.mapAttrs (_: prune) v
        );
      in
      kept
    else
      v;

  cleanOptionSettings = prune optionSettings;

  defaultsWithOptions = lib.recursiveUpdate defaultsWithExtra cleanOptionSettings;
in
lib.recursiveUpdate (lib.recursiveUpdate defaultsWithOptions settings) statusLineSettings
