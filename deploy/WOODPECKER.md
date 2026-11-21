# Woodpecker deployment

`.woodpecker.yaml` builds on pushes, pull requests, and manual runs.
Only a successful **push to `main`**, or a **manual run on `main`**, deploys.
The binary is built with `CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=6`.

## Required repository secrets

All deployment addresses, ports, and account names are supplied through
Woodpecker secrets. There are no fallback values in the pipeline or installer.

| Secret | Value |
| --- | --- |
| `deploy_host` | Target SSH hostname or IP address. |
| `deploy_port` | Target SSH port. |
| `deploy_user` | SSH login account authorized to deploy. |
| `deploy_service_user` | Existing system account from the service unit's `User=` setting; used to validate configuration. |
| `deploy_health_url` | Full HTTP or HTTPS health URL, including the listener port when required and `/health`. It must be reachable **from the target server**. |
| `deploy_ssh_key` | Full multiline OpenSSH private key authorized for the SSH account. Use a passphrase-free dedicated CI key; this pipeline has no interactive key agent. |
| `deploy_known_hosts` | Verified OpenSSH known-hosts line(s) for the target SSH endpoint, including its public host key. Match the hostname and port in the corresponding secrets. |

Enable all secrets for **`push` and `manual`** events only. Leave plugin/image
restrictions empty: deployment uses a commands step, not a Woodpecker plugin.
Secrets are injected only into the deployment step. Protect `main` and allow
only trusted contributors to push/run pipelines: repository writers can change
pipeline code and use the deployment credential.

The health URL and service account are transferred as private data files in the
release archive. They are read as data on the target server, without putting
secret values into remote shell commands. SSH keys remain on the CI agent.

## Woodpecker and server prerequisites

Activate this repository in Woodpecker with configuration path
`.woodpecker.yaml`. Use a Linux container agent that can reach the secret-defined
SSH endpoint and download Go/Alpine images, Go modules, and Alpine packages.
The agent does not need ARM hardware. No privileged mode, Docker socket, host
volume, registry credentials, or trusted-repository permissions are required.

Configure repository pipeline concurrency to **1** to keep releases in order.
The target also uses `flock` to prevent overlapping installations. Cancel
obsolete queued runs rather than replaying an old commit after a newer release.

The existing ARMv6 installation must already be active, with:

- `/usr/local/bin/plug-api`, owned by UID/GID zero and executable;
- `/etc/plug-api/config.json`, readable by the secret-defined service account;
- `/etc/systemd/system/plug-api.service`, configured for that service account;
- `sh`, `tar`, `install`, `mktemp`, `sha256sum`, `flock`, `runuser`, `systemctl`,
  and `curl` (Debian packages include `coreutils`, `util-linux`, and `curl`).

Authorize the CI public key in the SSH login account's `~/.ssh/authorized_keys`.
Obtain the SSH host-key fingerprint through a trusted session or server console:

```sh
sudo ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub
```

From a trusted machine, use `ssh-keyscan -p "$DEPLOY_PORT" "$DEPLOY_HOST"`
to collect a candidate entry. Compare its fingerprint with the trusted one
before saving it in `deploy_known_hosts`. The pipeline never accepts host keys
automatically. The known-hosts host field must include `[HOST]:PORT` when using
a nonstandard SSH port.

The SSH account must be allowed to run **`sudo -n sh -c ...` without a password
or TTY**. The pipeline streams the archive into this privileged shell and runs
the installation script. This grants the CI key root deployment authority;
a sudoers rule allowing arbitrary `sh` is not a restricted deployment permission.
The pipeline does not install sudoers rules or change SSH settings.

## Installation and recovery

Deployment verifies the binary checksum, then creates a root-only backup of the
installed binary, config, and unit plus SHA256 checksums under
`/var/lib/plug-api-deploy/backup.*`. It validates the candidate against the
existing config as the secret-defined service account, stops the service, and
atomically replaces the executable with UID/GID zero ownership and mode `0755`.
The existing configuration and systemd unit stay in place. Device Local Keys
and API bearer tokens do not need to be copied into Woodpecker.

There is a short API interruption during restart; active requests can take up
to the unit's shutdown timeout. After starting, deployment checks systemd status
and retries the secret-defined health URL up to 30 times. If replacement,
startup, or health fails, it restores the previous binary and checks it again,
while keeping the CI run failed. Invalid config or checksum leaves the installed
binary untouched. Health checks do not contact physical devices.

Backups contain secrets and remain on the target server, with no automatic
retention cleanup. Review disk usage and remove old backups after confirming a
release. A host crash or SIGKILL can prevent trap-based rollback; use a retained
backup for manual recovery. Substitute the backup directory printed by CI:

```sh
sudo systemctl stop plug-api.service
sudo install -o 0 -g 0 -m 0755 /var/lib/plug-api-deploy/backup.XXXXXXXX/plug-api /usr/local/bin/plug-api
sudo systemctl start plug-api.service
sudo systemctl is-active plug-api.service
curl --fail -- "$DEPLOY_HEALTH_URL"
```

For manual checks, set `DEPLOY_HEALTH_URL` privately in your shell. The config
and unit backups are for operator recovery; binary rollback does not overwrite
them. This pipeline does not perform config or unit migrations.

References: [workflow conditions](https://woodpecker-ci.org/docs/usage/workflow-syntax),
[secrets and event filters](https://woodpecker-ci.org/docs/usage/secrets).
