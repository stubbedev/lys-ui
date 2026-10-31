{
  perSystem = {
    go,
    lib,
    pkgs,
    self',
    ...
  }: {
    checks = lib.optionalAttrs pkgs.stdenv.hostPlatform.isLinux {
      inherit (self'.packages) lys-ui;
      # The Hyprland side of the compositor-in-the-loop gate (#66):
      # a private NixOS VM (own kernel, own virtio-gpu DRM node,
      # own seatd) that boots Hyprland and runs the same suite the
      # sway gate runs. Needs KVM; see tests/hyprland-vm.nix.
      lys-ui-hyprland-vm = pkgs.callPackage ../tests/hyprland-vm.nix {
        inherit go;
        inherit (self'.packages.lys-ui) src goModules;
      };
    };
  };
}
