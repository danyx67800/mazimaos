package api

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/danyx67800/mazimaos/daemon/internal/store"
)

var accentRE = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (s *Server) currentSettings() (store.Settings, error) {
	st, err := s.Store.Snapshot()
	if err != nil {
		return store.Settings{}, err
	}
	return st.Settings, nil
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.currentSettings()
	if err != nil {
		writeError(w, apiError{500, "settings_unavailable", err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"settings": settings})
}

type settingsPatch struct {
	Hostname       *string `json:"hostname"`
	Timezone       *string `json:"timezone"`
	Language       *string `json:"language"`
	Layout         *string `json:"layout"`
	Theme          *string `json:"theme"`
	Accent         *string `json:"accent"`
	Transparency   *int    `json:"transparency"`
	Wallpaper      *string `json:"wallpaper"`
	UIScale        *int    `json:"ui_scale"`
	ReduceMotion   *bool   `json:"reduce_motion"`
	UpdateChannel  *string `json:"update_channel"`
	AutoUpdate     *bool   `json:"auto_update"`
	SkippedVersion *string `json:"skipped_version"`
	RemoteAccess   *bool   `json:"remote_access"`
	FunnelEnabled  *bool   `json:"funnel_enabled"`
	TOTPRequired   *bool   `json:"totp_required"`
	SSOEnabled     *bool   `json:"sso_enabled"`
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	admin := s.requireAdmin(w, r)
	if admin == nil {
		return
	}
	var req settingsPatch
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, err)
		return
	}
	var out store.Settings
	err := s.Store.Update(func(st *store.State) error {
		v := &st.Settings
		if req.Hostname != nil {
			h := strings.ToLower(strings.TrimSpace(*req.Hostname))
			if !hostnameRE.MatchString(h) || len(h) > 63 {
				return errors.New("invalid hostname")
			}
			v.Hostname = h
		}
		if req.Timezone != nil {
			tz := strings.TrimSpace(*req.Timezone)
			if tz == "" || len(tz) > 64 {
				return errors.New("invalid timezone")
			}
			v.Timezone = tz
		}
		if req.Language != nil {
			if *req.Language != "it" && *req.Language != "en" {
				return errors.New("language must be it or en")
			}
			v.Language = *req.Language
		}
		if req.Layout != nil {
			if *req.Layout != "windows" && *req.Layout != "tablet" && *req.Layout != "compact" {
				return errors.New("invalid layout")
			}
			v.Layout = *req.Layout
		}
		if req.Theme != nil {
			switch *req.Theme {
			case "auto", "light", "dark", "amoled":
			default:
				return errors.New("invalid theme")
			}
			v.Theme = *req.Theme
		}
		if req.Accent != nil {
			if !accentRE.MatchString(*req.Accent) {
				return errors.New("accent must be a six-digit hex color")
			}
			v.Accent = *req.Accent
		}
		if req.Transparency != nil {
			if *req.Transparency < 0 || *req.Transparency > 100 {
				return errors.New("transparency must be between 0 and 100")
			}
			v.Transparency = *req.Transparency
		}
		if req.Wallpaper != nil {
			if len(*req.Wallpaper) > 128 {
				return errors.New("wallpaper name is too long")
			}
			v.Wallpaper = *req.Wallpaper
		}
		if req.UIScale != nil {
			if *req.UIScale < 80 || *req.UIScale > 150 {
				return errors.New("UI scale must be between 80 and 150")
			}
			v.UIScale = *req.UIScale
		}
		if req.ReduceMotion != nil {
			v.ReduceMotion = *req.ReduceMotion
		}
		if req.UpdateChannel != nil {
			switch *req.UpdateChannel {
			case "stable", "beta", "nightly":
			default:
				return errors.New("invalid update channel")
			}
			v.UpdateChannel = *req.UpdateChannel
			if s.Updater != nil {
				s.Updater.Channel = *req.UpdateChannel
			}
		}
		if req.AutoUpdate != nil {
			v.AutoUpdate = *req.AutoUpdate
		}
		if req.SkippedVersion != nil {
			v.SkippedVersion = strings.TrimSpace(*req.SkippedVersion)
		}
		if req.RemoteAccess != nil {
			v.RemoteAccess = *req.RemoteAccess
		}
		if req.FunnelEnabled != nil && *req.FunnelEnabled {
			return errors.New("enable Funnel through the Tailscale Funnel action")
		}
		if req.TOTPRequired != nil {
			return errors.New("the global 2FA policy is not configurable in this release")
		}
		if req.SSOEnabled != nil && *req.SSOEnabled {
			return errors.New("SSO is not available")
		}
		out = *v
		return nil
	})
	if err != nil {
		writeError(w, apiError{400, "settings_invalid", err.Error()})
		return
	}
	s.record(r, "", "settings.update", "system", "ok", nil)
	writeJSON(w, 200, map[string]any{"settings": out})
}

func (s *Server) handleDesktopGet(w http.ResponseWriter, r *http.Request) {
	ac := s.authFor(r)
	st, err := s.Store.Snapshot()
	if err != nil {
		writeError(w, apiError{500, "state_unavailable", err.Error()})
		return
	}
	d, ok := st.Desktops[ac.User.Username]
	if !ok {
		d = store.DesktopState{VirtualDesktops: 2, ActiveDesktop: 0}
	}
	writeJSON(w, 200, map[string]any{"desktop": d})
}

func (s *Server) handleDesktopPut(w http.ResponseWriter, r *http.Request) {
	ac := s.authFor(r)
	var d store.DesktopState
	if err := decodeJSON(w, r, &d); err != nil {
		writeError(w, err)
		return
	}
	if len(d.Windows) > 80 {
		writeError(w, apiError{400, "desktop_invalid", "too many windows"})
		return
	}
	if d.VirtualDesktops < 1 || d.VirtualDesktops > 6 || d.ActiveDesktop < 0 || d.ActiveDesktop >= d.VirtualDesktops {
		writeError(w, apiError{400, "desktop_invalid", "invalid virtual desktop configuration"})
		return
	}
	for i := range d.Windows {
		win := &d.Windows[i]
		if win.ID == "" || len(win.ID) > 64 || win.AppID == "" || len(win.AppID) > 64 {
			writeError(w, apiError{400, "desktop_invalid", "invalid window identifier"})
			return
		}
		if win.Width < 240 || win.Height < 160 || win.Width > 10000 || win.Height > 10000 ||
			win.X < -5000 || win.Y < -5000 || win.X > 20000 || win.Y > 20000 {
			writeError(w, apiError{400, "desktop_invalid", "window geometry is out of range"})
			return
		}
		if win.Desktop < 0 || win.Desktop >= d.VirtualDesktops {
			win.Desktop = 0
		}
	}
	err := s.Store.Update(func(st *store.State) error {
		d.UpdatedAt = s.now()
		st.Desktops[ac.User.Username] = d
		return nil
	})
	if err != nil {
		writeError(w, apiError{500, "desktop_save_failed", err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "updated_at": d.UpdatedAt})
}
