package dockerstore

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Client struct {
	http    *http.Client
	host    string
	base    string
	compose []string
	appsDir string
	mu      sync.Mutex
}

type Port struct {
	IP          string `json:"IP,omitempty"`
	PrivatePort uint16 `json:"PrivatePort"`
	PublicPort  uint16 `json:"PublicPort,omitempty"`
	Type        string `json:"Type"`
}

type Container struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Image   string            `json:"Image"`
	ImageID string            `json:"ImageID"`
	Command string            `json:"Command"`
	Created int64             `json:"Created"`
	State   string            `json:"State"`
	Status  string            `json:"Status"`
	Ports   []Port            `json:"Ports"`
	Labels  map[string]string `json:"Labels"`
	Mounts  []Mount           `json:"Mounts"`
	Network string            `json:"Network"`
	AppID   string            `json:"app_id,omitempty"`
}

type Mount struct {
	Type        string `json:"Type"`
	Name        string `json:"Name,omitempty"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	Mode        string `json:"Mode,omitempty"`
	RW          bool   `json:"RW"`
}

type ContainerStats struct {
	Read     time.Time `json:"read"`
	CPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs  uint32 `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64            `json:"usage"`
		Limit uint64            `json:"limit"`
		Stats map[string]uint64 `json:"stats"`
	} `json:"memory_stats"`
	Networks map[string]struct {
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
	} `json:"networks"`
}

type StatsView struct {
	CPUPercent  float64 `json:"cpu_percent"`
	Memory      uint64  `json:"memory_bytes"`
	MemoryLimit uint64  `json:"memory_limit_bytes"`
	RXBytes     uint64  `json:"rx_bytes"`
	TXBytes     uint64  `json:"tx_bytes"`
}

type Image struct {
	ID         string   `json:"Id"`
	Tags       []string `json:"RepoTags"`
	Created    int64    `json:"Created"`
	Size       int64    `json:"Size"`
	Containers int64    `json:"Containers"`
}

type Volume struct {
	Name       string            `json:"Name"`
	Driver     string            `json:"Driver"`
	Mountpoint string            `json:"Mountpoint"`
	Labels     map[string]string `json:"Labels"`
	Scope      string            `json:"Scope"`
}

type Network struct {
	ID       string `json:"Id"`
	Name     string `json:"Name"`
	Driver   string `json:"Driver"`
	Scope    string `json:"Scope"`
	Internal bool   `json:"Internal"`
}

type SystemDF struct {
	Images struct {
		TotalCount int64 `json:"TotalCount"`
		TotalSize  int64 `json:"TotalSize"`
	} `json:"Images"`
	Containers struct {
		TotalCount int64 `json:"TotalCount"`
		TotalSize  int64 `json:"TotalSize"`
	} `json:"Containers"`
	Volumes struct {
		TotalCount int64 `json:"TotalCount"`
		TotalSize  int64 `json:"TotalSize"`
	} `json:"Volumes"`
	BuildCache struct {
		TotalCount int64 `json:"TotalCount"`
		TotalSize  int64 `json:"TotalSize"`
	} `json:"BuildCache"`
}

func NewClient(host, appsDir string) (*Client, error) {
	if host == "" {
		host = "unix:///var/run/docker.sock"
	}
	transport := &http.Transport{
		MaxIdleConns:    20,
		IdleConnTimeout: 30 * time.Second,
	}
	base := "http://docker"
	if strings.HasPrefix(host, "unix://") {
		socket := strings.TrimPrefix(host, "unix://")
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socket)
		}
	} else if strings.HasPrefix(host, "tcp://") {
		u, err := url.Parse(host)
		if err != nil {
			return nil, err
		}
		base = "http://" + u.Host
	} else {
		return nil, fmt.Errorf("unsupported docker host %q", host)
	}
	c := &Client{
		http: &http.Client{Transport: transport, Timeout: 30 * time.Second},
		host: host, base: base, appsDir: appsDir,
	}
	c.compose = detectCompose()
	return c, nil
}

func detectCompose() []string {
	if _, err := exec.LookPath("docker"); err == nil {
		if err := exec.Command("docker", "compose", "version").Run(); err == nil {
			return []string{"docker", "compose"}
		}
	}
	if p, err := exec.LookPath("docker-compose"); err == nil {
		return []string{p}
	}
	return nil
}

func (c *Client) Host() string { return c.host }

func (c *Client) ComposeAvailable() bool { return len(c.compose) > 0 }

func (c *Client) Ping(ctx context.Context) error {
	_, err := c.get(ctx, "/_ping")
	return err
}

func (c *Client) Info(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	if _, err := c.getJSON(ctx, "/info", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Version(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	if _, err := c.getJSON(ctx, "/version", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) ListContainers(ctx context.Context, all bool) ([]Container, error) {
	path := "/containers/json?all=0"
	if all {
		path = "/containers/json?all=1"
	}
	var raw []Container
	if _, err := c.getJSON(ctx, path, &raw); err != nil {
		return nil, err
	}
	for i := range raw {
		raw[i].AppID = raw[i].Labels["io.mazima.app"]
		if raw[i].AppID == "" {
			raw[i].AppID = raw[i].Labels["com.docker.compose.project"]
			raw[i].AppID = strings.TrimPrefix(raw[i].AppID, "mazima-")
		}
		for _, n := range raw[i].Networks() {
			raw[i].Network = n
			break
		}
	}
	sort.Slice(raw, func(i, j int) bool { return raw[i].Created > raw[j].Created })
	return raw, nil
}

func (c *Container) Networks() []string {
	var out []string
	if c.Network != "" {
		out = append(out, c.Network)
	}
	return out
}

func (c *Client) InspectContainer(ctx context.Context, id string) (map[string]any, error) {
	var out map[string]any
	if _, err := c.getJSON(ctx, "/containers/"+url.PathEscape(id)+"/json", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) StartContainer(ctx context.Context, id string) error {
	_, _, err := c.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/start", nil, nil)
	return err
}

func (c *Client) StopContainer(ctx context.Context, id string) error {
	_, _, err := c.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/stop?t=10", nil, nil)
	return err
}

func (c *Client) RestartContainer(ctx context.Context, id string) error {
	_, _, err := c.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/restart?t=10", nil, nil)
	return err
}

func (c *Client) RemoveContainer(ctx context.Context, id string, force bool) error {
	path := "/containers/" + url.PathEscape(id) + "?v=0"
	if force {
		path += "&force=1"
	}
	_, _, err := c.do(ctx, http.MethodDelete, path, nil, nil)
	return err
}

func (c *Client) Logs(ctx context.Context, id string, tail int, tty bool) (string, error) {
	if tail <= 0 || tail > 5000 {
		tail = 200
	}
	path := fmt.Sprintf("/containers/%s/logs?stdout=1&stderr=1&tail=%d&timestamps=1", url.PathEscape(id), tail)
	resp, err := c.request(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return "", fmt.Errorf("docker logs: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var out bytes.Buffer
	if tty {
		_, err = io.Copy(&out, io.LimitReader(resp.Body, 4<<20))
		return out.String(), err
	}
	if err := demuxDockerStream(resp.Body, &out); err != nil {
		return out.String(), err
	}
	return out.String(), nil
}

func demuxDockerStream(r io.Reader, w io.Writer) error {
	br := bufio.NewReader(r)
	header := make([]byte, 8)
	for {
		if _, err := io.ReadFull(br, header); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}
			return err
		}
		size := binary.BigEndian.Uint32(header[4:8])
		if size > 8<<20 {
			return fmt.Errorf("docker log frame too large: %d", size)
		}
		if _, err := io.CopyN(w, br, int64(size)); err != nil {
			return err
		}
	}
}

func (c *Client) Stats(ctx context.Context, id string) (StatsView, error) {
	var raw ContainerStats
	if _, err := c.getJSON(ctx, "/containers/"+url.PathEscape(id)+"/stats?stream=false&one-shot=true", &raw); err != nil {
		return StatsView{}, err
	}
	v := StatsView{Memory: raw.MemoryStats.Usage, MemoryLimit: raw.MemoryStats.Limit}
	cpuDelta := float64(raw.CPUStats.CPUUsage.TotalUsage) - float64(raw.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(raw.CPUStats.SystemUsage) - float64(raw.PreCPUStats.SystemUsage)
	online := float64(raw.CPUStats.OnlineCPUs)
	if online == 0 {
		online = 1
	}
	if sysDelta > 0 && cpuDelta > 0 {
		v.CPUPercent = float64(int((cpuDelta/sysDelta*online*100)*100+0.5)) / 100
	}
	if cache, ok := raw.MemoryStats.Stats["inactive_file"]; ok && raw.MemoryStats.Usage > cache {
		v.Memory = raw.MemoryStats.Usage - cache
	}
	for _, n := range raw.Networks {
		v.RXBytes += n.RxBytes
		v.TXBytes += n.TxBytes
	}
	return v, nil
}

func (c *Client) UpdateResources(ctx context.Context, id string, memoryBytes int64, nanoCPUs int64) error {
	body := map[string]int64{}
	if memoryBytes > 0 {
		body["Memory"] = memoryBytes
	}
	if nanoCPUs > 0 {
		body["NanoCpus"] = nanoCPUs
	}
	b, _ := json.Marshal(body)
	_, _, err := c.do(ctx, http.MethodPost, "/containers/"+url.PathEscape(id)+"/update", bytes.NewReader(b), nil)
	return err
}

func (c *Client) ListImages(ctx context.Context) ([]Image, error) {
	var out []Image
	if _, err := c.getJSON(ctx, "/images/json", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) RemoveImage(ctx context.Context, id string, force bool) error {
	path := "/images/" + url.PathEscape(id)
	if force {
		path += "?force=1"
	}
	_, _, err := c.do(ctx, http.MethodDelete, path, nil, nil)
	return err
}

func (c *Client) SystemDF(ctx context.Context) (SystemDF, error) {
	var out SystemDF
	_, err := c.getJSON(ctx, "/system/df", &out)
	return out, err
}

func (c *Client) PruneDanglingImages(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	_, err := c.postJSON(ctx, "/images/prune?filters="+url.QueryEscape(`{"dangling":{"true":true}}`), nil, &out)
	return out, err
}

func (c *Client) ListVolumes(ctx context.Context) ([]Volume, error) {
	var out struct {
		Volumes []Volume `json:"Volumes"`
	}
	if _, err := c.getJSON(ctx, "/volumes", &out); err != nil {
		return nil, err
	}
	return out.Volumes, nil
}

func (c *Client) ListNetworks(ctx context.Context) ([]Network, error) {
	var out []Network
	if _, err := c.getJSON(ctx, "/networks", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	resp, err := c.request(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("docker %s: %s", path, strings.TrimSpace(string(b)))
	}
	return b, nil
}

func (c *Client) getJSON(ctx context.Context, path string, out any) (int, error) {
	resp, err := c.request(ctx, http.MethodGet, path, nil)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return resp.StatusCode, fmt.Errorf("docker %s: %s", path, strings.TrimSpace(string(b)))
	}
	return resp.StatusCode, json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(out)
}

func (c *Client) postJSON(ctx context.Context, path string, body []byte, out any) (int, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	resp, err := c.request(ctx, http.MethodPost, path, r)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return resp.StatusCode, fmt.Errorf("docker %s: %s", path, strings.TrimSpace(string(b)))
	}
	if out != nil {
		return resp.StatusCode, json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out)
	}
	return resp.StatusCode, nil
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader, out any) (int, http.Header, error) {
	resp, err := c.request(ctx, method, path, body)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return resp.StatusCode, resp.Header, fmt.Errorf("docker %s: %s", path, strings.TrimSpace(string(b)))
	}
	if out != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out); err != nil && !errors.Is(err, io.EOF) {
			return resp.StatusCode, resp.Header, err
		}
	}
	return resp.StatusCode, resp.Header, nil
}

func (c *Client) request(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.http.Do(req)
}

func (c *Client) ComposeUp(ctx context.Context, id string, forcePull bool) (string, error) {
	if !c.ComposeAvailable() {
		return "", errors.New("Docker Compose v2 is not installed")
	}
	dir := c.appDir(id)
	args := append(append([]string{}, c.compose[1:]...), "--project-name", "mazima-"+id, "--project-directory", dir, "-f", filepath.Join(dir, "compose.yaml"))
	if forcePull {
		args = append(args, "pull")
		if out, err := c.runCompose(ctx, args); err != nil {
			return out, err
		}
		args = append(append([]string{}, c.compose[1:]...), "--project-name", "mazima-"+id, "--project-directory", dir, "-f", filepath.Join(dir, "compose.yaml"))
	}
	args = append(args, "up", "-d", "--remove-orphans")
	return c.runCompose(ctx, args)
}

func (c *Client) ComposeDown(ctx context.Context, id string, volumes bool) (string, error) {
	if !c.ComposeAvailable() {
		return "", errors.New("Docker Compose v2 is not installed")
	}
	dir := c.appDir(id)
	args := append(append([]string{}, c.compose[1:]...), "--project-name", "mazima-"+id, "--project-directory", dir, "-f", filepath.Join(dir, "compose.yaml"), "down")
	if volumes {
		args = append(args, "--volumes")
	}
	return c.runCompose(ctx, args)
}

func (c *Client) ComposeLogs(ctx context.Context, id string, tail int) (string, error) {
	if !c.ComposeAvailable() {
		return "", errors.New("Docker Compose v2 is not installed")
	}
	if tail <= 0 || tail > 5000 {
		tail = 200
	}
	dir := c.appDir(id)
	args := append(append([]string{}, c.compose[1:]...), "--project-name", "mazima-"+id, "--project-directory", dir, "-f", filepath.Join(dir, "compose.yaml"), "logs", "--no-color", "--tail", strconv.Itoa(tail))
	return c.runCompose(ctx, args)
}

func (c *Client) runCompose(ctx context.Context, args []string) (string, error) {
	cmd := exec.CommandContext(ctx, c.compose[0], args...)
	cmd.Env = append(os.Environ(), "DOCKER_HOST="+c.host)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("compose: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func (c *Client) appDir(id string) string {
	return filepath.Join(c.appsDir, id)
}

func (c *Client) SetAppsDir(dir string) { c.appsDir = dir }
