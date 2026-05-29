{
  buildEnv,
  runCommand,
  formats,
  lib,
}:
{
  name,
  description,
  mcpServers ? { },
  lspServers ? { },
  skills ? [ ],
  commands ? [ ],
  agents ? [ ],
  # Hook configuration. Two complementary inputs, either or both:
  #   hooks    : attrs serialized to <plugin>/hooks/hooks.json. Use
  #              ${CLAUDE_PLUGIN_ROOT} inside command strings so hooks
  #              resolve relative to the plugin's own path at runtime.
  #   hooksDir : a derivation (or path) whose contents are copied under
  #              <plugin>/hooks/. Use this to ship the actual script
  #              files referenced by hooks.json.
  hooks ? null,
  hooksDir ? null,
}:
let
  json = formats.json { };

  pluginJsonDrv = runCommand "${name}-plugin-json" { } ''
    mkdir -p $out/.claude-plugin
    cp ${
      json.generate "plugin.json" {
        inherit
          name
          description
          mcpServers
          lspServers
          ;
      }
    } $out/.claude-plugin/plugin.json
  '';

  hasHooks = hooks != null || hooksDir != null;

  hooksDrv =
    if !hasHooks then
      null
    else
      runCommand "${name}-plugin-hooks" { } ''
        mkdir -p $out/hooks
        ${lib.optionalString (hooks != null) ''
          cp ${json.generate "hooks.json" hooks} $out/hooks/hooks.json
        ''}
        ${lib.optionalString (hooksDir != null) ''
          cp -r ${hooksDir}/. $out/hooks/
          chmod -R u+w $out/hooks
        ''}
      '';
in
buildEnv {
  inherit name;
  paths =
    [ pluginJsonDrv ]
    ++ skills
    ++ commands
    ++ agents
    ++ lib.optional hasHooks hooksDrv;
  pathsToLink = [
    "/.claude-plugin"
    "/skills"
    "/commands"
    "/agents"
    "/hooks"
  ];
  passthru.meta = {
    inherit name description;
  };
}
