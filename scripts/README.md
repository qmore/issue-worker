# Installer scripts

- `install.sh`: recommended worker installer. Downloads a prebuilt GitHub Release binary, verifies its SHA-256 checksum, and installs it without requiring Go.
- `install-daemon.sh`: development/source-build installer. Requires Go and should only be used on machines that are intentionally development environments.
