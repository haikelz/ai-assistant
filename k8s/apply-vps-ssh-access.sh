#!/bin/sh
set -eu

: "${VPS_SSH_HOST:?VPS_SSH_HOST must be set}"
: "${VPS_SSH_PORT:=22}"
: "${VPS_SSH_USER:=picoclaw-ops}"
: "${VPS_SSH_CIDR:?VPS_SSH_CIDR must be set to the VPS public IPv4 CIDR, for example 203.0.113.10/32}"
: "${VPS_SSH_KEY_FILE:?VPS_SSH_KEY_FILE must point to the private key file}"
: "${VPS_SSH_KNOWN_HOSTS_FILE:?VPS_SSH_KNOWN_HOSTS_FILE must point to a verified known_hosts file}"

namespace=${KUBERNETES_NAMESPACE:-default}
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

valid_ipv4_cidr() {
	awk -v value="$1" 'BEGIN {
		count = split(value, parts, "/")
		if (count != 2 || parts[2] !~ /^[0-9]+$/ || parts[2] > 32) exit 1
		count = split(parts[1], octets, ".")
		if (count != 4) exit 1
		for (i = 1; i <= 4; i++) if (octets[i] !~ /^[0-9]+$/ || octets[i] > 255) exit 1
	}'
}

case "$VPS_SSH_HOST" in ''|*[!A-Za-z0-9.-]*) echo "invalid VPS_SSH_HOST" >&2; exit 1 ;; esac
case "$VPS_SSH_USER" in ''|*[!A-Za-z0-9_-]*) echo "invalid VPS_SSH_USER" >&2; exit 1 ;; esac
case "$VPS_SSH_PORT" in ''|*[!0-9]*) echo "invalid VPS_SSH_PORT" >&2; exit 1 ;; esac
case "$VPS_SSH_CIDR" in ''|*[!0-9./]*) echo "invalid VPS_SSH_CIDR" >&2; exit 1 ;; esac
[ "$VPS_SSH_PORT" -ge 1 ] 2>/dev/null && [ "$VPS_SSH_PORT" -le 65535 ] 2>/dev/null || { echo "invalid VPS_SSH_PORT" >&2; exit 1; }
valid_ipv4_cidr "$VPS_SSH_CIDR" || { echo "invalid VPS_SSH_CIDR" >&2; exit 1; }
[ -r "$VPS_SSH_KEY_FILE" ] || { echo "cannot read VPS_SSH_KEY_FILE" >&2; exit 1; }
[ -r "$VPS_SSH_KNOWN_HOSTS_FILE" ] || { echo "cannot read VPS_SSH_KNOWN_HOSTS_FILE" >&2; exit 1; }

kubectl create secret generic ai-assistant-vps-ssh \
	--namespace "$namespace" \
	--dry-run=client \
	--output yaml \
	--from-literal=host="$VPS_SSH_HOST" \
	--from-literal=user="$VPS_SSH_USER" \
	--from-literal=port="$VPS_SSH_PORT" \
	--from-file=id_ed25519="$VPS_SSH_KEY_FILE" \
	--from-file=known_hosts="$VPS_SSH_KNOWN_HOSTS_FILE" | kubectl apply -f -

sed \
	-e "s|__VPS_SSH_CIDR__|$VPS_SSH_CIDR|g" \
	-e "s|__VPS_SSH_PORT__|$VPS_SSH_PORT|g" \
	"$script_dir/vps-ssh-network-policy.yaml.tmpl" | kubectl apply -n "$namespace" -f -

echo "Applied VPS SSH Secret and restricted egress policy in namespace $namespace"
