# Resolves the `agent-statusline` flake input for consumers that import this
# repo's modules by path rather than through its flake outputs — the case that
# matters, since `modules/home-manager.nix` is imported directly (e.g.
# `imports = [ "${claude-nix}/modules/home-manager.nix" ]`) and a plain
# home-manager module has no route to a flake input.
#
# `flake.nix` stays the single source of truth for the reference; this reads
# the *locked* node out of `flake.lock`, so the rev and narHash are exactly
# the ones `nix flake lock` pinned and evaluation stays pure.
#
# Threading our own `pkgs` through means agent-statusline's nixpkgs is never
# instantiated: both the schema and the binary build against the caller's
# nixpkgs, which is what `inputs.nixpkgs.follows = "nixpkgs"` asks for anyway.
{ pkgs }:
let
  inherit (pkgs) lib;

  lock = builtins.fromJSON (builtins.readFile ../flake.lock);
  node = lock.nodes.${lock.nodes.${lock.root}.inputs.agent-statusline};
  src = builtins.fetchTree node.locked;

  shared = import "${src}/lib" { inherit pkgs; };
  package = pkgs.callPackage "${src}/package.nix" { };
in
shared
// {
  inherit package src;

  # The `programs.claude-nix.statusLine` option: the shared schema plus
  # claude-nix's answers to the two things that schema deliberately leaves to
  # its consumers. Defined here rather than inline in the module so
  # `checks.eval-statusline` can evaluate the real thing.
  statusLineOption = lib.mkOption {
    description = ''
      Statusline rendered under Claude Code, via agent-statusline. The
      options come from that repo's shared schema, so every harness
      configures one binary through one set of names.
    '';
    default = { };
    type = lib.types.submodule {
      options = shared.statuslineOptions;
      config = {
        # The shared schema leaves `package` without a default so each
        # consumer supplies its own; ours builds from the locked input
        # against this evaluation's nixpkgs.
        package = lib.mkDefault package;

        # The shared default is 10, tracking Go's `config.Defaults()`.
        # claude-nix has always drawn the narrower bar, and moving the shared
        # default would break agent-statusline's Nix/Go drift check — so pin
        # it here. mkDefault, not mkOptionDefault: the latter is the very
        # priority the module system gives an option's own `default`, so 8
        # and 10 would collide rather than override.
        barWidth = lib.mkDefault 8;
      };
    };
  };
}
