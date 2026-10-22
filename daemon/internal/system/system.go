package system

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/shirou/gopsutil/v4/sensors"
)

type Disk struct {
	Device     string  `json:"device"`
	Mountpoint string  `json:"mountpoint"`
	Fstype     string  `json:"fstype"`
	Total      uint64  `json:"total"`
	Used       uint64  `json:"used"`
	Free       uint64  `json:"free"`
	UsedPct    float64 `json:"used_percent"`
}

type NetInterface struct {
	Name      string   `json:"name"`
	Addresses []string `json:"addresses"`
	RXBytes   uint64   `json:"rx_bytes"`
	TXBytes   uint64   `json:"tx_bytes"`
	State     string   `json:"state,omitempty"`
}

type Temperature struct {
	Sensor string  `json:"sensor"`
	Value  float64 `json:"celsius"`
	High   float64 `json:"high,omitempty"`
}

type GPU struct {
	Name     string   `json:"name"`
	Driver   string   `json:"driver,omitempty"`
	Render   string   `json:"render_node,omitempty"`
	Encoders []string `json:"encoders,omitempty"`
}

type Snapshot struct {
	Time         time.Time      `json:"time"`
	Hostname     string         `json:"hostname"`
	OS           string         `json:"os"`
	Kernel       string         `json:"kernel"`
	Arch         string         `json:"arch"`
	UptimeSec    uint64         `json:"uptime_seconds"`
	Load         [3]float64     `json:"load"`
	CPUPercent   float64        `json:"cpu_percent"`
	CPUCores     int            `json:"cpu_cores"`
	Memory       Memory         `json:"memory"`
	Swap         Memory         `json:"swap"`
	Disks        []Disk         `json:"disks"`
	Network      []NetInterface `json:"network"`
	Temperatures []Temperature  `json:"temperatures"`
	GPUs         []GPU          `json:"gpus"`
	Timezone     string         `json:"timezone"`
	NTP          bool           `json:"ntp"`
	ClockSynced  bool           `json:"clock_synced"`
}

type Memory struct {
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Available   uint64  `json:"available"`
	UsedPercent float64 `json:"used_percent"`
}

type Process struct {
	PID     int32   `json:"pid"`
	Name    string  `json:"name"`
	User    string  `json:"user,omitempty"`
	CPU     float64 `json:"cpu_percent"`
	Memory  float32 `json:"memory_percent"`
	RSS     uint64  `json:"rss"`
	Command string  `json:"command,omitempty"`
}

type Service struct {
	Name        string `json:"name"`
	Load        string `json:"load,omitempty"`
	Active      string `json:"active"`
	Sub         string `json:"sub,omitempty"`
	Description string `json:"description,omitempty"`
}

type Collector struct {
	mu      sync.Mutex
	history []Snapshot
	max     int
	now     func() time.Time
	lastErr error
}

func NewCollector() *Collector {
	return &Collector{max: 288, now: time.Now}
}

func (c *Collector) Start(ctx context.Context) {
	go func() {
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		_, _ = c.Snapshot()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_, _ = c.Snapshot()
			}
		}
	}()
}

func (c *Collector) Snapshot() (Snapshot, error) {
	s, err := collect(c.now)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.lastErr = err
	}
	c.history = append(c.history, s)
	if len(c.history) > c.max {
		c.history = c.history[len(c.history)-c.max:]
	}
	return s, err
}

func (c *Collector) History() []Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Snapshot(nil), c.history...)
}

func collect(now func() time.Time) (Snapshot, error) {
	s := Snapshot{Time: now().UTC(), Arch: runtime.GOARCH}
	s.Hostname, _ = os.Hostname()
	if info, err := host.Info(); err == nil {
		s.OS = info.Platform + " " + info.PlatformVersion
		s.Kernel = info.KernelVersion
		s.UptimeSec = info.Uptime
	}
	if avg, err := load.Avg(); err == nil {
		s.Load = [3]float64{avg.Load1, avg.Load5, avg.Load15}
	}
	if pct, err := cpu.Percent(0, false); err == nil && len(pct) > 0 {
		s.CPUPercent = round1(pct[0])
	}
	s.CPUCores = runtime.NumCPU()
	if vm, err := mem.VirtualMemory(); err == nil {
		s.Memory = Memory{Total: vm.Total, Used: vm.Used, Available: vm.Available, UsedPercent: round1(vm.UsedPercent)}
	}
	if sw, err := mem.SwapMemory(); err == nil {
		s.Swap = Memory{Total: sw.Total, Used: sw.Used, Available: sw.Free, UsedPercent: round1(sw.UsedPercent)}
	}
	s.Disks = disks()
	s.Network = network()
	if temps, err := sensors.SensorsTemperatures(); err == nil {
		for _, t := range temps {
			if t.Temperature <= -100 || t.Temperature > 200 {
				continue
			}
			s.Temperatures = append(s.Temperatures, Temperature{Sensor: t.SensorKey, Value: round1(t.Temperature), High: round1(t.High)})
		}
		sort.Slice(s.Temperatures, func(i, j int) bool { return s.Temperatures[i].Sensor < s.Temperatures[j].Sensor })
	}
	s.GPUs = gpus()
	s.Timezone = localTimezone()
	s.NTP, s.ClockSynced = clockStatus()
	return s, nil
}

func disks() []Disk {
	parts, err := disk.Partitions(false)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []Disk
	for _, p := range parts {
		if skipFilesystem(p.Fstype) || skipMount(p.Mountpoint) {
			continue
		}
		key := p.Device + "|" + p.Mountpoint
		if seen[key] {
			continue
		}
		seen[key] = true
		u, err := disk.Usage(p.Mountpoint)
		if err != nil || u.Total == 0 {
			continue
		}
		out = append(out, Disk{
			Device: p.Device, Mountpoint: p.Mountpoint, Fstype: p.Fstype,
			Total: u.Total, Used: u.Used, Free: u.Free, UsedPct: round1(u.UsedPercent),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mountpoint < out[j].Mountpoint })
	return out
}

func skipFilesystem(fs string) bool {
	switch fs {
	case "squashfs", "tmpfs", "devtmpfs", "proc", "sysfs", "cgroup", "cgroup2",
		"overlay", "aufs", "ramfs", "devpts", "mqueue", "tracefs", "debugfs",
		"pstore", "bpf", "autofs", "fusectl", "configfs", "securityfs", "nsfs":
		return true
	}
	return false
}

func skipMount(p string) bool {
	return strings.HasPrefix(p, "/snap/") || strings.HasPrefix(p, "/var/lib/docker/") ||
		strings.HasPrefix(p, "/run/") || strings.HasPrefix(p, "/sys/") ||
		strings.HasPrefix(p, "/proc/") || strings.HasPrefix(p, "/dev/")
}

func network() []NetInterface {
	io, _ := gnet.IOCounters(true)
	byName := map[string]gnet.IOCountersStat{}
	for _, x := range io {
		byName[x.Name] = x
	}
	ifaces, err := gnet.Interfaces()
	if err != nil {
		return nil
	}
	var out []NetInterface
	for _, iface := range ifaces {
		if iface.Name == "lo" || iface.Name == "lo0" {
			continue
		}
		n := NetInterface{Name: iface.Name, State: strings.Join(iface.Flags, ",")}
		for _, a := range iface.Addrs {
			n.Addresses = append(n.Addresses, a.Addr)
		}
		if st, ok := byName[iface.Name]; ok {
			n.RXBytes = st.BytesRecv
			n.TXBytes = st.BytesSent
		}
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func gpus() []GPU {
	var out []GPU
	if entries, err := os.ReadDir("/dev/dri"); err == nil {
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "renderD") {
				out = append(out, GPU{Name: "DRM render node", Render: "/dev/dri/" + e.Name()})
			}
		}
	}
	if _, err := exec.LookPath("nvidia-smi"); err == nil {
		if b, err := exec.Command("nvidia-smi", "--query-gpu=name,driver_version", "--format=csv,noheader").Output(); err == nil {
			for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
				parts := strings.Split(line, ", ")
				if len(parts) >= 2 {
					out = append(out, GPU{Name: parts[0], Driver: "nvidia " + parts[1]})
				}
			}
		}
	}
	if len(out) == 0 {
		out = append(out, GPU{Name: "Software rendering", Driver: "CPU"})
	}
	return out
}

func localTimezone() string {
	if b, err := os.ReadFile("/etc/timezone"); err == nil {
		return strings.TrimSpace(string(b))
	}
	if b, err := exec.Command("date", "+%Z").Output(); err == nil {
		return strings.TrimSpace(string(b))
	}
	return ""
}

func clockStatus() (ntp, synced bool) {
	if runtime.GOOS != "linux" {
		return false, false
	}
	if b, err := exec.Command("timedatectl", "show", "--property=NTP", "--property=NTPSynchronized", "--value").Output(); err == nil {
		lines := strings.Fields(string(b))
		if len(lines) >= 2 {
			return lines[0] == "yes", lines[1] == "yes"
		}
	}
	return false, false
}

func (c *Collector) TopProcesses(limit int) ([]Process, error) {
	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 15
	}
	var out []Process
	for _, p := range procs {
		name, _ := p.Name()
		if name == "" {
			continue
		}
		cpuPct, _ := p.CPUPercent()
		memPct, _ := p.MemoryPercent()
		user, _ := p.Username()
		cmd, _ := p.Cmdline()
		rss, _ := p.MemoryInfo()
		var rssVal uint64
		if rss != nil {
			rssVal = rss.RSS
		}
		out = append(out, Process{
			PID: p.Pid, Name: name, User: user, CPU: round1(cpuPct), Memory: float32(round1(float64(memPct))),
			RSS: rssVal, Command: truncate(cmd, 200),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CPU == out[j].CPU {
			return out[i].RSS > out[j].RSS
		}
		return out[i].CPU > out[j].CPU
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func Services() ([]Service, error) {
	if runtime.GOOS != "linux" {
		return []Service{}, nil
	}
	b, err := exec.Command("systemctl", "list-units", "--type=service", "--all", "--no-legend", "--plain", "--no-pager").Output()
	if err != nil {
		return nil, err
	}
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	var out []Service
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 0 {
			continue
		}
		s := Service{Name: fields[0]}
		if len(fields) > 1 {
			s.Load = fields[1]
		}
		if len(fields) > 2 {
			s.Active = fields[2]
		}
		if len(fields) > 3 {
			s.Sub = fields[3]
		}
		if len(fields) > 4 {
			s.Description = strings.Join(fields[4:], " ")
		}
		out = append(out, s)
	}
	return out, nil
}

func Logs(lines int) (string, error) {
	return LogsUnit("", lines)
}

func LogsUnit(unit string, lines int) (string, error) {
	if runtime.GOOS != "linux" {
		return "", errors.New("journal logs are only available on Linux")
	}
	if lines <= 0 || lines > 2000 {
		lines = 200
	}
	args := []string{"--no-pager", "-n", strconv.Itoa(lines), "--output=short-iso"}
	if unit != "" {
		if !serviceNameRE.MatchString(unit) {
			return "", errors.New("invalid service name")
		}
		args = append(args, "-u", unit)
	}
	b, err := exec.Command("journalctl", args...).CombinedOutput()
	if err != nil {
		return string(b), fmt.Errorf("journalctl: %w", err)
	}
	return string(b), nil
}

var serviceNameRE = regexp.MustCompile(`^[A-Za-z0-9@_.:-]{1,128}\.service$`)

func ServiceAction(name, action string) error {
	if runtime.GOOS != "linux" {
		return errors.New("service operations are only available on Linux")
	}
	if !serviceNameRE.MatchString(name) {
		return errors.New("invalid service name")
	}
	switch action {
	case "start", "stop", "restart", "reload", "enable", "disable":
	default:
		return errors.New("unsupported service action")
	}
	out, err := exec.Command("systemctl", action, name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s: %w: %s", action, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func SignalProcess(pid int, sig string) error {
	if runtime.GOOS != "linux" {
		return errors.New("process signals are only available on Linux")
	}
	if pid <= 1 || pid == os.Getpid() {
		return errors.New("refusing to signal this process")
	}
	var s syscall.Signal
	switch sig {
	case "TERM":
		s = syscall.SIGTERM
	case "KILL":
		s = syscall.SIGKILL
	default:
		return errors.New("signal must be TERM or KILL")
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(s)
}

func Power(action string) error {
	if runtime.GOOS != "linux" {
		return errors.New("power operations are only available on Linux")
	}
	var args []string
	switch action {
	case "reboot":
		args = []string{"reboot"}
	case "poweroff":
		args = []string{"poweroff"}
	default:
		return errors.New("unsupported power action")
	}
	if err := exec.Command("systemctl", args...).Run(); err != nil {
		return err
	}
	return nil
}

func round1(f float64) float64 {
	return float64(int(f*10+0.5)) / 10
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
