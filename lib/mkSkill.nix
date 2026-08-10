{
  writeTextFile,
  lib,
}:
{
  name,
  description,
  # List of tool specs, or a pre-joined string. Entries containing spaces
  # (e.g. "Bash(sem diff:*)") are joined with commas — a space join would
  # shear them mid-entry.
  allowed-tools ? [ ],
  # Any additional SKILL.md frontmatter fields, rendered as `key: value`
  # lines. Claude Code accepts e.g. argument-hint, disable-model-invocation,
  # user-invocable, context (fork), agent, model, effort, paths, when_to_use.
  extraFrontmatter ? { },
}:
content:
let
  toolsValue =
    if builtins.isList allowed-tools then
      lib.concatStringsSep (
        if lib.any (t: lib.hasInfix " " t) allowed-tools then ", " else " "
      ) allowed-tools
    else
      allowed-tools;

  formatValue =
    key: value:
    if builtins.isBool value then "${key}: ${lib.boolToString value}" else "${key}: ${toString value}";

  # allowed-tools is omitted entirely when empty: an empty `allowed-tools:`
  # line would restrict the skill to no tools.
  fields = [
    "name: ${name}"
    "description: ${description}"
  ]
  ++ lib.optional (allowed-tools != [ ] && allowed-tools != "") "allowed-tools: ${toolsValue}"
  ++ lib.mapAttrsToList formatValue extraFrontmatter;
in
writeTextFile {
  inherit name;
  text = ''
    ---
    ${lib.concatStringsSep "\n" fields}
    ---
    ${content}
  '';
  destination = "/skills/${name}/SKILL.md";
}
