package tailscale

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type Client struct {
	Socket string
	http   *http.Client
}

type Peer struct {
	ID             string   `json:"id"`
	HostName       string   `json:"hostname"`
	DNSName        string   `json:"dns_name"`
	OS             string   `json:"os"`
	IPs            []string `json:"ips"`
	Online         bool     `json:"online"`
	Active         bool     `json:"active"`
	ExitNode       bool     `json:"exit_node"`
	ExitNodeOption bool     `json:"exit_node_option"`
	Relay          string   `json:"relay,omitempty"`
	CurAddr        string   `json:"cur_addr,omitempty"`
	Direct         bool     `json:"direct"`
	LastSeen       string   `json:"last_seen,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	User           string   `json:"user,omitempty"`
}

type Status struct {
	Available      bool     `json:"available"`
	Error          string   `json:"error,omitempty"`
	Version        string   `json:"version"`
	BackendState   string   `json:"backend_state"`
	TUN            bool     `json:"tun"`
	AuthURL        string   `json:"auth_url,omitempty"`
	IPs            []string `json:"ips"`
	HostName       string   `json:"hostname"`
	DNSName        string   `json:"dns_name"`
	MagicDNSSuffix string   `json:"magic_dns_suffix"`
	TailnetName    string   `json:"tailnet_name"`
	Health         []string `json:"health,omitempty"`
	CertDomains    []string `json:"cert_domains,omitempty"`
	HTTPSEnabled   bool     `json:"https_enabled"`
	Peers          []Peer   `json:"peers"`
	PeerCount      int      `json:"peer_count"`
	OnlinePeers    int      `json:"online_peers"`
	ExitNode       string   `json:"exit_node,omitempty"`
	Services       []string `json:"services,omitempty"`
	Funnel         bool     `json:"funnel"`
	Socket         string   `json:"socket"`
}

type PeerStatusJSON struct {
	ID             string   `json:"ID"`
	HostName       string   `json:"HostName"`
	DNSName        string   `json:"DNSName"`
	OS             string   `json:"OS"`
	TailscaleIPs   []string `json:"TailscaleIPs"`
	Online         bool     `json:"Online"`
	Active         bool     `json:"Active"`
	ExitNode       bool     `json:"ExitNode"`
	ExitNodeOption bool     `json:"ExitNodeOption"`
	Relay          string   `json:"Relay"`
	CurAddr        string   `json:"CurAddr"`
	Addrs          []string `json:"Addrs"`
	LastSeen       string   `json:"LastSeen"`
	Tags           []string `json:"Tags"`
	UserID         any      `json:"UserID"`
}

type statusJSON struct {
	Version        string                     `json:"Version"`
	TUN            bool                       `json:"TUN"`
	BackendState   string                     `json:"BackendState"`
	AuthURL        string                     `json:"AuthURL"`
	TailscaleIPs   []string                   `json:"TailscaleIPs"`
	MagicDNSSuffix string                     `json:"MagicDNSSuffix"`
	Health         []string                   `json:"Health"`
	CertDomains    []string                   `json:"CertDomains"`
	Self           *PeerStatusJSON            `json:"Self"`
	Peer           map[string]*PeerStatusJSON `json:"Peer"`
	CurrentTailnet *struct {
		Name           string `json:"Name"`
		MagicDNSSuffix string `json:"MagicDNSSuffix"`
	} `json:"CurrentTailnet"`
	ExitNodeStatus *struct {
		ID string `json:"ID"`
	} `json:"ExitNodeStatus"`
	User map[string]struct {
		LoginName   string `json:"LoginName"`
		DisplayName string `json:"DisplayName"`
	} `json:"User"`
}

func NewClient(socket string) *Client {
	if socket == "" {
		socket = "/var/run/tailscale/tailscaled.sock"
	}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socket)
		},
		DisableKeepAlives: true,
	}
	return &Client{Socket: socket, http: &http.Client{Transport: tr, Timeout: 5 * time.Second}}
}

func (c *Client) Available() bool {
	_, err := os.Stat(c.Socket)
	return err == nil
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, headers map[string]string) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://local-tailscaled.sock"+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Host", "local-tailscaled.sock")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return c.http.Do(req)
}

func (c *Client) Status(ctx context.Context) (Status, error) {
	if !c.Available() {
		return Status{Available: false, Error: "tailscaled socket not found", Socket: c.Socket}, nil
	}
	resp, err := c.do(ctx, http.MethodGet, "/localapi/v0/status", nil, nil)
	if err != nil {
		return Status{Available: false, Error: err.Error(), Socket: c.Socket}, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Status{Available: false, Error: strings.TrimSpace(string(b)), Socket: c.Socket}, nil
	}
	var raw statusJSON
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&raw); err != nil {
		return Status{Available: false, Error: err.Error(), Socket: c.Socket}, nil
	}
	st := Status{
		Available: true, Version: raw.Version, BackendState: raw.BackendState, TUN: raw.TUN,
		AuthURL: raw.AuthURL, IPs: raw.TailscaleIPs, MagicDNSSuffix: raw.MagicDNSSuffix,
		Health: raw.Health, CertDomains: raw.CertDomains, Socket: c.Socket,
	}
	if raw.CurrentTailnet != nil {
		st.TailnetName = raw.CurrentTailnet.Name
		if st.MagicDNSSuffix == "" {
			st.MagicDNSSuffix = raw.CurrentTailnet.MagicDNSSuffix
		}
	}
	if raw.Self != nil {
		st.HostName = raw.Self.HostName
		st.DNSName = strings.TrimSuffix(raw.Self.DNSName, ".")
	}
	if raw.ExitNodeStatus != nil {
		st.ExitNode = raw.ExitNodeStatus.ID
	}
	for _, p := range raw.Peer {
		peer := Peer{
			ID: p.ID, HostName: p.HostName, DNSName: strings.TrimSuffix(p.DNSName, "."),
			OS: p.OS, IPs: p.TailscaleIPs, Online: p.Online, Active: p.Active,
			ExitNode: p.ExitNode, ExitNodeOption: p.ExitNodeOption, Relay: p.Relay,
			CurAddr: p.CurAddr, Direct: p.CurAddr != "", LastSeen: p.LastSeen,
			Tags: p.Tags,
		}
		st.Peers = append(st.Peers, peer)
	}
	sort.Slice(st.Peers, func(i, j int) bool {
		if st.Peers[i].Online != st.Peers[j].Online {
			return st.Peers[i].Online
		}
		return st.Peers[i].HostName < st.Peers[j].HostName
	})
	st.PeerCount = len(st.Peers)
	for _, p := range st.Peers {
		if p.Online {
			st.OnlinePeers++
		}
	}
	for _, d := range st.CertDomains {
		if st.DNSName != "" && strings.EqualFold(d, st.DNSName) {
			st.HTTPSEnabled = true
		}
	}
	svc, _ := c.ServeStatus(ctx)
	st.Services = svc
	if cfg, _, err := c.ServeConfig(ctx); err == nil {
		for _, enabled := range cfg.AllowFunnel {
			if enabled {
				st.Funnel = true
			}
		}
	}
	return st, nil
}

func (c *Client) InteractiveLogin(ctx context.Context) error {
	resp, err := c.do(ctx, http.MethodPost, "/localapi/v0/login-interactive", nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("tailscale login: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}

func (c *Client) LoginWithAuthKey(ctx context.Context, key string) error {
	if !strings.HasPrefix(key, "tskey-") {
		return errors.New("invalid Tailscale auth key")
	}
	body, _ := json.Marshal(map[string]any{"AuthKey": key})
	resp, err := c.do(ctx, http.MethodPost, "/localapi/v0/start", body, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("tailscale start: %s", strings.TrimSpace(string(b)))
	}
	return nil
}

func (c *Client) Prefs(ctx context.Context) (map[string]any, error) {
	resp, err := c.do(ctx, http.MethodGet, "/localapi/v0/prefs", nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("tailscale prefs: %s", resp.Status)
	}
	var out map[string]any
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) UpdatePrefs(ctx context.Context, updates map[string]any) error {
	prefs, err := c.Prefs(ctx)
	if err != nil {
		return err
	}
	for k, v := range updates {
		prefs[k] = v
	}
	body, _ := json.Marshal(prefs)
	resp, err := c.do(ctx, http.MethodPost, "/localapi/v0/prefs", body, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("tailscale prefs: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}

type serveConfig struct {
	TCP         map[string]any       `json:"TCP,omitempty"`
	Web         map[string]webConfig `json:"Web,omitempty"`
	AllowFunnel map[string]bool      `json:"AllowFunnel,omitempty"`
}

type webConfig struct {
	Handlers map[string]handler `json:"Handlers"`
}

type handler struct {
	Proxy string `json:"Proxy,omitempty"`
	Text  string `json:"Text,omitempty"`
}

func (c *Client) ServeConfig(ctx context.Context) (serveConfig, string, error) {
	var cfg serveConfig
	resp, err := c.do(ctx, http.MethodGet, "/localapi/v0/serve-config", nil, nil)
	if err != nil {
		return cfg, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return cfg, "", fmt.Errorf("tailscale serve config: %s", resp.Status)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&cfg); err != nil {
		return cfg, "", err
	}
	return cfg, resp.Header.Get("Etag"), nil
}

func (c *Client) ServeStatus(ctx context.Context) ([]string, error) {
	cfg, _, err := c.ServeConfig(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	for hp := range cfg.Web {
		out = append(out, hp)
	}
	sort.Strings(out)
	return out, nil
}

func (c *Client) EnableServe(ctx context.Context, funnel bool) error {
	st, err := c.Status(ctx)
	if err != nil {
		return err
	}
	if !st.Available || st.BackendState != "Running" {
		return errors.New("Tailscale is not connected")
	}
	if st.DNSName == "" {
		return errors.New("MagicDNS name is not available")
	}
	if !st.HTTPSEnabled {
		return errors.New("Tailscale HTTPS certificates are not enabled for this node; enable HTTPS in the Tailscale admin console DNS page")
	}
	target := "http://127.0.0.1:8088"
	cfg, etag, err := c.ServeConfig(ctx)
	if err != nil {
		return err
	}
	if cfg.TCP == nil {
		cfg.TCP = map[string]any{}
	}
	if cfg.Web == nil {
		cfg.Web = map[string]webConfig{}
	}
	if cfg.AllowFunnel == nil {
		cfg.AllowFunnel = map[string]bool{}
	}
	key := st.DNSName + ":443"
	cfg.TCP["443"] = map[string]any{"HTTPS": true}
	cfg.Web[key] = webConfig{Handlers: map[string]handler{"/": {Proxy: target}}}
	cfg.AllowFunnel[key] = funnel
	body, _ := json.Marshal(cfg)
	headers := map[string]string{}
	if etag != "" {
		headers["If-Match"] = etag
	}
	resp, err := c.do(ctx, http.MethodPost, "/localapi/v0/serve-config", body, headers)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("tailscale serve: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}

func (c *Client) DisableServe(ctx context.Context) error {
	cfg, etag, err := c.ServeConfig(ctx)
	if err != nil {
		return err
	}
	st, _ := c.Status(ctx)
	key := st.DNSName + ":443"
	delete(cfg.Web, key)
	delete(cfg.AllowFunnel, key)
	delete(cfg.TCP, "443")
	body, _ := json.Marshal(cfg)
	headers := map[string]string{}
	if etag != "" {
		headers["If-Match"] = etag
	}
	resp, err := c.do(ctx, http.MethodPost, "/localapi/v0/serve-config", body, headers)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("tailscale serve: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}

type WhoIs struct {
	LoginName   string   `json:"login_name"`
	DisplayName string   `json:"display_name"`
	NodeName    string   `json:"node_name"`
	Tags        []string `json:"tags,omitempty"`
	UserID      int64    `json:"user_id,omitempty"`
}

func (c *Client) WhoIs(ctx context.Context, addr string) (*WhoIs, error) {
	resp, err := c.do(ctx, http.MethodGet, "/localapi/v0/whois?addr="+addr, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("tailscale whois: %s", strings.TrimSpace(string(b)))
	}
	var raw struct {
		UserProfile struct {
			ID          int64  `json:"ID"`
			LoginName   string `json:"LoginName"`
			DisplayName string `json:"DisplayName"`
		} `json:"UserProfile"`
		Node struct {
			Name string   `json:"Name"`
			Tags []string `json:"Tags"`
		} `json:"Node"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&raw); err != nil {
		return nil, err
	}
	if raw.UserProfile.LoginName == "" {
		return nil, errors.New("whois returned no user")
	}
	return &WhoIs{
		LoginName: raw.UserProfile.LoginName, DisplayName: raw.UserProfile.DisplayName,
		NodeName: strings.TrimSuffix(raw.Node.Name, "."), Tags: raw.Node.Tags, UserID: raw.UserProfile.ID,
	}, nil
}

func (c *Client) Logout(ctx context.Context) error {
	resp, err := c.do(ctx, http.MethodPost, "/localapi/v0/logout", nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("tailscale logout: %s", resp.Status)
	}
	return nil
}

func ParseLoginURL(ctx context.Context, r io.Reader) (string, error) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "https://login.tailscale.com/") {
			return line, nil
		}
	}
	return "", errors.New("no login URL found")
}
