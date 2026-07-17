#!/usr/bin/env bash
set -Eeuo pipefail

ENV_FILE="${HF_ENV_FILE:-/etc/hostforge/hostforge.env}"
DEFAULT_MIN_FREE_BYTES=5368709120
MANAGED_LABEL="dev.hostforge.managed=true"
RESOURCE_LABEL="dev.hostforge.resource-type"
failures=0

for tool in docker df awk sort; do
  if ! command -v "${tool}" >/dev/null 2>&1; then
    echo "error: ${tool} is required" >&2
    exit 1
  fi
done
if ! docker info >/dev/null 2>&1; then
  echo "error: Docker is unavailable to the current user" >&2
  exit 1
fi

min_free_bytes="${HF_DATABASE_MIN_FREE_DISK_BYTES:-}"
if [[ -z "${min_free_bytes}" && -r "${ENV_FILE}" ]]; then
  min_free_bytes="$(awk -F= '/^[[:space:]]*HOSTFORGE_DATABASE_MIN_FREE_DISK_BYTES=/{value=$0; sub(/^[^=]*=/, "", value); gsub(/^[[:space:]]+|[[:space:]]+$/, "", value); print value; exit}' "${ENV_FILE}")"
fi
min_free_bytes="${min_free_bytes:-${DEFAULT_MIN_FREE_BYTES}}"
if [[ ! "${min_free_bytes}" =~ ^[0-9]+$ ]]; then
  echo "error: HOSTFORGE_DATABASE_MIN_FREE_DISK_BYTES must be a non-negative integer" >&2
  exit 1
fi

docker_root="$(docker info --format '{{.DockerRootDir}}')"
available_bytes="$(df -PB1 "${docker_root}" | awk 'NR==2 {print $4}')"

echo "Host capacity"
df -h "${docker_root}"
printf 'Docker root: %s\n' "${docker_root}"
printf 'Free bytes: %s\n' "${available_bytes}"
printf 'HostForge reserve: %s bytes\n' "${min_free_bytes}"
if (( available_bytes < min_free_bytes )); then
  echo "FAIL: Docker storage is already below the HostForge database safety reserve" >&2
  failures=$((failures + 1))
else
  echo "PASS: Docker storage is above the configured safety reserve"
fi

mapfile -t containers < <(docker ps -a -q \
  --filter "label=${MANAGED_LABEL}" \
  --filter "label=${RESOURCE_LABEL}=database-container" | sort)

echo
echo "Managed database containers"
if (( ${#containers[@]} == 0 )); then
  echo "No managed database containers found."
else
  printf '%-34s %-12s %-14s %s\n' "NAME" "CPU LIMIT" "MEMORY LIMIT" "IMAGE"
  for container_id in "${containers[@]}"; do
    inspection="$(docker inspect --format '{{.Name}}|{{.HostConfig.NanoCpus}}|{{.HostConfig.Memory}}|{{json .HostConfig.PortBindings}}|{{.Config.Image}}' "${container_id}")"
    IFS='|' read -r name nano_cpus memory_bytes port_bindings image_ref <<<"${inspection}"
    name="${name#/}"
    printf '%-34s %-12s %-14s %s\n' "${name}" "${nano_cpus}" "${memory_bytes}" "${image_ref}"
    if [[ "${nano_cpus}" == "0" || "${memory_bytes}" == "0" ]]; then
      echo "FAIL: ${name} is missing an enforced CPU or memory limit" >&2
      failures=$((failures + 1))
    fi
    if [[ "${port_bindings}" != "null" && "${port_bindings}" != "{}" ]]; then
      echo "FAIL: ${name} publishes a host port: ${port_bindings}" >&2
      failures=$((failures + 1))
    fi
  done
fi

echo
echo "Managed database volumes"
mapfile -t volumes < <(docker volume ls -q \
  --filter "label=${MANAGED_LABEL}" \
  --filter "label=${RESOURCE_LABEL}=database-volume" | sort)
if (( ${#volumes[@]} == 0 )); then
  echo "No managed database volumes found."
else
  printf '%-48s %14s %s\n' "VOLUME" "ALLOCATED" "MOUNTPOINT"
  managed_volume_bytes=0
  measured_volumes=0
  for volume_name in "${volumes[@]}"; do
    mountpoint="$(docker volume inspect --format '{{.Mountpoint}}' "${volume_name}")"
    allocated_bytes=""
    if [[ -n "${mountpoint}" && -d "${mountpoint}" ]]; then
      allocated_bytes="$(du -s -B1 -- "${mountpoint}" 2>/dev/null | awk '{print $1}' || true)"
    fi
    if [[ "${allocated_bytes}" =~ ^[0-9]+$ ]]; then
      allocated_human="$(awk -v bytes="${allocated_bytes}" 'BEGIN { split("B KiB MiB GiB TiB", units); value=bytes; unit=1; while (value >= 1024 && unit < 5) { value /= 1024; unit++ } printf "%.2f %s", value, units[unit] }')"
      printf '%-48s %14s %s\n' "${volume_name}" "${allocated_human}" "${mountpoint}"
      managed_volume_bytes=$((managed_volume_bytes + allocated_bytes))
      measured_volumes=$((measured_volumes + 1))
    else
      printf '%-48s %14s %s\n' "${volume_name}" "unavailable" "${mountpoint:-unknown}"
    fi
  done
  if (( measured_volumes > 0 )); then
    managed_volume_human="$(awk -v bytes="${managed_volume_bytes}" 'BEGIN { split("B KiB MiB GiB TiB", units); value=bytes; unit=1; while (value >= 1024 && unit < 5) { value /= 1024; unit++ } printf "%.2f %s", value, units[unit] }')"
    printf 'Managed database volume total: %s bytes (%s) across %d measured volume(s)\n' "${managed_volume_bytes}" "${managed_volume_human}" "${measured_volumes}"
  fi
fi

echo
echo "Managed environment networks"
mapfile -t networks < <(docker network ls -q \
  --filter "label=${MANAGED_LABEL}" \
  --filter "label=${RESOURCE_LABEL}=environment-network" | sort)
if (( ${#networks[@]} == 0 )); then
  echo "No managed environment networks found."
else
  for network_id in "${networks[@]}"; do
    docker network inspect --format '{{.Name}} containers={{len .Containers}}' "${network_id}"
  done
fi

echo
echo "Docker disk usage"
docker system df
if (( ${#volumes[@]} > 0 )); then
  echo
  echo "Detailed Docker usage (find the managed volume names listed above)"
  docker system df -v
fi

echo
echo "Standard database listeners (diagnostic only)"
if command -v ss >/dev/null 2>&1; then
  listeners="$(ss -lntH | awk '$4 ~ /:(3306|5432|6379|27017)$/ {print}' || true)"
  if [[ -n "${listeners}" ]]; then
    printf '%s\n' "${listeners}"
    echo "Review these listeners. Managed-container host publication is checked separately above."
  else
    echo "No standard database TCP listeners are bound on the host."
  fi
else
  echo "ss is unavailable; skipped host-listener diagnostics."
fi

echo
if (( failures > 0 )); then
  echo "Database VPS audit failed with ${failures} blocking finding(s)." >&2
  exit 1
fi
echo "Database VPS audit passed."
