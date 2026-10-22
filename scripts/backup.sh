#!/bin/sh
set -eu

log() {
	printf 'mazima-backup: %s\n' "$*" >&2
}

: "${RESTIC_REPOSITORY:?RESTIC_REPOSITORY is required}"
: "${RESTIC_PASSWORD_FILE:?RESTIC_PASSWORD_FILE is required}"
: "${MAZIMA_DATA_DIR:=/var/lib/mazima}"
: "${MAZIMA_APPS_DIR:=/data/apps}"

command -v restic >/dev/null 2>&1 || {
	log "restic is not installed"
	exit 1
}

if [ ! -r "$RESTIC_PASSWORD_FILE" ]; then
	log "password file $RESTIC_PASSWORD_FILE is not readable"
	exit 1
fi

mode=$(stat -c %a "$RESTIC_PASSWORD_FILE" 2>/dev/null || stat -f %Lp "$RESTIC_PASSWORD_FILE")
case "$mode" in
400 | 600) ;;
*)
	log "password file $RESTIC_PASSWORD_FILE must be mode 400 or 600 (found $mode)"
	exit 1
	;;
esac

if command -v flock >/dev/null 2>&1; then
	exec 9>/run/mazima-backup.lock
	if ! flock -n 9; then
		log "another backup is already running"
		exit 0
	fi
fi

if ! restic cat config >/dev/null 2>&1; then
	if [ "${RESTIC_INIT:-0}" = "1" ]; then
		log "initializing repository $RESTIC_REPOSITORY"
		restic init
	else
		log "repository is not initialized or is unreachable; set RESTIC_INIT=1 to initialize"
		exit 1
	fi
fi

log "starting backup of $MAZIMA_DATA_DIR and $MAZIMA_APPS_DIR"
set -- backup \
	--tag mazima \
	--host "$(hostname)" \
	--exclude-caches \
	--exclude "$MAZIMA_DATA_DIR/backups" \
	--exclude "$MAZIMA_DATA_DIR/releases" \
	--exclude "$MAZIMA_DATA_DIR/sessions" \
	--exclude "$MAZIMA_DATA_DIR/updates" \
	--exclude "$MAZIMA_DATA_DIR/setup.token" \
	"$MAZIMA_DATA_DIR"
if [ -d "$MAZIMA_APPS_DIR" ]; then
	set -- "$@" "$MAZIMA_APPS_DIR"
fi
restic "$@"

log "applying retention policy"
restic forget \
	--tag mazima \
	--host "$(hostname)" \
	--keep-daily 7 \
	--keep-weekly 4 \
	--keep-monthly 6 \
	--prune

if [ "${RESTIC_CHECK:-0}" = "1" ]; then
	log "checking repository"
	restic check
fi

log "backup complete"
