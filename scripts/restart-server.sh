#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

ADDR="${CHAIN_ANALYSIS_ADDR:-:8090}"
PORT="${ADDR##*:}"
if [[ -z "$PORT" || ! "$PORT" =~ ^[0-9]+$ ]]; then
  PORT="8090"
fi

PID_FILE="data/run/server.pid"
LOG_FILE="data/logs/server-runtime.log"
BIN_PATH="data/bin/chain-analysis-server"
HEALTH_URL="http://127.0.0.1:${PORT}/api/health"

ACTION="${1:-restart}"

mkdir -p data/run data/logs data/bin

report_failure() {
  local message="$1"
  echo "${message}"
  echo "recent log output:"
  tail -n 80 "${LOG_FILE}" 2>/dev/null || true
  return 1
}

pid_is_listening_on_port() {
  local pid="$1"
  if ! command -v lsof >/dev/null 2>&1; then
    return 0
  fi
  lsof -tiTCP:${PORT} -sTCP:LISTEN 2>/dev/null | grep -qx "${pid}"
}

stop_server() {
  if [[ -f "$PID_FILE" ]]; then
    OLD_PID="$(cat "$PID_FILE" 2>/dev/null || true)"
    if [[ -n "${OLD_PID}" ]] && kill -0 "${OLD_PID}" 2>/dev/null; then
      kill "${OLD_PID}" || true
      for _ in {1..20}; do
        if ! kill -0 "${OLD_PID}" 2>/dev/null; then
          break
        fi
        sleep 0.1
      done
      if kill -0 "${OLD_PID}" 2>/dev/null; then
        kill -9 "${OLD_PID}" || true
      fi
    fi
    rm -f "$PID_FILE"
  fi

  if command -v lsof >/dev/null 2>&1; then
    LISTEN_PIDS="$(lsof -tiTCP:${PORT} -sTCP:LISTEN 2>/dev/null || true)"
    for PID in ${LISTEN_PIDS}; do
      kill -9 "${PID}" || true
    done
  fi
}

start_with_perl_daemon() {
  if ! command -v perl >/dev/null 2>&1; then
    return 1
  fi

  local abs_bin abs_log daemon_pid_file
  abs_bin="$(cd "$(dirname "${BIN_PATH}")" && pwd)/$(basename "${BIN_PATH}")"
  abs_log="$(cd "$(dirname "${LOG_FILE}")" && pwd)/$(basename "${LOG_FILE}")"
  daemon_pid_file="data/run/server.daemon.pid"
  rm -f "${daemon_pid_file}"

  perl -e '
    use POSIX qw(setsid);
    my ($pid_file, $bin, $log_file, $root) = @ARGV;
    my $pid = fork();
    exit 0 if $pid;
    die "fork1:$!" unless defined $pid;
    setsid() or die "setsid:$!";
    my $pid2 = fork();
    exit 0 if $pid2;
    die "fork2:$!" unless defined $pid2;
    open(my $pf, ">", $pid_file) or die "pidfile:$!";
    print {$pf} $$;
    close($pf);
    chdir($root) or die "chdir:$!";
    open(STDIN, "<", "/dev/null") or die "stdin:$!";
    open(STDOUT, ">>", $log_file) or die "stdout:$!";
    open(STDERR, ">&", \*STDOUT) or die "stderr:$!";
    exec {$bin} $bin or die "exec:$!";
  ' "${daemon_pid_file}" "${abs_bin}" "${abs_log}" "${ROOT_DIR}"

  local pid=""
  for _ in {1..60}; do
    if [[ -f "${daemon_pid_file}" ]]; then
      pid="$(cat "${daemon_pid_file}" 2>/dev/null || true)"
    fi
    if [[ -n "${pid}" && "${pid}" != "-" ]]; then
      rm -f "${daemon_pid_file}"
      echo "${pid}"
      return 0
    fi
    sleep 0.05
  done
  rm -f "${daemon_pid_file}"
  return 1
}

start_server() {
  COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo unknown)"
  BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  VERSION="${CHAIN_ANALYSIS_BUILD_VERSION:-dev}"
  LDFLAGS="-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildTime=${BUILD_TIME}"

  go build -ldflags "${LDFLAGS}" -o "${BIN_PATH}" ./cmd/server

  NEW_PID=""
  if NEW_PID="$(start_with_perl_daemon)"; then
    :
  else
    nohup "${BIN_PATH}" > "${LOG_FILE}" 2>&1 < /dev/null &
    NEW_PID=$!
    disown "${NEW_PID}" 2>/dev/null || true
  fi

  echo "${NEW_PID}" > "${PID_FILE}"

  for _ in {1..60}; do
    if curl -fsS "${HEALTH_URL}" >/dev/null 2>&1; then
      break
    fi
    if ! kill -0 "${NEW_PID}" 2>/dev/null; then
      report_failure "server exited before healthcheck passed (pid=${NEW_PID})"
      return 1
    fi
    sleep 0.25
  done

  if ! curl -fsS "${HEALTH_URL}" >/dev/null 2>&1; then
    report_failure "server failed to become healthy on ${HEALTH_URL}"
    return 1
  fi

  for _ in {1..20}; do
    if ! kill -0 "${NEW_PID}" 2>/dev/null; then
      report_failure "server exited during stability check window (pid=${NEW_PID})"
      return 1
    fi
    if ! pid_is_listening_on_port "${NEW_PID}"; then
      report_failure "server pid=${NEW_PID} is not listening on port ${PORT} during stability check"
      return 1
    fi
    if ! curl -fsS "${HEALTH_URL}" >/dev/null 2>&1; then
      report_failure "server healthcheck failed during stability check window"
      return 1
    fi
    sleep 0.25
  done

  echo "server restarted: pid=${NEW_PID} port=${PORT} stable_for=5s"
  return 0
}

case "${ACTION}" in
  restart)
    stop_server
    start_server
    ;;
  stop)
    stop_server
    echo "server stopped"
    ;;
  *)
    echo "usage: $0 [restart|stop]"
    exit 1
    ;;
esac
