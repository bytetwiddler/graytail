# Debian packaging

`control.template` is used by `make deb` to build a `.deb` package containing
`graytail` and `grayquery`. `__VERSION__` and `__ARCH__` are substituted at
build time from the Makefile's `DEB_VERSION`/`DEB_ARCH` (see `make deb`).

Edit the `Maintainer`, `Section`, and `Description` fields here if you fork
this project and want the package to carry your own identity instead.
