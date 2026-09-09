---
name: vps-ops
description: "Operate the playground VPS through the restricted vps command. Use for Linux, Docker, and Kubernetes requests."
---

# VPS operations

Use only `/usr/local/bin/vps`; never run `ssh`, `scp`, `sftp`, `docker`, or
`kubectl` directly. The wrapper sends arguments over a forced-command SSH
connection to the VPS.

Linux commands are enabled by the owner and forwarded as separate arguments:

```sh
vps linux uname -a
vps linux systemctl status nginx
vps linux bash -lc 'journalctl -u nginx -n 100 | tail -n 20'
```

Docker operations are enabled by the owner. Pass arguments as separate values:

```sh
vps docker ps
vps docker logs --tail 100 <container>
vps docker restart <container>
```

Kubernetes operations are enabled by the owner. Pass arguments as separate
values:

```sh
vps kubectl get pods -A
vps kubectl logs -n <namespace> <pod> --tail=100
vps kubectl rollout restart deployment/<name> -n <namespace>
```

For destructive or ambiguous Linux, Docker, or Kubernetes commands, restate the
exact command and ask the Telegram user for a clear confirmation in a separate
message. Never request, print, or read private keys, `.env` files, Kubernetes
Secrets, or provider credentials.
