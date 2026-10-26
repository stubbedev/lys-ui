{
  perSystem = {pkgs, ...}: let
    # One Go toolchain for every package build, the dev shell and the
    # Hyprland VM check. go.mod declares `go 1.27.3` and buildGoModule
    # compiles with GOTOOLCHAIN=local, so the version is load-bearing:
    # nixpkgs' default `go` (1.26) cannot compile this module, so the pin
    # is go_1_27 - no toolchain is ever downloaded at build or dev time.
    go = pkgs.go_1_27;
  in {
    _module.args = {
      inherit go;
      buildGoModule = pkgs.buildGoModule.override {inherit go;};
      testFontsConf = pkgs.makeFontsConf {
        fontDirectories = [pkgs.dejavu_fonts pkgs.noto-fonts-color-emoji];
      };
    };
  };
}
