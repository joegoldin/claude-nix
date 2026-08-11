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
  # Component derivations for the newer plugin surfaces. Each is a list of
  # derivations laying files out under the matching top-level directory
  # (output-styles/, themes/, workflows/, monitors/), which Claude Code
  # auto-loads from the plugin root.
  outputStyles ? [ ],
  themes ? [ ],
  workflows ? [ ],
  monitors ? [ ],
  # Optional plugin.json metadata. Omitted from the manifest when unset, so a
  # plugin that declares none renders the same manifest it always did.
  version ? null,
  author ? null,
  homepage ? null,
  repository ? null,
  license ? null,
  keywords ? [ ],
  # Free-form manifest passthrough for anything not modelled above
  # (userConfig, dependencies, defaultEnabled, metadata, ...).
  extraManifest ? { },
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

  manifest = {
    inherit
      name
      description
      mcpServers
      lspServers
      ;
  }
  // lib.filterAttrs (_: v: v != null && v != [ ]) {
    inherit
      version
      author
      homepage
      repository
      license
      keywords
      ;
  }
  // extraManifest;

  pluginJsonDrv = runCommand "${name}-plugin-json" { } ''
    mkdir -p $out/.claude-plugin
    cp ${json.generate "plugin.json" manifest} $out/.claude-plugin/plugin.json
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
  paths = [
    pluginJsonDrv
  ]
  ++ skills
  ++ commands
  ++ agents
  ++ outputStyles
  ++ themes
  ++ workflows
  ++ monitors
  ++ lib.optional hasHooks hooksDrv;
  # Only link the component directories this plugin actually ships. buildEnv
  # would otherwise materialize an empty dir for every name listed, leaving
  # Claude Code a handful of empty surfaces to scan.
  pathsToLink = [
    "/.claude-plugin"
  ]
  ++ lib.optional (skills != [ ]) "/skills"
  ++ lib.optional (commands != [ ]) "/commands"
  ++ lib.optional (agents != [ ]) "/agents"
  ++ lib.optional hasHooks "/hooks"
  ++ lib.optional (outputStyles != [ ]) "/output-styles"
  ++ lib.optional (themes != [ ]) "/themes"
  ++ lib.optional (workflows != [ ]) "/workflows"
  ++ lib.optional (monitors != [ ]) "/monitors";
  passthru.meta = {
    inherit name description;
  };
}
