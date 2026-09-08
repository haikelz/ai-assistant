#!/usr/bin/env bash
set -euo pipefail

: "${PICOCLAW_OPS_PUBLIC_KEY_FILE:?PICOCLAW_OPS_PUBLIC_KEY_FILE must point to the bot public key}"
: "${PICOCLAW_KUBECONFIG_FILE:=}"

if [[ $EUID -ne 0 ]]; then
	echo 'run as root on the VPS' >&2
	exit 1
fi

[[ -r "$PICOCLAW_OPS_PUBLIC_KEY_FILE" ]] || { echo 'cannot read PICOCLAW_OPS_PUBLIC_KEY_FILE' >&2; exit 1; }
public_key=$(<"$PICOCLAW_OPS_PUBLIC_KEY_FILE")
[[ "$public_key" == ssh-ed25519\ * || "$public_key" == ecdsa-sha2-*\ * || "$public_key" == ssh-rsa\ * ]] || { echo 'unsupported SSH public key format' >&2; exit 1; }

if ! id picoclaw-ops >/dev/null 2>&1; then
	useradd --create-home --home-dir /var/lib/picoclaw-ops --shell /bin/bash picoclaw-ops
fi

install -d -o picoclaw-ops -g picoclaw-ops -m 0700 /var/lib/picoclaw-ops/.ssh
authorized_keys=/var/lib/picoclaw-ops/.ssh/authorized_keys
touch "$authorized_keys"
chown picoclaw-ops:picoclaw-ops "$authorized_keys"
chmod 0600 "$authorized_keys"
forced_key="restrict,command=\"/usr/local/libexec/picoclaw-vps-exec\" $public_key"
grep -qxF "$forced_key" "$authorized_keys" || printf '%s\n' "$forced_key" >> "$authorized_keys"

install -D -o root -g root -m 0755 "$(dirname "$0")/../scripts/vps/picoclaw-vps-exec" /usr/local/libexec/picoclaw-vps-exec

if getent group docker >/dev/null; then
	usermod -aG docker picoclaw-ops
else
	echo 'docker group does not exist; Docker commands will remain unavailable' >&2
fi

if [[ -n "$PICOCLAW_KUBECONFIG_FILE" ]]; then
	[[ -r "$PICOCLAW_KUBECONFIG_FILE" ]] || { echo 'cannot read PICOCLAW_KUBECONFIG_FILE' >&2; exit 1; }
	install -D -o picoclaw-ops -g picoclaw-ops -m 0600 "$PICOCLAW_KUBECONFIG_FILE" /var/lib/picoclaw-ops/.kube/config
fi

echo 'Provisioned picoclaw-ops. This account can run all Docker commands and whatever Kubernetes permissions its kubeconfig grants.'
