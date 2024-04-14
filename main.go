package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"

	"github.com/caarlos0/env/v9"
)

// Config is the tool's configuration, loaded from env vars
type Config struct {
	// PortFile is the path to the file which contains only the VPNs forwarded port
	PortFile string `env:"PORT_FILE"`

	// RefreshIntervalSeconds is the number of seconds between refreshes of the port file and setting of the qBittorrent torrent port
	RefreshIntervalSeconds int `env:"REFRESH_INTERVAL_SECONDS" envDefault:"5"`

	// QBittorrentAPINetloc is the network location of the qBittorrent API server
	QBittorrentAPINetloc string `env:"QBITTORRENT_API_NETLOC"`

	// QBittorrentUsername is the username to use when authenticating with the QBittorrent API
	QBittorrentUsername string `env:"QBITTORRENT_USERNAME" envDefault:"admin"`

	// QBittorrrentPassword is the password to use when authenticating with the QBittorrent API
	QBittorrentPassword string `env:"QBITTORRENT_PASSWORD"`
}

// LoadConfig from environment vars
func LoadConfig() (*Config, error) {
	var cfg Config
	if err := env.ParseWithOptions(&cfg, env.Options{
		Prefix: "QBITTORRENT_PORT_PLUGIN_",
	}); err != nil {
		return nil, fmt.Errorf("failed to load configuration from env vars: %s", err)
	}

	return &cfg, nil
}

// QBittorrentClient is an API client for qBittorrent
type QBittorrentClient struct {
	// baseURL is the location of the qBittorrent API location
	baseURL url.URL

	// httpClient used to make API requests, stores auth cookies
	httpClient *http.Client
}

// NewQBittorrentClientOptions are options for creating a new QBittorrentClient
type NewQBittorrentClientOptions struct {
	// NetworkLocation is the location of the qBittorrent server
	NetworkLocation string
}

// NewQBittorrentClient creates a new QBittorrentClient
func NewQBittorrentClient(opts NewQBittorrentClientOptions) (*QBittorrentClient, error) {
	// Parse base URL
	baseURL, err := url.Parse(opts.NetworkLocation)
	if err != nil {
		return nil, fmt.Errorf("failed to parse network location into valid URL: %s", err)
	}

	// Create HTTP client
	cookieJar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar for http client: %s", err)
	}

	httpClient := &http.Client{
		Jar: cookieJar,
	}

	return &QBittorrentClient{
		baseURL:    *baseURL,
		httpClient: httpClient,
	}, nil
}

// baseHeaders returns the headers required for each request
func (client *QBittorrentClient) baseHeaders() map[string][]string {
	return map[string][]string{
		"Referer": {
			client.baseURL.String(),
		},
	}
}

// QBittorrentLoginNotAuthorizedError occurs when a qBittorrent API login request fails because credentials were not accepted by the server
type QBittorrentLoginNotAuthorizedError struct {
	err string
}

// Error returns an error message
func (e QBittorrentLoginNotAuthorizedError) Error() string {
	return e.err
}

// Login authenticates with the API, must be called for each client in order for later API calls to work
// https://github.com/qbittorrent/qBittorrent/wiki/WebUI-API-(qBittorrent-4.1)#login
// Returns QBittorrentLoginNotAuthorizedError if the credentials were not accepted
func (client *QBittorrentClient) Login(username string, password string) error {
	// Setup request
	reqURL := client.baseURL
	reqURL.Path += "/api/v2/auth/login"

	reqBody := io.NopCloser(strings.NewReader(fmt.Sprintf("username=%s&password=%s", username, password)))

	req := http.Request{
		Method: "GET",
		URL:    &reqURL,
		Body:   reqBody,
		Header: client.baseHeaders(),
	}

	// Do request
	resp, err := client.httpClient.Do(&req)
	if err != nil {
		return fmt.Errorf("failed to make HTTP request: %s", err)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %s", err)
	}
	if resp.StatusCode == 403 {
		return QBittorrentLoginNotAuthorizedError{fmt.Sprintf("not authorized: '%s'", respBody)}
	} else if resp.StatusCode != 200 {
		return fmt.Errorf("failed to login, status code=%d, body='%s'", resp.StatusCode, respBody)
	}

	// Authentication cookie should now be in jar
	return nil
}

// QBittorrentServerPreferences are settings which control the behavior of qBittorrent
type QBittorrentServerPreferences struct {
	// ListenPort is the port on which qBittorrent will listen for incoming torrent connections
	ListenPort int `json:"listen_port,omitempty"`
}

// SetServerPreferences updates qBittorrent server preferences
// https://github.com/qbittorrent/qBittorrent/wiki/WebUI-API-(qBittorrent-4.1)#set-application-preferences
func (client *QBittorrentClient) SetServerPreferences(prefs QBittorrentServerPreferences) error {
	// Setup request
	reqURL := client.baseURL
	reqURL.Path += "/api/v2/app/setPreferences"

	reqBodyBytes, err := json.Marshal(prefs)
	if err != nil {
		return fmt.Errorf("failed to encode server preferences as JSON: %s", err)
	}
	reqBody := io.NopCloser(strings.NewReader(fmt.Sprintf("json=%s", reqBodyBytes)))

	req := http.Request{
		Method: "POST",
		URL:    &reqURL,
		Body:   reqBody,
		Header: client.baseHeaders(),
	}

	// Do request
	resp, err := client.httpClient.Do(&req)
	if err != nil {
		return fmt.Errorf("failed to make HTTP request: %s", err)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %s", err)
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("failed to set server preferences: status code=%d, body='%s'", resp.StatusCode, respBody)
	}

	return nil
}

// GetServerPreferences retrieves the current qBittorrent server preferences
// https://github.com/qbittorrent/qBittorrent/wiki/WebUI-API-(qBittorrent-4.1)#get-application-preferences
func (client *QBittorrentClient) GetServerPreferences() (*QBittorrentServerPreferences, error) {
	// Setup request
	reqURL := client.baseURL
	reqURL.Path += "/api/v2/app/preferences"

	req := http.Request{
		Method: "GET",
		URL:    &reqURL,
		Header: client.baseHeaders(),
	}

	// Do request
	resp, err := client.httpClient.Do(&req)
	if err != nil {
		return nil, fmt.Errorf("failed to make HTTP request: %s", err)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %s", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get server preferences: status code=%d, body='%s'", resp.StatusCode, respBody)
	}

	var prefs QBittorrentServerPreferences
	if err := json.Unmarshal(respBody, &prefs); err != nil {
		return nil, fmt.Errorf("failed to decode response into JSON: %s", err)
	}

	return &prefs, nil
}
