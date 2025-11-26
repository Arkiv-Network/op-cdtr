{ pkgs, perSystem }:
perSystem.devshell.mkShell {
  packages = [
    # go
    pkgs.go
    pkgs.golangci-lint
    pkgs.goreleaser
    pkgs.syft
    pkgs.gcc # necessary on ci for tests

    # other
    perSystem.self.formatter
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
}
