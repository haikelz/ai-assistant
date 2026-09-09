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

linux_output=$(printf '%s\0%s\0%s\0%s\0%s\0' linux /usr/bin/printf '%s:%s' first second | SSH_ORIGINAL_COMMAND=/usr/local/libexec/picoclaw-vps-exec "$executor")
[ "$linux_output" = 'first:second' ] || {
	echo 'expected Linux argv to be forwarded unchanged' >&2
	exit 1
}

kubernetes_trace=$(printf '%s\0%s\0%s\0' kubernetes version --client | SSH_ORIGINAL_COMMAND=/usr/local/libexec/picoclaw-vps-exec bash -x "$executor" 2>&1 || true)
printf '%s\n' "$kubernetes_trace" | grep -Fq 'KUBECONFIG=/var/lib/picoclaw-ops/.kube/config' || {
	echo 'expected Kubernetes to use the picoclaw-ops kubeconfig' >&2
	exit 1
}

if printf 'unsupported\0' | SSH_ORIGINAL_COMMAND=/usr/local/libexec/picoclaw-vps-exec "$executor" >/dev/null 2>&1; then
	echo 'expected unsupported operation to fail' >&2
	exit 1
fi

if printf '%s\0%s\0' linux /bin/true | SSH_ORIGINAL_COMMAND=malicious "$executor" >/dev/null 2>&1; then
	echo 'expected unexpected SSH command to fail' >&2
	exit 1
fi
