#!/bin/sh
# End-to-end smoke test for install.sh.
#
# Serves a fake release archive over HTTP, runs install.sh against it with a
# relative PREFIX, and asserts that the alias symlink is absolute and resolves
# and that the post-install version check succeeds. A `sudo` shim goes first on
# PATH so the install lands in a temporary prefix without privileges.
#
# Usage: scripts/install-smoke-test.sh    (requires curl, tar, python3)

set -eu

repo_root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
server_pid=""

cleanup() {
    if [ -n "$server_pid" ]; then
        kill "$server_pid" 2>/dev/null || true
        wait "$server_pid" 2>/dev/null || true
    fi
    rm -rf "$work"
}
trap cleanup EXIT

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

for tool in curl tar python3; do
    command -v "$tool" >/dev/null 2>&1 || fail "$tool is required"
done

# The platform names must match the uname mapping in install.sh.
os_name=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os_name" in
    linux | darwin) ;;
    *) fail "unsupported OS for this test: $os_name" ;;
esac

raw_arch=$(uname -m)
case "$raw_arch" in
    x86_64 | amd64) arch_name="amd64" ;;
    aarch64 | arm64)
        if [ "$os_name" = "darwin" ]; then
            arch_name="arm64"
        else
            arch_name="armv8"
        fi
        ;;
    *) fail "unsupported architecture for this test: $raw_arch" ;;
esac

name="${NAME:-mu}"
version=$(sed -n 's/^VERSION="\(.*\)"$/\1/p' "$repo_root/install.sh")
[ -n "$version" ] || fail "could not read VERSION from install.sh"

base_name="${name}-${os_name}-${arch_name}"
# Release archives carry the version suffix but contain the plain platform
# binary, which is what install.sh looks for when it unpacks.
bin_name="${base_name}-${version}"
serve_root="$work/serve"
payload="$work/payload"
mkdir -p "$serve_root/$version" "$payload" "$work/run" "$work/shim"

# The fake release binary. It mimics the real CLI: --version prints the version
# and anything else (e.g. the removed `version` subcommand) fails with usage.
cat > "$payload/$base_name" <<'EOF'
#!/bin/sh
case "$1" in
    --version | -v) echo "myUtilities version v0.0.0-smoke-test" ;;
    *)
        echo "Usage: myUtilities <command> [flags]" >&2
        exit 80
        ;;
esac
EOF
chmod +x "$payload/$base_name"
tar czf "$serve_root/$version/${bin_name}.tar.gz" -C "$payload" "$base_name"

# A `sudo` shim, so the test can install into the temporary prefix unprivileged.
cat > "$work/shim/sudo" <<'EOF'
#!/bin/sh
exec "$@"
EOF
chmod +x "$work/shim/sudo"

# Serve the fake release and wait until it answers.
port=""
for candidate in 18080 18081 18082 18083 18084; do
    python3 -m http.server "$candidate" --bind 127.0.0.1 --directory "$serve_root" >"$work/http.log" 2>&1 &
    server_pid=$!
    archive_url="http://127.0.0.1:$candidate/$version/${bin_name}.tar.gz"
    for _ in 1 2 3 4 5 6 7 8 9 10; do
        if curl -fsS -o /dev/null "$archive_url" 2>/dev/null; then
            port="$candidate"
            break
        fi
        sleep 0.3
    done
    if [ -n "$port" ]; then
        break
    fi
    kill "$server_pid" 2>/dev/null || true
    wait "$server_pid" 2>/dev/null || true
    server_pid=""
done
[ -n "$port" ] || fail "could not start a local HTTP server for the fake release"

# Install from a clean directory with a relative PREFIX that does not exist yet,
# so install.sh has to create it.
output=$(cd "$work/run" && PATH="$work/shim:$PATH" NAME="$name" PREFIX="prefix/bin" \
    BASE_URL="http://127.0.0.1:$port" sh "$repo_root/install.sh" 2>&1) ||
    fail "install.sh exited non-zero:
$output"

echo "$output" | grep -q "myUtilities version v0.0.0-smoke-test" ||
    fail "post-install version check did not print the installed version:
$output"

alias_path="$work/run/prefix/bin/$name"
[ -L "$alias_path" ] || fail "expected a symlink at $alias_path"
[ -x "$alias_path" ] || fail "$alias_path is not executable (dangling symlink?)"

target=$(readlink "$alias_path")
case "$target" in
    /*) ;;
    *) fail "alias symlink target is not absolute: $target" ;;
esac

# The alias must work from any working directory, which only holds when the
# symlink target is absolute.
got=$(cd / && "$alias_path" --version)
[ "$got" = "myUtilities version v0.0.0-smoke-test" ] || fail "unexpected alias output: $got"

echo "install.sh smoke test passed (relative PREFIX, alias -> $target)"
