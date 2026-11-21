#!/bin/sh
set -eu
# Never enable shell tracing here: the environment contains an SSH private key.
: "${DEPLOY_HOST:?Set DEPLOY_HOST}"
: "${DEPLOY_PORT:?Set DEPLOY_PORT}"
: "${DEPLOY_USER:?Create the deploy_user Woodpecker secret}"
: "${DEPLOY_SERVICE_USER:?Create the deploy_service_user Woodpecker secret}"
: "${DEPLOY_HEALTH_URL:?Create the deploy_health_url Woodpecker secret}"
: "${DEPLOY_SSH_KEY:?Create the deploy_ssh_key Woodpecker secret}"
: "${DEPLOY_KNOWN_HOSTS:?Create the deploy_known_hosts Woodpecker secret}"

umask 077
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
trap 'exit 1' HUP INT TERM
printf '%s\n' "$DEPLOY_SSH_KEY" > "$work/key"
printf '%s\n' "$DEPLOY_KNOWN_HOSTS" > "$work/known_hosts"
unset DEPLOY_SSH_KEY DEPLOY_KNOWN_HOSTS

# Use a file instead of a pipe so tar errors cannot be hidden by SSH's exit code.
mkdir "$work/release"
# Transfer runtime settings as private data files, never interpolated shell code.
printf '%s\n' "$DEPLOY_HEALTH_URL" > "$work/release/health-url"
printf '%s\n' "$DEPLOY_SERVICE_USER" > "$work/release/service-user"
unset DEPLOY_HEALTH_URL DEPLOY_SERVICE_USER
cp bin/plug-api-linux-armv6 bin/plug-api-linux-armv6.sha256 deploy/install.sh "$work/release/"
tar -cf "$work/release.tar" -C "$work/release" \
    plug-api-linux-armv6 plug-api-linux-armv6.sha256 install.sh health-url service-user
ssh -T -F /dev/null -i "$work/key" -p "$DEPLOY_PORT" \
    -o BatchMode=yes -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes \
    -o UserKnownHostsFile="$work/known_hosts" -o GlobalKnownHostsFile=/dev/null \
    -o ConnectTimeout=15 -o ServerAliveInterval=15 -o ServerAliveCountMax=3 \
    "$DEPLOY_USER@$DEPLOY_HOST" "$(cat <<'REMOTE'
    sudo -n sh -c '
        set -eu
        umask 077
        stage=$(mktemp -d /var/tmp/plug-api-deploy.XXXXXXXX)
        trap "rm -rf \"\$stage\"" EXIT
        trap "exit 1" HUP INT TERM
        tar -xf - -C "$stage"
        cd "$stage"
        sh ./install.sh
    '
REMOTE
)" < "$work/release.tar"
