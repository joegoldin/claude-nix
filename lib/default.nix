{ pkgs }:
{
  mkSkill = pkgs.callPackage ./mkSkill.nix { };

  mkCommand = pkgs.callPackage ./mkCommand.nix { };

  mkAgent = pkgs.callPackage ./mkAgent.nix { };

  mkPlugin = pkgs.callPackage ./mkPlugin.nix { };

  mkClaude = pkgs.callPackage ./mkClaude.nix { };

  mkClaudeConfig = pkgs.callPackage ./mkClaudeConfig.nix { };

  # Pure settings.json merge (curried: takes an args attrset). Not via
  # callPackage — callPackage can't make a curried function overridable.
  mergeClaudeSettings = import ./mergeClaudeSettings.nix { inherit (pkgs) lib; };
}
