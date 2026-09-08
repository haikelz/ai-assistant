---
name: vps-ops
description: "Operate the VPS through the restricted vps command. Use for Linux status, Docker, and Kubernetes requests."
---

# VPS operations

Use only `/usr/local/bin/vps`; never run `ssh`, `scp`, `sftp`, `docker`, or
`kubectl` directly. The wrapper sends arguments over a forced-command SSH
connection to the VPS.

Read-only Linux operations:

```sh
vps linux status
vps linux processes
vps linux journal <service-name>.service [line-count]
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

Before Docker or Kubernetes commands that change state, restate the exact
command and ask the Telegram user for a clear confirmation in a separate
message. Never request, print, or read private keys, `.env` files, Kubernetes
Secrets, or provider credentials.
