#!/usr/bin/env bash
# VAPID keys for scan alerts that arrive while Stockbot is closed.
# Sourced by setup.sh and pantry-update.sh. PANTRY_DIR must already be set.
# A pair that is already in .env is kept. Generating a new pair signs out
# browsers that subscribed with the old public key, so this does not rotate.

vapid_env_value() {
  { grep "^${1}=" "${PANTRY_DIR}/.env" || true; } | head -1 | cut -d= -f2- | tr -d '[:space:]'
}

vapid_set_env() {
  local key="$1" value="$2" tmp
  tmp=$(mktemp)
  if [[ -f ${PANTRY_DIR}/.env ]] && grep -q "^${key}=" "${PANTRY_DIR}/.env"; then
    awk -v key="$key" -v value="$value" '
      BEGIN { done = 0 }
      $0 ~ ("^" key "=") && done == 0 {
        print key "=" value
        done = 1
        next
      }
      { print }
      END { if (done == 0) print key "=" value }
    ' "${PANTRY_DIR}/.env" > "$tmp" || return 1
  else
    if [[ -f ${PANTRY_DIR}/.env ]]; then
      cat "${PANTRY_DIR}/.env" > "$tmp" || return 1
    fi
    printf '%s=%s\n' "$key" "$value" >> "$tmp" || return 1
  fi
  mv "$tmp" "${PANTRY_DIR}/.env"
}

# vapid_generate_pair prints the public key, then the private key.
# Both are unpadded base64url. The public key is the uncompressed P-256 point.
vapid_generate_pair() {
  local pem text priv_hex pub_hex
  if ! command -v openssl >/dev/null 2>&1; then
    echo "openssl is required to generate VAPID keys" >&2
    return 1
  fi
  if ! command -v python3 >/dev/null 2>&1; then
    echo "python3 is required to generate VAPID keys" >&2
    return 1
  fi
  pem=$(openssl ecparam -name prime256v1 -genkey -noout) || return 1
  text=$(printf '%s\n' "$pem" | openssl pkey -text -noout) || return 1
  priv_hex=$(printf '%s\n' "$text" | awk 'BEGIN{p=0} /^priv:/{p=1; next} /^pub:/{p=0} p{gsub(/[^0-9a-fA-F]/,""); printf "%s",$0}')
  pub_hex=$(printf '%s\n' "$text" | awk 'BEGIN{p=0} /^pub:/{p=1; next} /^ASN1/{p=0} p{gsub(/[^0-9a-fA-F]/,""); printf "%s",$0}')
  # OpenSSL sometimes prints the private integer with a leading zero byte.
  if [[ ${#priv_hex} -eq 66 && "$priv_hex" == 00* ]]; then
    priv_hex=${priv_hex:2}
  fi
  if [[ ${#priv_hex} -ne 64 || ${#pub_hex} -ne 130 || "$pub_hex" != 04* ]]; then
    echo "Could not read a P-256 VAPID key from openssl" >&2
    return 1
  fi
  python3 - "$pub_hex" "$priv_hex" <<'PY'
import base64, binascii, sys
for hex_key in sys.argv[1:]:
    raw = binascii.unhexlify(hex_key)
    print(base64.urlsafe_b64encode(raw).decode("ascii").rstrip("="))
PY
}

# ensure_vapid_keys writes a missing pair into ${PANTRY_DIR}/.env.
# Returns 0 when a pair is present. Returns 1 when it could not create one.
ensure_vapid_keys() {
  local pub priv subject host pair
  if [[ ! -f ${PANTRY_DIR}/.env ]]; then
    echo "No ${PANTRY_DIR}/.env; VAPID keys were not written" >&2
    return 1
  fi
  pub=$(vapid_env_value VAPID_PUBLIC_KEY)
  priv=$(vapid_env_value VAPID_PRIVATE_KEY)
  if [[ -z "$pub" || -z "$priv" ]]; then
    if [[ -n "$pub" || -n "$priv" ]]; then
      echo "Replacing a partial VAPID pair so the public and private keys match" >&2
    fi
    pair=$(vapid_generate_pair) || return 1
    pub=$(printf '%s\n' "$pair" | sed -n '1p')
    priv=$(printf '%s\n' "$pair" | sed -n '2p')
    if [[ -z "$pub" || -z "$priv" ]]; then
      echo "Could not generate VAPID keys" >&2
      return 1
    fi
    vapid_set_env VAPID_PUBLIC_KEY "$pub" || return 1
    vapid_set_env VAPID_PRIVATE_KEY "$priv" || return 1
    echo "Generated VAPID keys for scan alerts"
  fi
  subject=$(vapid_env_value VAPID_SUBJECT)
  if [[ -z "$subject" ]]; then
    host=$(vapid_env_value PUBLIC_HOST)
    if [[ -n "$host" ]]; then
      subject="mailto:pantry@${host}"
    else
      subject="mailto:pantry@localhost"
    fi
    vapid_set_env VAPID_SUBJECT "$subject" || return 1
    echo "Set VAPID_SUBJECT for scan alerts"
  fi
  return 0
}
