{
  lib,
  buildGoModule,
}:
buildGoModule {
  pname = "claude-statusline";
  version = "0.1.0";

  src = lib.cleanSource ./.;

  vendorHash = null; # using vendored deps

  subPackages = [ "cmd/claude-statusline" ];

  ldflags = [
    "-s"
    "-w"
  ];

  meta = with lib; {
    description = "Custom statusline for Claude Code, shipped via claude-nix";
    license = licenses.mit;
    mainProgram = "claude-statusline";
    platforms = platforms.unix;
  };
}
