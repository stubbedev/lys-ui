{
  description = "lys-ui - pure-Go Wayland widget toolkit";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-parts = {
      url = "github:hercules-ci/flake-parts";
      inputs.nixpkgs-lib.follows = "nixpkgs";
    };
    # lys-lib's GPU driver environment for the dev shell (Go code comes
    # from go.mod, not from here).
    lys-lib = {
      url = "github:stubbedev/lys-lib";
      inputs.nixpkgs.follows = "nixpkgs";
      inputs.flake-parts.follows = "flake-parts";
    };
  };

  outputs = inputs @ {flake-parts, ...}:
    flake-parts.lib.mkFlake {inherit inputs;} {
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "aarch64-darwin"
      ];
      imports = [
        ./nix/checks.nix
        ./nix/package.nix
        ./nix/shell.nix
        ./nix/toolchain.nix
      ];
    };
}
