#!/bin/sh
set -eu

duration=${TAILPATH_PERF_DURATION:-10m}
rps=${TAILPATH_PERF_RPS:-125}
api_samples=${TAILPATH_PERF_API_SAMPLES:-50}
output_dir=${TAILPATH_PERF_OUTPUT_DIR:-perf-artifacts}
run_id=${GITHUB_RUN_ID:-local-$$}
server_name="tailpath-perf-server-$run_id"
volume_name="tailpath-perf-data-$run_id"
server_image="tailpath-perf-server:$run_id"
client_image="tailpath-perf-client:$run_id"
go_proxy=${TAILPATH_PERF_GOPROXY:-https://proxy.golang.org,direct}
rss_limit_kib=$((384 * 1024))
monitor_pid=""

case "$run_id" in
  *[!A-Za-z0-9_.-]*)
    echo "perf-gate: unsafe run identifier $run_id" >&2
    exit 1
    ;;
esac

mkdir -p "$output_dir"
: >"$output_dir/rss-kib.tsv"
: >"$output_dir/cgroup-stats.jsonl"

cleanup() {
  if [ -n "$monitor_pid" ]; then
    kill "$monitor_pid" 2>/dev/null || true
    wait "$monitor_pid" 2>/dev/null || true
  fi
  docker container rm --force "$server_name" >/dev/null 2>&1 || true
  docker volume rm "$volume_name" >/dev/null 2>&1 || true
}
trap cleanup EXIT HUP INT TERM

DOCKER_BUILDKIT=1 docker build --target runtime \
  --build-arg VERSION=perf --build-arg "GOPROXY=$go_proxy" -t "$server_image" .
DOCKER_BUILDKIT=1 docker build --target perf-client \
  --build-arg VERSION=perf --build-arg "GOPROXY=$go_proxy" -t "$client_image" .
docker volume create "$volume_name" >/dev/null
docker run --detach \
  --name "$server_name" \
  --cpus 2 \
  --memory 512m \
  --memory-swap 512m \
  --pids-limit 256 \
  --read-only \
  --mount "type=volume,source=$volume_name,target=/var/lib/tailpath" \
  "$server_image" \
  fixture-server \
  --empty \
  --listen=127.0.0.1:8080 \
  --admin-listen=127.0.0.1:8091 \
  --database=/var/lib/tailpath/perf.db \
  --web-dir=/opt/tailpath/web >/dev/null

attempt=0
until docker exec "$server_name" /usr/local/bin/tailpath \
  healthcheck --url=http://127.0.0.1:8091/healthz >/dev/null 2>&1; do
  attempt=$((attempt + 1))
  if [ "$attempt" -ge 60 ]; then
    docker logs "$server_name" >"$output_dir/server.log" 2>&1 || true
    echo "perf-gate: server did not become healthy" >&2
    exit 1
  fi
  sleep 1
done

monitor_resources() {
  while docker inspect --format '{{.State.Running}}' "$server_name" 2>/dev/null | grep -Fx true >/dev/null; do
    pid=$(docker inspect --format '{{.State.Pid}}' "$server_name")
    rss_kib=$(awk '/^VmRSS:/ { print $2 }' "/proc/$pid/status" 2>/dev/null || echo 0)
    printf '%s\t%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$rss_kib" >>"$output_dir/rss-kib.tsv"
    docker stats --no-stream --format '{{json .}}' "$server_name" >>"$output_dir/cgroup-stats.jsonl" 2>/dev/null || true
    sleep 1
  done
}
monitor_resources &
monitor_pid=$!

set +e
docker run --rm \
  --network "container:$server_name" \
  "$client_image" \
  --server=http://127.0.0.1:8080 \
  --duration="$duration" \
  --rps="$rps" \
  --api-samples="$api_samples" \
  >"$output_dir/load.json" \
  2>"$output_dir/load.stderr"
load_status=$?
set -e

kill "$monitor_pid" 2>/dev/null || true
wait "$monitor_pid" 2>/dev/null || true
monitor_pid=""

docker inspect "$server_name" >"$output_dir/container-inspect.json"
docker logs "$server_name" >"$output_dir/server.log" 2>&1 || true
peak_rss_kib=$(awk 'BEGIN { max = 0 } $2 > max { max = $2 } END { print max }' "$output_dir/rss-kib.tsv")

jq -n \
  --arg duration "$duration" \
  --argjson requestedRPS "$rps" \
  --argjson loadExitCode "$load_status" \
  --argjson peakRSSKiB "$peak_rss_kib" \
  --argjson rssLimitKiB "$rss_limit_kib" \
  --slurpfile load "$output_dir/load.json" \
  --slurpfile inspect "$output_dir/container-inspect.json" \
  '{
    version: 1,
    duration: $duration,
    requestedReportsPerSecond: $requestedRPS,
    loadExitCode: $loadExitCode,
    peakRSSKiB: $peakRSSKiB,
    rssLimitKiB: $rssLimitKiB,
    oomKilled: $inspect[0][0].State.OOMKilled,
    restartCount: $inspect[0][0].RestartCount,
    serverRunningAfterLoad: $inspect[0][0].State.Running,
    load: $load[0],
    passed: (
      $loadExitCode == 0 and
      $load[0].passed == true and
      $peakRSSKiB <= $rssLimitKiB and
      $inspect[0][0].State.OOMKilled == false and
      $inspect[0][0].RestartCount == 0 and
      $inspect[0][0].State.Running == true
    )
  }' >"$output_dir/gate.json"

cat "$output_dir/gate.json"
jq -e '.passed == true' "$output_dir/gate.json" >/dev/null
