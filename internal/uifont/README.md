# internal/uifont

The faces lys-ui's default look is drawn in, embedded in every binary
that imports the toolkit.

## Geist-Variable.ttf, GeistMono-Variable.ttf

Geist and Geist Mono, as variable fonts (wght 100-900).

- Source: https://github.com/vercel/geist-font at
  75d647056bcbd9eaba3a14b1b58290d0692685d6,
  `packages/next/dist/fonts/geist-sans/Geist-Variable.ttf` and
  `packages/next/dist/fonts/geist-mono/GeistMono-Variable.ttf`
- SHA-256:
  - Geist-Variable.ttf
    `73894e0448cae90a92b6c2f8732b7bb9acb7b94c418bff559dad4a18e1de9659`
  - GeistMono-Variable.ttf
    `87c2aff9723544a9adaea19d92e42a33705c9723624801b6e0224c2206a6af0d`
- License: SIL Open Font License 1.1, see OFL.txt (copied from the same
  commit), which must stay beside the fonts. The license permits
  bundling and redistribution inside software.

The files are unmodified. To update them, download the variable fonts
and OFL.txt from a newer commit, update the commit and checksums above,
and review the widget goldens that use the UI face.
