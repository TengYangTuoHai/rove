#!/bin/bash
# Capture the native app and Pi during a manually reproduced peek-scroll stall.
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: bash scripts/diagnose-peek-scroll.sh [options]

Run this on the Mac where scrolling stutters, with Rove and the affected Pi
session open. After the countdown, scroll that Pi popup until capture finishes.
The script samples native stacks; it sends no terminal input or UI events and
changes no settings. No Go, Xcode or extra packages are required.

  --app-pid PID     Rove process (auto-detected when exactly one is running)
  --pi-pid PID      Affected Pi process; repeat to sample several (default: all Pi)
  --duration SEC    Capture for 1–30 seconds (default: 8)
  --delay SEC       Countdown before sampling, 0–30 seconds (default: 5)
  --out DIRECTORY  Create a new output directory (default: a temporary directory)
  --help           Show this help

Share the printed output directory with the developer. It contains stack
samples, executable paths/build identity, macOS/hardware/display information,
and sampler logs. It does not capture screenshots or terminal transcripts.
USAGE
}

app_pid=""
pi_pids=()
duration=8
delay=5
out=""
while (($#)); do
  case "$1" in
    --help|-h) usage; exit 0 ;;
    --app-pid|--pi-pid|--duration|--delay|--out)
      if (($# < 2)); then printf 'Missing value for %s\n' "$1" >&2; exit 2; fi
      case "$1" in
        --app-pid) app_pid=$2 ;;
        --pi-pid) pi_pids+=("$2") ;;
        --duration) duration=$2 ;;
        --delay) delay=$2 ;;
        --out) out=$2 ;;
      esac
      shift 2 ;;
    *) printf 'Unknown option: %s\n' "$1" >&2; exit 2 ;;
  esac
done

# Bash 3.2 treats an empty array expansion as unbound under set -u.
for number in "$duration" "$delay" ${pi_pids[@]+"${pi_pids[@]}"} ${app_pid:+"$app_pid"}; do
  case "$number" in ''|*[!0-9]*) printf 'Expected an integer, got: %s\n' "$number" >&2; exit 2 ;; esac
done
duration=$((10#$duration))
delay=$((10#$delay))
if ((duration < 1 || duration > 30 || delay > 30)); then
  printf 'Duration must be 1–30 seconds; delay must be 0–30 seconds.\n' >&2
  exit 2
fi
if [[ $(uname -s) != Darwin ]]; then printf 'Run this script on macOS.\n' >&2; exit 2; fi

processes=$(/bin/ps -U "$(id -u)" -o pid=,comm=)
if [[ -z "$app_pid" ]]; then
  detected_apps=()
  while read -r pid; do [[ -n "$pid" ]] && detected_apps+=("$pid"); done < <(
    printf '%s\n' "$processes" | awk '/\/Rove\.app\/Contents\/MacOS\/Rove$/ || /\/rove$/ {print $1}'
  )
  if ((${#detected_apps[@]} != 1)); then
    printf 'Found %s Rove processes. Specify --app-pid from Activity Monitor.\n' "${#detected_apps[@]}" >&2
    printf '%s\n' "$processes" | awk '/Rove|rove/' >&2
    exit 2
  fi
  app_pid=${detected_apps[0]}
fi
if ((${#pi_pids[@]} == 0)); then
  while read -r pid; do [[ -n "$pid" ]] && pi_pids+=("$pid"); done < <(
    printf '%s\n' "$processes" | awk '$2 == "pi" || $2 ~ /\/pi$/ {print $1}'
  )
fi
if ((${#pi_pids[@]} > 8)); then
  printf 'More than eight Pi processes found; select the affected one with --pi-pid.\n' >&2
  exit 2
fi
labels=(herdr)
targets=("$app_pid")
for pid in ${pi_pids[@]+"${pi_pids[@]}"}; do
  [[ "$pid" == "$app_pid" ]] && continue
  targets+=("$pid")
  labels+=("pi-$pid")
done
for pid in "${targets[@]}"; do
  if ! /bin/ps -p "$pid" -o pid= | awk 'NF {found=1} END {exit !found}'; then
    printf 'Process %s is not running.\n' "$pid" >&2; exit 2
  fi
done

if [[ -z "$out" ]]; then
  out=$(mktemp -d "${TMPDIR:-/tmp}/herdr-peek-scroll.XXXXXX")
else
  mkdir "$out"  # Refuse to overwrite an existing capture.
fi
app_exe=$(/bin/ps -p "$app_pid" -o comm= | awk '{sub(/^[[:space:]]+/, ""); print}')
{
  date -u '+capture_created_utc=%Y-%m-%dT%H:%M:%SZ'
  printf 'duration_seconds=%s\napp_pid=%s\n' "$duration" "$app_pid"
  sw_vers
  /usr/sbin/sysctl hw.model hw.memsize hw.ncpu machdep.cpu.brand_string
  for pid in "${targets[@]}"; do /bin/ps -p "$pid" -o pid=,ppid=,comm=; done
  if [[ -f "$app_exe" ]]; then
    /usr/bin/shasum -a 256 "$app_exe"
    case "$app_exe" in
      */Contents/MacOS/*)
        info_plist="${app_exe%/MacOS/*}/Info.plist"
        /usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$info_plist" || true
        /usr/libexec/PlistBuddy -c 'Print :CFBundleVersion' "$info_plist" || true
        ;;
    esac
  fi
} > "$out/environment.txt" 2>&1
/usr/sbin/system_profiler -timeout 5 -detailLevel mini SPDisplaysDataType > "$out/displays.txt" 2>&1 || true
if ((${#pi_pids[@]} == 0)); then
  printf 'Pi was not auto-detected; collecting Rove only. Use --pi-pid to include it.\n'
fi
printf 'Capturing Rove PID %s and %s Pi process(es).\n' "$app_pid" "${#pi_pids[@]}"
printf 'In %s seconds, scroll the affected popup for %s seconds.\n' "$delay" "$duration"
sleep "$delay"
printf 'Sampling now — reproduce the stutter.\n'
jobs=()
for i in "${!targets[@]}"; do
  /usr/bin/sample "${targets[$i]}" "$duration" 1 -file "$out/${labels[$i]}.sample.txt" > "$out/${labels[$i]}.sampler.log" 2>&1 &
  jobs+=("$!")
done
failed=0
for i in "${!jobs[@]}"; do
  if ! wait "${jobs[$i]}"; then
    printf 'Sampling %s failed; see %s/%s.sampler.log\n' "${labels[$i]}" "$out" "${labels[$i]}" >&2
    failed=1
  fi
done
{
  date -u '+capture_finished_utc=%Y-%m-%dT%H:%M:%SZ'
  /usr/bin/pmset -g
  /usr/bin/pmset -g batt
  /usr/bin/pmset -g therm
} > "$out/power.txt" 2>&1 || true
printf 'Finished. Capture directory: %s\n' "$out"
exit "$failed"
