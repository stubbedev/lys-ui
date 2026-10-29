{self, ...}: {
  perSystem = {
    buildGoModule,
    lib,
    pkgs,
    testFontsConf,
    ...
  }: let
    # Shared builder for every lys-ui derivation: pure Go (CGO off),
    # modules fetched through buildGoModule's standard flow with
    # vendorHash pinning go.sum, so no build reaches the network.
    mkLysUI = {
      pname,
      subPackages ? [],
      description ? "",
      mainProgram ? null,
    }:
      buildGoModule {
        inherit pname subPackages;
        version = "0-unstable-${self.lastModifiedDate or "0"}";
        src = lib.fileset.toSource {
          root = ../.;
          fileset = lib.fileset.difference (lib.fileset.gitTracked ../.) (
            lib.fileset.unions [
              ../flake.nix
              ../flake.lock
              ../nix
            ]
          );
        };
        env.CGO_ENABLED = "0";
        vendorHash = "sha256-bgQrfLgyhAwGViIye27gzYgqz1peozRg9IO2YBsA6q8=";
        doCheck = true;
        preCheck = ''
          export HOME="$TMPDIR" FONTCONFIG_FILE=${testFontsConf}
        '';
        # The GPU renderer loads the Vulkan loader at run time (no
        # cgo, so no rpath): the binaries find it through the
        # wrapper's library path, NixOS having none of its own.
        nativeBuildInputs = [pkgs.makeWrapper];
        nativeCheckInputs = [pkgs.dbus];
        postFixup = ''
          for bin in $out/bin/*; do
            wrapProgram "$bin" --prefix LD_LIBRARY_PATH : ${lib.makeLibraryPath [pkgs.vulkan-loader]}
          done
        '';

        meta = with lib;
          {
            homepage = "https://github.com/stubbedev/lys-ui";
            license = licenses.mit;
            platforms = platforms.linux;
            inherit description;
          }
          // lib.optionalAttrs (mainProgram != null) {inherit mainProgram;};
      };

    # The module itself: one build of every package in the repo -
    # all demo binaries plus the wlpointer test driver. Downstream
    # nix Go builds can also reach lys-ui.goModules for the vendored
    # dependency tree; plain Go consumers pin this repo by semver
    # tag in go.mod (see README, Packaging).
    lys-ui = mkLysUI {
      pname = "lys-ui";
      description = "pure-Go Wayland widget toolkit (module and demos)";
    };
  in {
    # A Wayland toolkit: the builds exist on Linux only, while the dev
    # shell stays available on every system.
    packages = lib.optionalAttrs pkgs.stdenv.hostPlatform.isLinux {
      inherit lys-ui;

      lys-ui-bar = mkLysUI {
        pname = "lys-ui-bar";
        subPackages = ["cmd/lys-ui-bar"];
        description = "lys-ui layer-shell status bar";
        mainProgram = "lys-ui-bar";
      };

      lys-ui-panel = mkLysUI {
        pname = "lys-ui-panel";
        subPackages = ["cmd/lys-ui-panel"];
        description = "lys-ui layer-shell panel";
        mainProgram = "lys-ui-panel";
      };

      lys-ui-hello = mkLysUI {
        pname = "lys-ui-hello";
        subPackages = ["cmd/lys-ui-hello"];
        description = "lys-ui widget showcase";
        mainProgram = "lys-ui-hello";
      };

      default = lys-ui;
    };
  };
}
