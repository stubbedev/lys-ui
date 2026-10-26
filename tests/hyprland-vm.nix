# The Hyprland side of the compositor-in-the-loop gate (#66).
#
# A NixOS VM that exists for exactly one job: give Hyprland the DRM
# node aquamarine cannot live without. Hyprland's headless backend has
# no allocator of its own - it needs a real DRM card, and on a shared
# machine that card belongs to the developer's live session. Inside
# this VM the card is virtio-gpu: private kernel, private seatd, no
# other master. Hyprland boots against it directly (the same shape a
# real desktop session has), software-rendered via llvmpipe.
#
# The VM shares the module source and the Go module cache read-only
# over 9p, runs the same internal/headlesstest suite the sway gate
# runs (the tests are compositor-agnostic; the driver is picked with
# LYS_UI_TEST_COMPOSITOR), and passes iff the suite passes.
#
# Build: nix build .#checks.<system>.lys-ui-hyprland-vm
{
  pkgs,
  lib,
  src,
  go,
  goModules,
}: let
  # Same pin set the sway recipe uses (internal/headlesstest guards
  # them): the showcase, the states client and the multilist client
  # float at fixed sizes and positions so client-traced widget
  # coordinates are compositor coordinates. The config must verify
  # clean (Hyprland --verify-config): any error raises an error banner
  # that reserves a strip at the top of the output, shifting every
  # window off the coordinates the clients trace - which is what kept
  # this gate red from #66 on. Hyprland 0.56 legacy rule syntax: "windowrule = <field> <value>, ..." where fields are
  # effects (float, size, move) or "match:<prop> <regex>".
  hyprConf = pkgs.writeText "lys-ui-vm-hypr.conf" ''
    monitor=,preferred,auto,1

    animations {
        enabled = 0
    }
    # The harness taps keys with a roundtrip between press and release;
    # on this software-rendered VM one roundtrip (~800ms) outlasts the
    # default 600ms repeat delay, turning every tap into a repeating
    # held key. Taps are taps: repeat starts far past any roundtrip.
    input {
        repeat_delay = 5000
    }
    decoration {
        rounding = 0
        shadow {
            enabled = false
        }
    }
    misc {
        disable_splash_rendering = true
        force_default_wallpaper = 0
        # The activation test checks a granted xdg-activation token by
        # the focus it moves; by default Hyprland only marks the window.
        focus_on_activate = true
    }
    debug {
        disable_logs = false
        enable_stdout_logs = true
    }

    windowrule = float on, match:class ^(dev\\.stubbe\\.lys\\.ui\\.hello)$, size 640 470, move 0 0

    windowrule = float on, match:class ^(dev\\.stubbe\\.lys\\.ui\\.states)$, size 420 280, move 40 40

    windowrule = float on, match:class ^(dev\\.stubbe\\.lys\\.ui\\.multilist)$, size 300 300, move 0 0

    windowrule = float on, match:title ^modal dialog$, move 340 0
  '';

  lysUIUser = "lys-ui";
  uid = 1000;
  runtimeDir = "/run/user/${toString uid}";
  sig = "lys-ui-vm";
in
  pkgs.testers.nixosTest {
    name = "lys-ui-hyprland-vm";

    nodes.machine = {...}: {
      virtualisation.graphics = true;
      virtualisation.cores = 4;
      virtualisation.memorySize = 4096;
      virtualisation.diskSize = 8192;
      # The DRM node: a virtio-gpu card. Plain (no virgl) - Hyprland
      # needs the node for the GBM allocator, llvmpipe does the
      # rendering. -vga none drops the default bochs card so Hyprland
      # sees exactly one DRM device.
      virtualisation.qemu.options = [
        "-vga none"
        "-device virtio-gpu-pci"
      ];
      virtualisation.sharedDirectories = {
        lys-ui-src = {
          source = "${src}";
          target = "/src";
        };
        lys-ui-mods = {
          source = "${goModules}";
          target = "/mods";
        };
      };

      # seatd grants the compositor DRM master without a logind login
      # session; the test drives Hyprland through su, not a seat login.
      services.seatd.enable = true;

      # The GBM allocator loads its DRI helpers from /run/opengl-driver;
      # mesa's set includes the software (swrast/llvmpipe) drivers the
      # virtio node renders with.
      hardware.graphics.enable = true;
      hardware.graphics.extraPackages = [pkgs.mesa];

      users.users.${lysUIUser} = {
        isNormalUser = true;
        uid = lib.mkForce uid;
        extraGroups = [
          "seat"
          "video"
          "render"
          "input"
        ];
      };

      environment.systemPackages = [
        pkgs.hyprland
        go
        # The client-side cursor path's theme (XCURSOR_PATH below).
        pkgs.vanilla-dmz
      ];
    };

    testScript = ''
      start_all()
      machine.wait_for_unit("multi-user.target")

      # The premise everything else stands on: the VM owns a real DRM
      # card Hyprland can take master on.
      machine.succeed("test -e /dev/dri/card0")
      machine.succeed("test -e /dev/dri/renderD128")

      machine.succeed("install -d -m 0700 -o ${lysUIUser} -g users ${runtimeDir}")
      machine.succeed("install -d -m 0755 -o ${lysUIUser} -g users /tmp/lysuitest")

      # The harness kills by config path and reads the pid from the
      # session dir (Env.PIDPath); the recipe keeps both names where
      # the driver expects them, like the sway recipe does.
      machine.succeed(
        "install -m 0644 -o ${lysUIUser} -g users ${hyprConf} ${runtimeDir}/hypr.conf"
      )

      # A config error is not a warning here: Hyprland's error banner
      # reserves a strip of the output and shifts every window off the
      # coordinates the clients trace. Refuse to boot on one.
      verdict = machine.succeed(
        "su - ${lysUIUser} -c 'XDG_RUNTIME_DIR=${runtimeDir} Hyprland --verify-config -c ${runtimeDir}/hypr.conf 2>&1' | tail -n 5"
      )
      if "config ok" not in verdict:
          raise Exception("the Hyprland config does not verify:\n" + verdict)

      hypr_env = (
        "XDG_RUNTIME_DIR=${runtimeDir} HYPRLAND_INSTANCE_SIGNATURE=${sig} "
        "LIBGL_ALWAYS_SOFTWARE=1 "
      )

      machine.succeed(
        "su - ${lysUIUser} -c '"
        + hypr_env
        + "Hyprland -c ${runtimeDir}/hypr.conf >${runtimeDir}/hyprland.log 2>&1 "
        "& echo $! > ${runtimeDir}/hyprland.pid'"
      )

      # The Wayland socket appearing is the boot signal; past it, wait
      # for hyprctl to answer so config errors and backend trouble
      # surface with their logs instead of as a later suite timeout.
      # A boot failure dumps every Hyprland log it left behind: the
      # launcher log, the instance log and any crash report.
      import time

      runtime_dir = "${runtimeDir}"
      for attempt in range(60):
          rc, _ = machine.execute(f"ls {runtime_dir}/wayland-*")
          if rc == 0:
              break
          time.sleep(2)
      else:
          machine.execute("find /tmp/lysuitest /home/lys-ui -name '*.log' -o -name 'hyprland*' 2>/dev/null | head -20 >&2")
          print(machine.execute(f"cat {runtime_dir}/hyprland.log")[1])
          print(machine.execute(f"cat {runtime_dir}/hypr/*/hyprland.log 2>/dev/null | tail -n 60")[1])
          print(machine.execute("cat /home/lys-ui/.cache/hyprland/crash-reports/*.txt 2>/dev/null | head -n 80")[1])
          raise Exception("Hyprland never created its Wayland socket")

      # Hyprland 0.56 auto-generates its instance signature (the env
      # var is advisory), so discover the live instance dir and aim
      # hyprctl and the suite at it.
      hypr_sig = machine.succeed(f"ls {runtime_dir}/hypr").split()[0]
      hyprctl_env = (
        "XDG_RUNTIME_DIR=${runtimeDir} HYPRLAND_INSTANCE_SIGNATURE="
        + hypr_sig
        + " LIBGL_ALWAYS_SOFTWARE=1 "
      )
      print(f"Hyprland instance signature: {hypr_sig}")

      hyprctl_ok = False
      for attempt in range(30):
          rc, out = machine.execute("su - ${lysUIUser} -c '" + hyprctl_env + "hyprctl version'")
          if rc == 0:
              hyprctl_ok = True
              break
          time.sleep(2)
      if not hyprctl_ok:
          print(machine.execute(f"cat {runtime_dir}/hyprland.log")[1])
          print(machine.execute(f"cat {runtime_dir}/hypr/*/hyprland.log 2>/dev/null | tail -n 60")[1])
          machine.execute(f"ls -la {runtime_dir}/hypr/*/ >&2")
          raise Exception("hyprctl never answered")
      machine.execute(
        "su - ${lysUIUser} -c '"
        + hyprctl_env
        + "hyprctl configerrors'"
      )
      print(machine.succeed("su - ${lysUIUser} -c '" + hyprctl_env + "hyprctl monitors'"))
      print(machine.succeed("su - ${lysUIUser} -c '" + hyprctl_env + "hyprctl rules'"))

      wayland_display = machine.succeed(
        "basename $(ls ${runtimeDir}/wayland-* | head -1)"
      ).strip()
      print(f"Hyprland up on {wayland_display}")

      suite_env = (
        "XDG_RUNTIME_DIR=${runtimeDir} WAYLAND_DISPLAY="
        + wayland_display
        + " HYPRLAND_INSTANCE_SIGNATURE="
        + hypr_sig
        + " LYS_TEST_HEADLESS=1 LYS_UI_TEST_COMPOSITOR=hyprland "
        "XCURSOR_PATH=${pkgs.vanilla-dmz}/share/icons XCURSOR_THEME=Vanilla-DMZ "
        "GOFLAGS=-mod=vendor GOPROXY=off GOSUMDB=off "
        "GOCACHE=/tmp/gocache GOPATH=/tmp/gopath CGO_ENABLED=0 "
      )

      # A writable build tree: the source share is read-only, and the
      # vendored modules (goModules) drop in as ./vendor so the suite's
      # own `go build` calls compile offline.
      machine.succeed(
        "cp -r /src /tmp/build && cp -r /mods /tmp/build/vendor"
        " && chown -R ${lysUIUser}:users /tmp/build"
      )

      # -run pattern, kept as a knob while the gate settles on Hyprland;
      # the quotes are literal: the command runs through sh
      suite_run = "'.'"

      try:
          machine.succeed(
            "su - ${lysUIUser} -c 'cd /tmp/build && "
            + suite_env
            + "go test ./internal/headlesstest -count=1 -run "
            + suite_run
            + " >/tmp/lysuitest/suite.log 2>&1'"
          )
      except Exception:
          # The whole suite log: each failing test carries its client's
          # log tail, the evidence a 120-line suite tail cuts off.
          print(machine.execute("cat /tmp/lysuitest/suite.log")[1])
          print(machine.execute(f"tail -n 60 {runtime_dir}/hypr/{hypr_sig}/hyprland.log")[1])
          # Each client's keyboard story: the keymaps it received and the
          # keys it routed - the evidence a key test failure needs.
          print(machine.execute(
            f"for f in {runtime_dir}/client-*.log; do echo \"== $f\"; "
            "grep -E 'keymap|keyboard|popover key|wire key|modifiers|demo: key' $f | head -n 16; done"
          )[1])
          print(machine.execute(f"grep -niE 'session_lock|sessionlock|unlock|refusing' {runtime_dir}/hypr/{hypr_sig}/hyprland.log | head -n 120")[1])
          raise Exception("compositor-in-the-loop suite failed on Hyprland")
      print(machine.succeed("tail -n 200 /tmp/lysuitest/suite.log"))
    '';
  }
