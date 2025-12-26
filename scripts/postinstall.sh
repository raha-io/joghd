#!/usr/bin/env bash

# https://stackoverflow.com/questions/3822621/how-to-exit-if-a-command-failed
set -eu
set -o pipefail

# Create config directory if it doesn't exist
mkdir -p /etc/joghd

# Copy example config if no config exists
if [ ! -f /etc/joghd/config.toml ]; then
	cp /etc/joghd/config.example.toml /etc/joghd/config.toml
	echo "Created /etc/joghd/config.toml from example"
fi

echo "Joghd installed successfully!"
echo "Edit /etc/joghd/config.toml to configure your health checks"
