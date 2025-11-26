{ pkgs, perSystem }:
perSystem.devshell.mkShell {
  packages = [
    # go
    pkgs.air
    pkgs.go
    pkgs.goreleaser
    pkgs.golangci-lint

    # other
    perSystem.self.formatter
    pkgs.just
  ];

  env = [
    {
      name = "NIX_PATH";
      value = "nixpkgs=${toString pkgs.path}";
    }
    {
      name = "NIX_DIR";
      eval = "$PRJ_ROOT/nix";
    }
  ];

  commands = [
  ];
}
