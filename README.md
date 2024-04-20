# qBittorrent Port Updater
Automatically sets qBittorrent's torrent port based on a status file.

# Table Of Contents
- [Overview](#overview)
- [Usage](#usage)
- [Development](#development)

# Overview
If you have a setup which runs your VPN and puts the port that is forwarded in a file then this tool will configure qBittorrent to use this port for torrenting.

# Usage
The [`noahhuppert/qbittorrent-port-updater`](https://hub.docker.com/repository/docker/noahhuppert/qbittorrent-port-updater/general) Docker image is provided for each release. Environment variables are used for configuration, see the [Configuration section](#configuration).

Run the container so that it has access to a directory with a file containing the port which is forwarded. This file will be checked periodically by the program.

See [`examples/`](./examples/) for common container deployment tool examples.

## Configuration
Configuration values are supplied via environment variables:

- `QBITTORRENT_PORT_UPDATER_PORT_FILE` (String, Required): Path to file which contains only the VPNs forwarded port
- `QBITTORRENT_PORT_UPDATER_REFRESH_INTERVAL_SECONDS` (String, Default: `60`): The number of seconds between refreshes of reading the port file and setting qBittorrent torrent port
- `QBITTORRENT_PORT_UPDATER_QBITTORRENT_API_NETLOC` (String, Required): Network location of qBittorrent server
- `QBITTORRENT_PORT_UPDATER_QBITTORRENT_USERNAME` (String, Default: `admin`): The username used to authenticate with the qBittorrent API
- `QBITTORRENT_PORT_UPDATER_QBITTORRENT_PASSWORD` (String, Required): The password used to authenticate with the qBittorrent API
- `QBITTORRENT_PORT_UPDATER_ALLOW_PORT_FILE_NOT_EXIST` (Boolean, Default: `true`): If `true` then the program will allow the port file to not exist, this is useful if your VPN takes a moment to create the file. Set to `false` to raise an error if the port file does not exist
- `QBITTORRENT_PORT_UPDATER_VERBOSE` (Boolean, Default: `false`): If `true` debug information, with potentially sensitive values, will be printed to the console

# Development
Written in Go. Calls the qBittorrent API.

To develop:

1. `go mod download`
2. Make a copy of [`dev-example.env`](./dev-example.env) named `dev.env`, fill in your own values
3. `go run .`

## Releases
To make a new release:

1. Make a new GitHub release with a semantic version tag like `v<major>.<minor>.<patch>`
2. `docker build -t noahhuppert/qbittorrent-port-updater:<VERSION> .`
3. `docker push noahhuppert/qbittorrent-port-updater:<VERSION>`