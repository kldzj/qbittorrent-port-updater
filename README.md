# qQittorrent Port Plugin
Automatically sets qBittorrent's torrent port based on a status file.

# Table Of Contents
- [Overview](#overview)
- [Configuration](#configuration)

# Overview
If you have a setup which runs your VPN and puts the port that is forwarded in a file then this tool will configure qBittorrent to use this port for torrenting.

# Configuration
Configuration values are supplied via environment variables:

- `QBITTORRENT_PORT_PLUGIN_PORT_FILE` (String, Required): Path to file which contains only the VPNs forwarded port
- `QBITTORRENT_PORT_PLUGIN_REFRESH_INTERVAL_SECONDS` (String, Default: `60`): The number of seconds between refreshes of reading the port file and setting qBittorrent torrent port
- `QBITTORRENT_PORT_PLUGIN_QBITTORRENT_API_NETLOC` (String, Required): Network location of qBittorrent server
- `QBITTORRENT_PORT_PLUGIN_QBITTORRENT_USERNAME` (String, Default: `admin`): The username used to authenticate with the qBittorrent API
- `QBITTORRENT_PORT_PLUGIN_QBITTORRENT_PASSWORD` (String, Required): The password used to authenticate with the qBittorrent API
- `QBITTORRENT_PORT_PLUGIN_ALLOW_PORT_FILE_NOT_EXIST` (Boolean, Default: `true`): If `true` then the program will allow the port file to not exist, this is useful if your VPN takes a moment to create the file. Set to `false` to raise an error if the port file does not exist