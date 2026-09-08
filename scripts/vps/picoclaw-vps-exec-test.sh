#!/bin/sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
executor=$script_dir/picoclaw-vps-exec

help_output=$(printf 'help\0' | SSH_ORIGINAL_COMMAND=/usr/local/libexec/picoclaw-vps-exec "$executor")
printf '%s\n' "$help_output" | grep -Fq 'Allowed operations:'

if printf 'docker\0' | SSH_ORIGINAL_COMMAND=/usr/local/libexec/picoclaw-vps-exec "$executor" >/dev/null 2>&1; then
	echo 'expected docker without arguments to fail' >&2
	exit 1
fi

if printf 'unsupported\0' | SSH_ORIGINAL_COMMAND=/usr/local/libexec/picoclaw-vps-exec "$executor" >/dev/null 2>&1; then
	echo 'expected unsupported operation to fail' >&2
	exit 1
fi

if printf '%s\0%s\0%s\0' linux-journal ../../etc/passwd 100 | SSH_ORIGINAL_COMMAND=/usr/local/libexec/picoclaw-vps-exec "$executor" >/dev/null 2>&1; then
	echo 'expected unsafe systemd unit to fail' >&2
	exit 1
fi
