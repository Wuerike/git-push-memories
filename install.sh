#!/bin/sh
set -eu

repository="Wuerike/git-push-memories"
install_dir="${HOME}/.local/bin"

if [ "$(uname -s)" != "Linux" ]; then
  printf '%s\n' "gpm currently supports Linux only." >&2
  exit 1
fi

case "$(uname -m)" in
  x86_64|amd64) architecture="amd64" ;;
  aarch64|arm64) architecture="arm64" ;;
  *) printf '%s\n' "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

install_temporary=""
temporary="$(mktemp -d)"
trap 'rm -rf "$temporary"; [ -z "$install_temporary" ] || rm -f "$install_temporary"' EXIT HUP INT TERM
archive="git-push-memories_linux_${architecture}.tar.gz"
base_url="https://github.com/${repository}/releases/latest/download"

curl --fail --location --silent --show-error "${base_url}/${archive}" --output "${temporary}/${archive}"
curl --fail --location --silent --show-error "${base_url}/checksums.txt" --output "${temporary}/checksums.txt"
(cd "$temporary" && sha256sum --check --ignore-missing checksums.txt)
tar -xzf "${temporary}/${archive}" -C "$temporary"
mkdir -p "$install_dir"
install_target="${install_dir}/gpm"
install_temporary="$(mktemp "${install_dir}/.gpm.XXXXXX")"
release_binary="${temporary}/gpm"
if [ ! -f "$release_binary" ]; then
  release_binary="${temporary}/git-push-memories"
fi
install -m 0755 "$release_binary" "$install_temporary"
mv -f "$install_temporary" "$install_target"
install_temporary=""

printf '%s\n' "Installed gpm in ${install_dir}."
case ":${PATH}:" in
  *":${install_dir}:"*) ;;
  *) printf '%s\n' "Add ${install_dir} to PATH before continuing." ;;
esac
printf '%s\n' "Next: ${install_target} setup /path/to/memory-repository"
