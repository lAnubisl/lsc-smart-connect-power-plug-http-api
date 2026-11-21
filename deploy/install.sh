#!/bin/sh
# Runs as root inside a private staging directory on the target server.
set -eu
umask 077
IFS= read -r DEPLOY_HEALTH_URL < health-url
IFS= read -r DEPLOY_SERVICE_USER < service-user
: "${DEPLOY_HEALTH_URL:?Missing health URL}"
: "${DEPLOY_SERVICE_USER:?Missing service account}"
test "$(id -u)" = 0
case "$(uname -m)" in
    armv6l) ;;
    *) echo 'Deployment requires an ARMv6 host' >&2; exit 1 ;;
esac
for command in flock runuser systemctl curl sha256sum; do
    command -v "$command" >/dev/null
done

# Serialize all deployments, including those from other CI runs.
exec 9>/run/lock/plug-api-deploy.lock
flock -w 300 9
sha256sum -c plug-api-linux-armv6.sha256
test -f /usr/local/bin/plug-api
test -f /etc/plug-api/config.json
test -f /etc/systemd/system/plug-api.service
systemctl is-active --quiet plug-api.service

install -d -o 0 -g 0 -m 0700 /var/lib/plug-api-deploy
backup=$(mktemp -d /var/lib/plug-api-deploy/backup.XXXXXXXX)
cp -p /usr/local/bin/plug-api "$backup/plug-api"
cp -p /etc/plug-api/config.json "$backup/config.json"
cp -p /etc/systemd/system/plug-api.service "$backup/plug-api.service"
(cd "$backup" && sha256sum plug-api config.json plug-api.service > SHA256SUMS)
echo "Previous release backed up to $backup (root only)"

# The staging parent is private; install alongside the executable so the service
# account can validate it and rename stays atomic on the same filesystem.
candidate=$(mktemp /usr/local/bin/.plug-api.XXXXXXXX)
changed=0
health() {
    attempt=0
    while [ "$attempt" -lt 30 ]; do
        if systemctl is-active --quiet plug-api.service && \
            curl --fail --silent --output /dev/null --connect-timeout 2 --max-time 3 \
                -- "$DEPLOY_HEALTH_URL"; then
            return 0
        fi
        attempt=$((attempt + 1))
        sleep 2
    done
    return 1
}
finish() {
    result=$?
    trap - EXIT HUP INT TERM
    rm -f "$candidate"
    if [ "$changed" = 1 ]; then
        echo 'Deployment failed; restoring the previous binary' >&2
        # Stop may fail if startup failed; still attempt restoration.
        systemctl stop plug-api.service || true
        restore=$(mktemp /usr/local/bin/.plug-api-rollback.XXXXXXXX)
        if install -o 0 -g 0 -m 0755 "$backup/plug-api" "$restore" && \
            mv -f "$restore" /usr/local/bin/plug-api && \
            systemctl start plug-api.service && health; then
            echo 'Previous binary restored and healthy' >&2
        else
            echo "ROLLBACK FAILED; recover from $backup" >&2
        fi
        rm -f "$restore"
        result=1
    fi
    exit "$result"
}
trap finish EXIT
trap 'exit 1' HUP INT TERM
install -o 0 -g 0 -m 0755 plug-api-linux-armv6 "$candidate"
runuser -u "$DEPLOY_SERVICE_USER" -- "$candidate" -config /etc/plug-api/config.json -check-config

# Set before stop so an interrupted deployment also restores service availability.
changed=1
systemctl stop plug-api.service
mv -f "$candidate" /usr/local/bin/plug-api
systemctl start plug-api.service
health
changed=0
echo 'plug-api deployment is healthy'
