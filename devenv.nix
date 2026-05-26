{ pkgs, ... }:
{
  languages.go.enable = true;

  packages = with pkgs; [
    gopls
    gotools
    go-tools
    git
  ];
}
