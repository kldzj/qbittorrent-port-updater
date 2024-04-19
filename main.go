package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Noah-Huppert/gointerrupt"
	"github.com/caarlos0/env/v9"
)

// Config is the tool's configuration, loaded from env vars
type Config struct {
	// PortFile is the path to the file which contains only the VPNs forwarded port
	PortFile string `env:"PORT_FILE,required"`

	// RefreshIntervalSeconds is the number of seconds between refreshes of the port file and setting of the qBittorrent torrent port
	RefreshIntervalSeconds int `env:"REFRESH_INTERVAL_SECONDS,required" envDefault:"60"`

	// QBittorrentAPINetloc is the network location of the qBittorrent API server
	QBittorrentAPINetloc string `env:"QBITTORRENT_API_NETLOC,required"`

	// QBittorrentUsername is the username to use when authenticating with the QBittorrent API
	QBittorrentUsername string `env:"QBITTORRENT_USERNAME,required" envDefault:"admin"`

	// QBittorrrentPassword is the password to use when authenticating with the QBittorrent API
	QBittorrentPassword string `env:"QBITTORRENT_PASSWORD,required"`

	// AllowPortFileNotExist controls whether or not the port PortFile can not exist, if false and the PortFile does not exist then the program will error
	AllowPortFileNotExist bool `env:"ALLOW_PORT_FILE_NOT_EXIST,required" envDefault:"true"`
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
	// logger is used to output information
	logger *log.Logger

	// baseURL is the location of the qBittorrent API location
	baseURL url.URL

	// httpClient used to make API requests, stores auth cookies
	httpClient *http.Client

	// username to login with
	username string

	// password to login with
	password string
}

// NewQBittorrentClientOptions are options for creating a new QBittorrentClient
type NewQBittorrentClientOptions struct {
	// Logger is used to output information
	Logger *log.Logger

	// NetworkLocation is the location of the qBittorrent server
	NetworkLocation string

	// Username to login with
	Username string

	// Password to login with
	Password string
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
		logger:     opts.Logger,
		baseURL:    *baseURL,
		httpClient: httpClient,
		username:   opts.Username,
		password:   opts.Password,
	}, nil
}

// QBittorrentLoginNotAuthorizedError occurs when a qBittorrent API login request fails because credentials were not accepted by the server
type QBittorrentLoginNotAuthorizedError struct {
	err string
}

// Error returns an error message
func (e QBittorrentLoginNotAuthorizedError) Error() string {
	return e.err
}

// QBittorrentUnauthorizedError indicates the API client is not logged in
type QBittorrentUnauthorizedError struct{}

// Error returns a string representation
func (e QBittorrentUnauthorizedError) Error() string {
	return "not authorized"
}

// doReq sends the provided request, if autoLogin is true also tries to automatically login if the server indicates we are not logged in.
// Returns (response, response body, error)
func (client *QBittorrentClient) doReq(ctx context.Context, req *http.Request, autoLogin bool) (*http.Response, []byte, error) {
	//req.Header.Add("Referer", client.baseURL.String())

	resp, err := client.httpClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to make request: %s", err)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, fmt.Errorf("failed to read response body: %s", err)
	}

	if resp.StatusCode == http.StatusForbidden {
		// Try to automatically login and then repeat request
		if autoLogin {
			client.logger.Println("automatically logging in")
			if err := client.Login(ctx); err != nil {
				return resp, nil, fmt.Errorf("failed to login: %s", err)
			}

			return client.doReq(ctx, req, false)
		}

		return resp, respBody, QBittorrentUnauthorizedError{}
	} else if resp.StatusCode != http.StatusOK {
		return resp, respBody, fmt.Errorf("non-OK status code %d - %s: '%s'", resp.StatusCode, resp.Status, respBody)
	}

	return resp, respBody, nil
}

// Login authenticates with the API, must be called for each client in order for later API calls to work
// https://github.com/qbittorrent/qBittorrent/wiki/WebUI-API-(qBittorrent-4.1)#login
// Returns QBittorrentLoginNotAuthorizedError if the credentials were not accepted
func (client *QBittorrentClient) Login(ctx context.Context) error {
	// Setup request
	reqURL := client.baseURL
	reqURL.Path += "/api/v2/auth/login"

	reqBodyValues := url.Values{}
	reqBodyValues.Set("username", client.username)
	reqBodyValues.Set("password", client.password)

	req, err := http.NewRequest("POST", reqURL.String(), strings.NewReader(reqBodyValues.Encode()))
	if err != nil {
		return fmt.Errorf("failed to craft HTTP request: %s", err)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// Do request
	resp, respBody, err := client.doReq(ctx, req, false)
	if err != nil {
		return err
	}
	if resp.StatusCode == 403 {
		return QBittorrentLoginNotAuthorizedError{fmt.Sprintf("not authorized: '%s'", respBody)}
	}

	cookies := resp.Cookies()

	if len(cookies) == 0 {
		return fmt.Errorf("received no authentication cookie in response from the server, body: %s", respBody)
	}

	client.httpClient.Jar.SetCookies(&client.baseURL, cookies)

	// Authentication cookie should now be in jar
	return nil
}

// QBittorrentServerPreferences are settings which control the behavior of qBittorrent
type QBittorrentServerPreferences struct {
	// ListenPort is the port on which qBittorrent will listen for incoming torrent connections
	ListenPort uint16 `json:"listen_port,omitempty"`
}

// SetServerPreferences updates qBittorrent server preferences
// https://github.com/qbittorrent/qBittorrent/wiki/WebUI-API-(qBittorrent-4.1)#set-application-preferences
func (client *QBittorrentClient) SetServerPreferences(ctx context.Context, prefs QBittorrentServerPreferences) error {
	// Setup request
	reqURL := client.baseURL
	reqURL.Path += "/api/v2/app/setPreferences"

	prefsJSON, err := json.Marshal(prefs)
	if err != nil {
		return fmt.Errorf("failed to encode server preferences as JSON: %s", err)
	}
	reqBodyValues := url.Values{}
	reqBodyValues.Set("json", string(prefsJSON))

	req, err := http.NewRequest("POST", reqURL.String(), strings.NewReader(reqBodyValues.Encode()))
	if err != nil {
		return fmt.Errorf("failed to craft HTTP request: %s", err)
	}
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	// Do request
	_, _, err = client.doReq(ctx, req, true)
	if err != nil {
		return err
	}

	return nil
}

// GetServerPreferences retrieves the current qBittorrent server preferences
// https://github.com/qbittorrent/qBittorrent/wiki/WebUI-API-(qBittorrent-4.1)#get-application-preferences
func (client *QBittorrentClient) GetServerPreferences(ctx context.Context) (*QBittorrentServerPreferences, error) {
	// Setup request
	reqURL := client.baseURL
	reqURL.Path += "/api/v2/app/preferences"

	req, err := http.NewRequest("GET", reqURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to craft HTTP request: %s", err)
	}

	// Do request
	_, respBody, err := client.doReq(ctx, req, true)
	if err != nil {
		return nil, err
	}

	var prefs QBittorrentServerPreferences
	if err := json.Unmarshal(respBody, &prefs); err != nil {
		return nil, fmt.Errorf("failed to decode response into JSON: %s", err)
	}

	return &prefs, nil
}

// PortSyncer reads the port file and sets qBittorrent's torrent port if it differs
type PortSyncer struct {
	// logger is used to output information
	logger *log.Logger

	// qBittorrentClient is the API client used to make qBittorrent API requests
	qBittorrentClient *QBittorrentClient

	// allowPortFileNotExist indicates if the PortFile can not exist without an error being thrown
	allowPortFileNotExist bool

	// portFile is the file which contains the VPNs forwarded port
	portFile string
}

// NewPortSyncerOptions are options to create a new port syncer
type NewPortSyncerOptions struct {
	// Logger is used to output information
	Logger *log.Logger

	// QBittorrentClient is the API client used to make qBittorrent API requests
	QBittorrentClient *QBittorrentClient

	// AllowPortFileNotExist indicates if the PortFile can not exist without an error being thrown
	AllowPortFileNotExist bool

	// PortFile is the file which contains the VPNs forwarded port
	PortFile string
}

// NewPortSyncer creates a new PortSyncer
func NewPortSyncer(opts NewPortSyncerOptions) *PortSyncer {
	return &PortSyncer{
		logger:                opts.Logger,
		qBittorrentClient:     opts.QBittorrentClient,
		allowPortFileNotExist: opts.AllowPortFileNotExist,
		portFile:              opts.PortFile,
	}
}

// GetPortFileValue reads the port file and gets the integer value of the port
func (syncer *PortSyncer) GetPortFileValue() (uint16, error) {
	fileBytes, err := os.ReadFile(syncer.portFile)
	if err != nil {
		return 0, fmt.Errorf("failed to read port file '%s': %s", syncer.portFile, err)
	}

	fileInt, err := strconv.ParseUint(string(fileBytes), 10, 16)
	if err != nil {
		return 0, fmt.Errorf("failed to convert port file contents '%s' into int16: %s", fileBytes, err)
	}

	return uint16(fileInt), nil
}

// ReconcileTorrentPort ensures that qBittorrent's torrent port is the one provided
// Returns a boolean indicating if the port had to be changed
func (syncer *PortSyncer) ReconcileTorrentPort(ctx context.Context, port uint16) (bool, error) {
	prefs, err := syncer.qBittorrentClient.GetServerPreferences(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get current qBittorrent server preferences : %s", err)
	}

	if prefs.ListenPort == port {
		return false, nil
	}

	err = syncer.qBittorrentClient.SetServerPreferences(ctx, QBittorrentServerPreferences{
		ListenPort: port,
	})
	if err != nil {
		return false, fmt.Errorf("failed to set qBittorrent torrent port: %s", err)
	}

	return true, nil
}

// Sync reads the port file and ensures qBittorrent is using that port for torrents
// Will automatically login to the qBittorrent API if not authorized and re-call Sync() itself. The selfCall argument tracks if Sync() is re-calling itself so it doesn't recruse infinitely.
// Returns a boolean indicating if the qBittorrent port had to be changed
func (syncer *PortSyncer) Sync(ctx context.Context) (bool, error) {
	if _, err := os.Stat(syncer.portFile); errors.Is(err, os.ErrNotExist) {
		if syncer.allowPortFileNotExist {
			syncer.logger.Printf("port file '%s' does not exist yet, skipping sync...", syncer.portFile)
			return false, nil
		}

		return false, fmt.Errorf("port file '%s' does not", syncer.portFile)
	}

	port, err := syncer.GetPortFileValue()
	if err != nil {
		return false, fmt.Errorf("failed to get desired port from port file: %s", err)
	}

	changed, err := syncer.ReconcileTorrentPort(ctx, port)
	if err != nil {
		return false, fmt.Errorf("failed to reconcile qBittorrent port differences: %s", err)
	}

	if changed {
		syncer.logger.Printf("Changed qBittorrent torrent port to %d", port)
	} else {
		syncer.logger.Printf("No change to qBittorrent torrent port (is: %d)", port)
	}

	return changed, nil
}

// Loop calls the sync process on an interval until ctx is canceled
func (syncer *PortSyncer) Loop(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)

	if _, err := syncer.Sync(ctx); err != nil {
		return fmt.Errorf("failed to sync port: %s", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if _, err := syncer.Sync(ctx); err != nil {
				return fmt.Errorf("failed to sync port: %s", err)
			}
		}
	}
}

func main() {
	ctxPair := gointerrupt.NewCtxPair(context.Background())

	// Load configuration
	cfg, err := LoadConfig()
	if err != nil {
		log.Fatalf("failed to load configuration: %s", err)
	}

	log.Println("loaded configuration")
	log.Printf("  Port File                : %s", cfg.PortFile)
	log.Printf("  Allow Port File Not Exist: %t", cfg.AllowPortFileNotExist)
	log.Printf("  Refresh Interval         : %ds", cfg.RefreshIntervalSeconds)
	log.Printf("  qBittorrent API          : %s", cfg.QBittorrentAPINetloc)
	log.Printf("  qBittorrent Username     : %s", cfg.QBittorrentUsername)

	redactedQBittorrentPW := "<READACTED>"
	if len(cfg.QBittorrentPassword) == 0 {
		redactedQBittorrentPW = "<EMPTY>"
	}
	log.Printf("  qBittorrent Password     : %s", redactedQBittorrentPW)

	// Create qBittorrent client
	qbittorrentLogger := log.Default()
	qbittorrentLogger.SetPrefix("qbittorrent")
	qBittorrentClient, err := NewQBittorrentClient(NewQBittorrentClientOptions{
		Logger:          qbittorrentLogger,
		NetworkLocation: cfg.QBittorrentAPINetloc,
		Username:        cfg.QBittorrentUsername,
		Password:        cfg.QBittorrentPassword,
	})
	if err != nil {
		log.Fatalf("failed to create qBittorrent API client: %s", err)
	}

	// Create syncer and start
	syncerLogger := log.Default()
	syncerLogger.SetPrefix("port-syncer")
	syncer := NewPortSyncer(NewPortSyncerOptions{
		Logger:                syncerLogger,
		QBittorrentClient:     qBittorrentClient,
		AllowPortFileNotExist: cfg.AllowPortFileNotExist,
		PortFile:              cfg.PortFile,
	})

	log.Println("starting sync loop")

	go func() {
		select {
		case <-ctxPair.Graceful().Done():
			log.Println("received graceful stop signal, exitting...")
		case <-ctxPair.Harsh().Done():
			log.Println("received harsh stop signal, exitting...")
		}
	}()

	err = syncer.Loop(ctxPair.Graceful(), time.Duration(cfg.RefreshIntervalSeconds)*time.Second)
	if err != nil {
		log.Fatalf("failed to run sync loop: %s", err)
	}

	log.Println("done")
}
