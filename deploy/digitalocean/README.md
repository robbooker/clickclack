# DigitalOcean pilot

This deployment runs the ClickClack application on localhost behind Nginx.
SQLite and uploads persist under `/var/lib/clickclack`; verified backups are
written under `/var/backups/clickclack`.

The production host is `clickclack-longboard` and the public origin is
`https://chat.longboardai.com`. Keep the checked-out source pinned to a tested
commit and take a backup before updating it.

## Host

- Droplet: `clickclack-longboard` (`159.223.157.158`, Ubuntu 24.04)
- SSH: `ssh -i ~/.ssh/longboard-bot root@159.223.157.158`
- Source: `/opt/clickclack-src`
- Compose project: `/opt/clickclack-src/deploy/digitalocean`
- Persistent data: `/var/lib/clickclack`
- On-host backups: `/var/backups/clickclack`
- Nginx site: `/etc/nginx/sites-enabled/clickclack`

The application container only publishes port 8080 on loopback. UFW exposes
SSH and Nginx (ports 80 and 443); Nginx terminates TLS and proxies HTTP and
WebSocket traffic.

## Deploy or update

The `.env` file on the host is root-only and contains the GitHub OAuth
credentials. Do not commit it or print it in command output.

```sh
ssh -i ~/.ssh/longboard-bot root@159.223.157.158
cd /opt/clickclack-src
/usr/local/sbin/clickclack-backup
git fetch origin main
git checkout <tested-commit>
cd deploy/digitalocean
sed -i "s/^CLICKCLACK_WEB_VERSION=.*/CLICKCLACK_WEB_VERSION=$(git -C ../.. rev-parse --short=12 HEAD)/" .env
docker compose build app
docker compose up -d app
curl --fail --silent https://chat.longboardai.com/readyz
docker inspect --format '{{.State.Health.Status}} restarts={{.RestartCount}}' clickclack
```

The pilot began on commit `7012841bf8eab6650018912b79bb81e7b41f6ca5`.

## Backups and restore

`clickclack-backup.timer` runs daily around 07:15 UTC. Each backup is created
with ClickClack's online backup command, checked with SQLite's
`PRAGMA integrity_check`, compressed, and retained for 14 days.

```sh
systemctl list-timers clickclack-backup.timer
/usr/local/sbin/clickclack-backup
ls -lh /var/backups/clickclack
```

To restore, stop the app, preserve the current database, decompress the chosen
backup into place, then start and verify the app:

```sh
cd /opt/clickclack-src/deploy/digitalocean
docker compose stop app
cp -a /var/lib/clickclack/clickclack.db /var/lib/clickclack/clickclack.db.before-restore
rm -f /var/lib/clickclack/clickclack.db-wal /var/lib/clickclack/clickclack.db-shm
gzip -cd /var/backups/clickclack/<backup>.db.gz > /var/lib/clickclack/clickclack.db
chown root:root /var/lib/clickclack/clickclack.db
chmod 0640 /var/lib/clickclack/clickclack.db
docker compose start app
curl --fail --silent https://chat.longboardai.com/readyz
```

These backups live on the Droplet. Enable DigitalOcean automated backups or
copy them to object storage before treating the pilot as durable production.

## Authentication and onboarding

Rob Booker's bootstrapped owner has both the local and GitHub identities and
owns the private `Longboard` workspace. With no GitHub organization gate,
other GitHub logins enter ClickClack's isolated `Guests` workspace and cannot
see the private workspace. Adding a pilot member currently requires an admin
user/membership step; ClickClack does not yet expose a general invite-consume
endpoint.

The sign-in screen also accepts a one-time code. Create an owner code on the
host, paste the printed `mgt_...` value into the browser within 15 minutes, and
the resulting browser session remains valid for 30 days:

```sh
docker exec clickclack clickclack admin magic-link create \
  --data /app/data \
  --email madspreadsheets@gmail.com \
  --name "Rob Booker"
```

The code is a short-lived secret. Do not place it in shell history, logs, chat,
or source control.

OpenClaw is not required for human chat. Add Buddy later as a scoped bot token
after the human workflow is accepted.

## Checks

```sh
curl --fail https://chat.longboardai.com/healthz
curl --fail https://chat.longboardai.com/readyz
docker inspect --format '{{.State.Status}} {{.State.Health.Status}} {{.RestartCount}}' clickclack
nginx -t
ufw status
systemctl is-active clickclack-backup.timer certbot.timer unattended-upgrades.service
```
