#!/bin/sh
# Open the herdr-top popup (plugin-owned). Geometry comes from herdr-plugin.toml.
set -eu
exec "${HERDR_BIN_PATH:-herdr}" plugin pane open --plugin herdr-top --entrypoint popup --focus
