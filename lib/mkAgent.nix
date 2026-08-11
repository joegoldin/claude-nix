{
  writeTextFile,
  lib,
}:
{
  name,
  description,
  # Tools available to this agent. Replaces the default set.
  tools ? [ ],
  # Tools removed from the default set. Ignored by Claude Code if `tools` is set.
  disallowedTools ? [ ],
  # Skills preloaded for this agent.
  skills ? [ ],
  model ? null,
  # Thinking effort: "low" | "medium" | "high" | "max", or an integer.
  effort ? null,
  # Permission mode the agent runs in: "default" | "acceptEdits" |
  # "bypassPermissions" | "plan" | "dontAsk" | "auto".
  permissionMode ? null,
  # Maximum conversation turns before the agent stops.
  maxTurns ? null,
  # Memory scope: "user" | "project" | "local".
  memory ? null,
  # Filesystem isolation; "worktree" runs the agent in a temporary worktree.
  isolation ? null,
  # Run in the background by default, reporting back as a task notification.
  background ? null,
  # Auto-submitted first message when this agent drives the main session
  # (via `--agent` or `settings.agent`). Unused when spawned as a subagent.
  initialPrompt ? null,
  # Display colour in the agents UI.
  color ? null,
  # MCP servers connected while this agent runs, and hooks registered for its
  # lifetime. Both are nested structures, emitted as JSON (valid YAML flow
  # style) rather than block YAML.
  mcpServers ? { },
  hooks ? { },
  # Escape hatch for any frontmatter field not modelled above.
  extraFrontmatter ? { },
}:
content:
let
  # Claude Code accepts either a scalar or an array for the list-shaped agent
  # fields. Lists are comma-joined (the documented form, and what an entry
  # like "Bash(git log:*)" needs so a space join can't shear it); nested
  # structures are emitted as JSON, which is valid YAML flow style.
  renderValue =
    value:
    if builtins.isBool value then
      lib.boolToString value
    else if builtins.isInt value || builtins.isFloat value then
      toString value
    else if builtins.isList value then
      lib.concatStringsSep ", " value
    else if builtins.isAttrs value then
      builtins.toJSON value
    else
      value;

  scalars = {
    inherit
      name
      description
      model
      effort
      permissionMode
      maxTurns
      memory
      isolation
      background
      initialPrompt
      color
      ;
  };

  collections = {
    inherit
      tools
      disallowedTools
      skills
      mcpServers
      hooks
      ;
  };

  # An unset scalar and an empty collection both drop their key: an empty
  # `tools:` line would hand the agent no tools at all rather than the default
  # set, and Claude Code treats an absent key as "inherit the default".
  present =
    lib.filterAttrs (_: v: v != null) scalars
    // lib.filterAttrs (_: v: v != [ ] && v != { }) collections
    // extraFrontmatter;

  # name/description first (they are the required pair), rest alphabetical.
  ordered =
    lib.filter (k: present ? ${k}) [
      "name"
      "description"
    ]
    ++ lib.filter (k: k != "name" && k != "description") (lib.attrNames present);

  lines = map (k: "${k}: ${renderValue present.${k}}") ordered;
in
writeTextFile {
  inherit name;
  text = ''
    ---
    ${lib.concatStringsSep "\n" lines}
    ---

    ${content}
  '';
  destination = "/agents/${name}.md";
}
