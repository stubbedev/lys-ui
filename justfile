# lys-ui dev tasks.

# List all recipes with their descriptions.
default:
    @just --list

# Directory for headless test sessions: compositor socket, client logs, pids.
test_dir := "/tmp/lys-ui-test-env"

# Directory for the compositor-in-the-loop test suite, separate from
# the interactive demo dir so the two never clobber each other.
headless_test_dir := "/tmp/lys-ui-test-env-24"

# Run every release gate in order: vet, lint, test, build.
check: vet lint test build

# The Hyprland side of the compositor-in-the-loop gate: boots Hyprland
# inside a private NixOS VM (own kernel, own virtio-gpu DRM node, own
# seatd - aquamarine cannot boot without a real DRM node and the
# developer's card belongs to their live session) and runs the same
# suite just headless runs, driven by LYS_UI_TEST_COMPOSITOR=hyprland.
# Needs KVM; a required CI gate alongside sway.
hyprland-vm:
    nix build -L .#checks.{{ arch() }}-{{ os() }}.lys-ui-hyprland-vm

# The AT-SPI bridge's bus-level tests (#65, #114): a private
# dbus-daemon plays the accessibility bus, a second connection plays
# the registry and the assistive technology. No compositor, no
# at-spi2-core needed.
atspi:
    go test ./internal/atspi/

# The GPU renderer's tests on lavapipe, Mesa's CPU Vulkan driver, so
# they gate without a GPU; `go test ./gpu/...` alone runs them on the
# host's device, or skips them without one.
gpu:
    LYS_UI_TEST_GPU=lavapipe go test -count=1 ./gpu/...

# Run the suite under the race detector. It needs cgo, so only this
# recipe enables it, with the dev shell's pinned C compiler
# (LYS_UI_RACE_CC); allocation budgets skip under -race, whose
# instrumentation allocates.
race:
    CC="$LYS_UI_RACE_CC" CGO_ENABLED=1 go test -race ./...

# The bridge against real at-spi2-core (#114) - the scripted
# accerciser pass: at-spi-bus-launcher on a private session bus, the
# real registryd, and a client that finds the application under the
# registry's desktop, reads the Cache, walks to an entry, and moves
# its caret. ATSPI_LIBEXEC (set by the dev shell) points at
# at-spi2-core's libexec.
atspi-verify:
    go test -count=1 -run 'TestRealAtSpi2Core' -v ./internal/atspi/

# Run the compositor-in-the-loop input tests (internal/headlesstest):
# boots a private headless sway with the test-env recipe, then runs
# the suite against it with LYS_UI_HEADLESS=1 - the tests attach to the
# running compositor, launch the showcase, and drive a synthetic seat
# through pointer and keyboard while asserting on the client's trace
# log. The env is always torn down, pass or fail. Runs inside the dev
# shell so sway is on PATH; needs no display and never touches the
# desktop session.
headless:
    #!/bin/sh
    dir="{{headless_test_dir}}"
    just test-env "$dir" || exit 1
    trap 'just test-env-stop "$dir"' EXIT INT TERM
    . "$dir/client.env"
    export WAYLAND_DISPLAY DBUS_SESSION_BUS_ADDRESS XDG_RUNTIME_DIR="$dir" LYS_UI_HEADLESS=1
    # The client-side cursor path needs a theme; the dev shell supplies
    # one (LYS_UI_TEST_XCURSOR_PATH) so the run does not depend on the host.
    if [ -n "$LYS_UI_TEST_XCURSOR_PATH" ]; then
        export XCURSOR_PATH="$LYS_UI_TEST_XCURSOR_PATH" XCURSOR_THEME=Vanilla-DMZ
    fi
    go test ./internal/headlesstest ./capture ./vinput ./app -count=1

# `check` plus the headless input gate - what CI runs. The gate is
# required there: LYS_UI_HEADLESS=1 turns every skip into a failure.
check-headless: check headless

# Static analysis of every package with go vet.
vet:
    go vet ./...

# Lint every package; settings live in .golangci.yml.
lint:
    golangci-lint run

# Run the test suite for every package.
test:
    go test ./...

# Run every benchmark once, tests skipped: Measure/Arrange/Paint over
# the showcase tree. CI archives the numbers as an artifact for trend
# watching - deliberately not a gate, shared runners are too noisy.
bench:
    go test -bench . -run '^$' -benchmem ./...

# Regenerate the golden-image snapshots after a deliberate visual
# change: render and widget goldens are rewritten, so review the diff
# (git diff -- '*testdata/golden*') before committing. See
# internal/golden for the comparison policy.
goldens:
    UPDATE_GOLDEN=1 go test ./render ./widget

# Rasterize the SVG references (render/testdata/svgref) with
# rsvg-convert, the independent renderer render.LoadSVG is held to; the
# PNG's size is the size the test renders at.
svg-refs:
    #!/usr/bin/env bash
    set -euo pipefail
    for svg in render/testdata/svgref/*.svg; do
        rsvg-convert -w 64 -h 64 "$svg" -o "${svg%.svg}.png"
    done

# Compile-check the bar binary; the output is discarded.
build:
    go build -o /dev/null ./cmd/lys-ui-bar

# Format every Go source in place: gofumpt and gci as configured in
# .golangci.yml, which also excludes third_party/ (upstream code carried
# verbatim) and generated files.
fmt:
    golangci-lint fmt ./...

# The formatting gate: fail, showing the diff, when any source is not
# formatted.
fmt-check:
    golangci-lint fmt --diff ./...

# Run the widget showcase (lys-ui-hello): clicks, drag, tooltips, menu, Tab focus.
demo:
    go run ./cmd/lys-ui-hello

# Run the widget showcase inside the headless test compositor with
# input traces on; pair with the move/click/sweep recipes to drive it.
# LYS_UI_DEBUG picks trace categories (see internal/debug).
demo-headless LYS_UI_DEBUG="input,frame": test-env test-build
    #!/bin/sh
    dir="{{test_dir}}"
    if [ ! -f "$dir/client.env" ]; then
        echo "test compositor env missing; run just test-env"; exit 1
    fi
    . "$dir/client.env"
    export WAYLAND_DISPLAY DBUS_SESSION_BUS_ADDRESS XDG_RUNTIME_DIR="$dir" LYS_UI_DEBUG="{{LYS_UI_DEBUG}}"
    "$dir/lys-ui-hello" >"$dir/demo.log" 2>&1 &
    echo $! >"$dir/demo.pid"
    sleep 1
    if kill -0 "$(cat "$dir/demo.pid")" 2>/dev/null; then
        echo "demo running (pid $(cat "$dir/demo.pid")); traces: just test-log"
    else
        echo "demo died:"; cat "$dir/demo.log"; exit 1
    fi

# Run the glass demo: the theme's glass surfaces over a moving
# backdrop; t toggles light and dark, Escape quits.
glass:
    go run ./cmd/lys-ui-glass

# Run the component gallery: every component lys-ui implements, built
# with the ui builders; Ctrl+T toggles light and dark, Escape quits.
gallery:
    go run ./cmd/lys-ui-gallery

# Screenshot every gallery demo, overlay opened, in both
# palettes into dir: the gallery's -shots mode on a private headless
# sway that floats its windows at the origin. The docs site shows them.
docshots dir="docs/public/components":
    #!/bin/sh
    env="{{test_dir}}-shots"
    just test-env "$env" || exit 1
    trap 'just test-env-stop "$env"' EXIT INT TERM
    . "$env/client.env"
    export WAYLAND_DISPLAY DBUS_SESSION_BUS_ADDRESS XDG_RUNTIME_DIR="$env"
    # Scale 2, so the shots stay sharp on high-density screens, on a
    # mode 1280 logical pixels wide: a floating window wider than the
    # output is centered over its edges, cropping the shot.
    swaymsg -s "$(ls "$env"/sway-ipc.*.sock)" output '*' mode 2560x1600 scale 2 >/dev/null || exit 1
    go run ./cmd/lys-ui-gallery -shots "{{dir}}"

# Run the application-model demo (lys-ui-multi): per-output bars,
# on-demand windows, close-request veto, Escape quits.
multi:
    go run ./cmd/lys-ui-multi

# Run the application-model demo inside the headless test compositor.
multi-headless LYS_UI_DEBUG="input,frame": test-env test-build
    #!/bin/sh
    dir="{{test_dir}}"
    if [ ! -f "$dir/client.env" ]; then
        echo "test compositor env missing; run just test-env"; exit 1
    fi
    . "$dir/client.env"
    export WAYLAND_DISPLAY DBUS_SESSION_BUS_ADDRESS XDG_RUNTIME_DIR="$dir" LYS_UI_DEBUG="{{LYS_UI_DEBUG}}"
    go build -tags lysuidebug -o "$dir/lys-ui-multi" ./cmd/lys-ui-multi || exit 1
    "$dir/lys-ui-multi" >"$dir/demo.log" 2>&1 &
    echo $! >"$dir/demo.pid"
    sleep 1
    if kill -0 "$(cat "$dir/demo.pid")" 2>/dev/null; then
        echo "lys-ui-multi running (pid $(cat "$dir/demo.pid")); traces: just test-log"
    else
        echo "lys-ui-multi died:"; cat "$dir/demo.log"; exit 1
    fi

# Build the headless test binaries: the traced showcase and wlpointer.
test-build dir=test_dir:
    go build -tags lysuidebug -o {{dir}}/lys-ui-hello ./cmd/lys-ui-hello
    go build -o {{dir}}/wlpointer ./cmd/wlpointer

# Start the headless test compositor: sway on wlroots' headless backend
# in a private XDG_RUNTIME_DIR, so synthetic-input runs never touch the
# real desktop. Idempotent: a second call reports and exits. The sway
# config pins the showcase (dev.stubbe.lys.ui.hello) to a floating
# 640x470 window at the output's origin and the states client
# (dev.stubbe.lys.ui.states) to a floating 420x280 window - keep those in
# sync with internal/headlesstest (TestSwayRecipePinsTheShowcase and
# the state test guard them).
test-env dir=test_dir:
    #!/bin/sh
    dir="{{dir}}"
    if [ -f "$dir/sway.pid" ] && kill -0 "$(cat "$dir/sway.pid")" 2>/dev/null; then
        echo "test compositor already running in $dir"
        exit 0
    fi
    rm -rf "$dir"
    # A daemonizing compositor can outlive its pid file: sweep anything
    # still bound to this dir's config before booting a fresh sway, or
    # the new socket and the old one race for clients.
    pkill -9 -f "$dir/sway.cfg" 2>/dev/null
    pkill -f "$dir/bus.conf" 2>/dev/null
    mkdir -p "$dir" && chmod 700 "$dir"
    # focus_on_window_activation focus: a granted xdg-activation token
    # moves the focus, which the activation test checks.
    printf '%s\n' 'output * mode 1280x800 scale 1' \
        'default_border none' \
        'default_floating_border none' \
        'input type:keyboard repeat_delay 5000' \
        'focus_on_window_activation focus' \
        'for_window [app_id="dev.stubbe.lys.ui.hello"] floating enable, move position 0 0, resize set 640 470' \
        'for_window [app_id="dev.stubbe.lys.ui.states"] floating enable, move position 40 40, resize set 420 280' \
        'for_window [app_id="dev.stubbe.lys.ui.multilist"] floating enable, move position 0 0, resize set 300 300' \
        'for_window [title="modal dialog"] floating enable, move position 340 0' \
        'for_window [app_id="dev.stubbe.lys.ui.shots"] floating enable, move position 0 0' \
        > "$dir/sway.cfg"
    # The pid file must hold sway itself, not the dev shell wrapper:
    # a wrapper that runs the command through a shell without exec'ing
    # can die with the kill test's SIGKILL while sway lives on holding
    # the display socket - the client never sees an EOF and parks
    # forever. A shell that writes its own pid and then execs sway
    # records the process that owns the socket, whatever the wrapper's
    # fork/exec choice is.
    # A private session bus from a config written here (the
    # dbustest helper's shape): nixpkgs' sway wrapper runs
    # dbus-run-session when no bus is set, which needs the host's
    # /etc/dbus-1/session.conf - absent on CI runners - and the tests'
    # D-Bus clients (single instance, portals) get a bus of their own
    # instead of the developer's desktop one.
    printf '%s\n' '<!DOCTYPE busconfig PUBLIC "-//freedesktop//DTD D-BUS Bus Configuration 1.0//EN" "http://www.freedesktop.org/standards/dbus/1.0/busconfig.dtd">' \
        "<busconfig><type>session</type><listen>unix:path=$dir/bus</listen><auth>EXTERNAL</auth>" \
        '<policy context="default"><allow send_destination="*" eavesdrop="true"/><allow eavesdrop="true"/><allow own="*"/></policy></busconfig>' \
        > "$dir/bus.conf"
    bus=$(dbus-daemon --config-file="$dir/bus.conf" --fork --nopidfile --print-address=1)
    if [ -z "$bus" ]; then
        echo "private session bus failed to start"; exit 1
    fi
    # Inside the dev shell (CI runs `nix develop -c just headless`)
    # sway is already on PATH, so the dev shell is only the fallback
    # for a host shell.
    boot="exec sway -c '$dir/sway.cfg'"
    if ! command -v sway >/dev/null 2>&1; then
        boot="exec nix develop -c sway -c '$dir/sway.cfg'"
    fi
    DBUS_SESSION_BUS_ADDRESS="$bus" XDG_RUNTIME_DIR="$dir" WLR_BACKENDS=headless WLR_LIBINPUT_NO_DEVICES=1 \
        WLR_RENDERER=pixman sh -c "echo \$\$ > '$dir/sway.pid'; $boot" \
        >"$dir/sway.log" 2>&1 &
    sock=""
    for i in $(seq 1 50); do
        sock=$(ls "$dir"/wayland-[0-9]* 2>/dev/null | head -1)
        [ -n "$sock" ] && break
        sleep 0.2
    done
    if [ -z "$sock" ]; then
        echo "sway failed to start; log: $dir/sway.log"
        tail -n 40 "$dir/sway.log"
        exit 1
    fi
    printf 'WAYLAND_DISPLAY=%s\nDBUS_SESSION_BUS_ADDRESS=%s\n' "${sock##*/}" "$bus" >"$dir/client.env"
    echo "test compositor up: $(cat "$dir/client.env") XDG_RUNTIME_DIR=$dir"

# Stop the headless test compositor and wipe its runtime dir.
test-env-stop dir=test_dir:
    #!/bin/sh
    dir="{{dir}}"
    if [ -f "$dir/sway.pid" ]; then
        pid="$(cat "$dir/sway.pid")"
        kill "$pid" 2>/dev/null
        for i in $(seq 1 20); do
            kill -0 "$pid" 2>/dev/null || break
            sleep 0.1
        done
        kill -9 "$pid" 2>/dev/null
    fi
    # The recorded pid can be a parent whose forked child is the real
    # compositor holding the display socket; sweep anything still bound
    # to this dir's config so no survivor outlives the teardown.
    pkill -9 -f "$dir/sway.cfg" 2>/dev/null
    pkill -f "$dir/bus.conf" 2>/dev/null
    pkill -f "$dir/lys-ui-hello" 2>/dev/null
    pkill -f "$dir/lys-ui-multi" 2>/dev/null
    pkill -f "$dir/lys-ui-states" 2>/dev/null
    rm -rf "$dir"
    echo "test compositor stopped"

# Follow the headless demo's trace log.
test-log:
    tail -n 40 -f {{test_dir}}/demo.log

# Move the virtual pointer to compositor coordinates in the test env.
move x="220" y="120": test-env test-build
    env XDG_RUNTIME_DIR={{test_dir}} $(cat {{test_dir}}/client.env) {{test_dir}}/wlpointer move {{x}} {{y}}

# Click the virtual pointer at compositor coordinates (optional button code).
click x="220" y="120" button="272": test-env test-build
    env XDG_RUNTIME_DIR={{test_dir}} $(cat {{test_dir}}/client.env) {{test_dir}}/wlpointer click {{x}} {{y}} {{button}}

# Rest the pointer on compositor coordinates for a duration, so
# dwell-dependent behavior (tooltip delay) can trigger.
dwell x="150" y="80" ms="900": test-env test-build
    env XDG_RUNTIME_DIR={{test_dir}} $(cat {{test_dir}}/client.env) {{test_dir}}/wlpointer dwell {{x}} {{y}} {{ms}}

# Send a vertical wheel tick at compositor coordinates (positive down).
axis x="450" y="350" dy="60": test-env test-build
    env XDG_RUNTIME_DIR={{test_dir}} $(cat {{test_dir}}/client.env) {{test_dir}}/wlpointer axis {{x}} {{y}} {{dy}}

# Sweep the pointer along a row (y0 == y1) or column through the test env.
sweep x0="100" y0="100" x1="600" y1="100" step="40": test-env test-build
    env XDG_RUNTIME_DIR={{test_dir}} $(cat {{test_dir}}/client.env) {{test_dir}}/wlpointer sweep {{x0}} {{y0}} {{x1}} {{y1}} {{step}}

# Run the layer-shell panel (lys-ui-panel): right-anchored widgets with live input.
panel:
    go run ./cmd/lys-ui-panel

# Run the layer-shell status bar (lys-ui-bar).
bar:
    go run ./cmd/lys-ui-bar
