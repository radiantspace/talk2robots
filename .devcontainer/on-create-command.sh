#!/bin/bash
if [ -n "${CODESPACES:-}" ]; then
  sudo apt-get install pkg-config libogg-dev libvorbis-dev libopus-dev libopusfile-dev
  cd /workspaces/talk2robots
  make tidy
fi
