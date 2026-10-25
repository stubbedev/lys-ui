{ self, ... }: {
  perSystem = { buildGoModule, lib, pkgs, ... }:
    let
      # Shared builder for every lys-ui derivation: pure Go (CGO off),
      # modules fetched through buildGoModule's standard flow with
      # vendorHash pinning go.sum, so no build reaches the network.
      # The test suite lives in CI (`just check` + `just headless`),
      # not in the build sandbox.
      mkLysUI = { pname, subPackages ? [ ], description ? "", mainProgram ? null }:
        buildGoModule {
          inherit pname subPackages;
          version = self.shortRev or "dev";
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
          vendorHash = "sha256-felNnnH8nFJsk0t3RAvfO4ZN5zHyrsr/fWmq0DAUBEE=";
          doCheck = false;
          # third_party/ holds patched upstream packages lys-ui imports;
          # they build as dependencies, not as targets of their own.
          excludedPackages = [ "third_party" ];

          meta = with lib; {
            license = licenses.mit;
            platforms = platforms.linux;
            inherit description;
          } // lib.optionalAttrs (mainProgram != null) { inherit mainProgram; };
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
    in
    {
      # A Wayland toolkit: the builds exist on Linux only, while the dev
      # shell stays available on every system.
      packages = lib.optionalAttrs pkgs.stdenv.hostPlatform.isLinux {
        inherit lys-ui;

        lys-ui-bar = mkLysUI {
          pname = "lys-ui-bar";
          subPackages = [ "cmd/lys-ui-bar" ];
          description = "lys-ui layer-shell status bar";
          mainProgram = "lys-ui-bar";
        };

        lys-ui-panel = mkLysUI {
          pname = "lys-ui-panel";
          subPackages = [ "cmd/lys-ui-panel" ];
          description = "lys-ui layer-shell panel";
          mainProgram = "lys-ui-panel";
        };

        lys-ui-hello = mkLysUI {
          pname = "lys-ui-hello";
          subPackages = [ "cmd/lys-ui-hello" ];
          description = "lys-ui widget showcase";
          mainProgram = "lys-ui-hello";
        };

        default = lys-ui;
      };
    };
}
