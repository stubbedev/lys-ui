---
layout: home

hero:
  name: lys-ui
  text: Wayland apps in pure Go
  tagline: A component framework with typed messages, GTK4- and libadwaita-class widgets, a modern component set, a GTK-flavoured CSS engine, and an event loop that idles at 0% CPU. No cgo.
  image:
    src: /lys-ui.svg
    alt: lys-ui
  actions:
    - theme: brand
      text: Get started
      link: /guide/getting-started
    - theme: alt
      text: Widgets
      link: /widgets
    - theme: alt
      text: GitHub
      link: https://github.com/stubbedev/lys-ui

features:
  - title: Components
    details: A relm4-style framework. Each component is a model with typed input and output messages, built with typed view builders.
  - title: Modern components
    details: Accordions to sidebars, data tables to date pickers, in a dark and a light palette, restyled by an application stylesheet.
  - title: The desktop
    details: xdg-shell and layer-shell windows, session lock, portals, clipboard, drag and drop, screen capture and AT-SPI accessibility.
  - title: Tested on real compositors
    details: The compositor-in-the-loop suite runs on a private headless sway and on Hyprland in a NixOS VM, and both are required CI gates.
---
