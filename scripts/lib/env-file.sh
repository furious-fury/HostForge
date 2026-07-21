#!/usr/bin/env bash

# Append a non-secret default only when an active assignment is absent. Existing
# operator values always win, including custom Caddy storage layouts.
hostforge_read_env_value() {
  local env_file="$1" key="$2"
  awk -v key="${key}" '
    $0 ~ "^[[:space:]]*" key "=" {
      value=$0
      sub("^[[:space:]]*" key "=", "", value)
      gsub(/\r$/, "", value)
      if ((value ~ /^".*"$/) || (value ~ /^\047.*\047$/)) {
        value=substr(value, 2, length(value)-2)
      }
      print value
      exit
    }
  ' "${env_file}"
}

hostforge_ensure_env_default() {
  local env_file="$1" key="$2" value="$3"
  if [[ ! "${key}" =~ ^[A-Z][A-Z0-9_]*$ ]]; then
    echo "error: invalid environment key ${key}" >&2
    return 1
  fi
  if [[ ! -f "${env_file}" ]]; then
    echo "error: environment file ${env_file} does not exist" >&2
    return 1
  fi
  if tr -d '\r' <"${env_file}" | grep -Eq "^[[:space:]]*${key}="; then
    return 0
  fi
  printf '\n%s=%s\n' "${key}" "${value}" >>"${env_file}"
}
