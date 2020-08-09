package entity

import (
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

type HostStatus struct {
	Online      bool      `json:"online"`
	AgentOnline bool      `json:"agent_online"`
	ActiveUser  string    `json:"active_user"`
	UpdatedAt   time.Time `json:"updated_at"`
	Error       string    `json:"error"`
}

type Host struct {
	Name            string            `yaml:"name"`
	OnlineCheckPort int               `yaml:"online_check_port"`
	AgentHost       string            `yaml:"agent_host"`
	AgentPort       int               `yaml:"agent_port"`
	CameraID        int               `yaml:"camera_id"`
	Users           map[string]string `yaml:"users"`
}

func (h Host) CheckOnline(timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", h.Name, h.OnlineCheckPort), timeout)
	if err != nil {
		return false
	}

	conn.Close()

	return true
}

func (h Host) CheckAgentOnline(timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", h.Name, h.AgentPort), timeout)
	if err != nil {
		return false
	}

	conn.Close()

	return true
}

func (h Host) Status() (io.ReadCloser, string, error) {
	res, err := http.Get(fmt.Sprintf("http://%s:%d/status", h.Name, h.AgentPort))
	if err != nil {
		return nil, "", err
	}

	userNameBytes, err := base64.StdEncoding.DecodeString(res.Header.Get("X-Active-User"))
	if err != nil {
		return nil, "", fmt.Errorf("decode user name: %w", err)
	}

	userName := string(userNameBytes)

	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, userName, fmt.Errorf("not 200 status code: %d", res.StatusCode)
	}

	return res.Body, userName, nil
}

func (h Host) LogoutCurrentUser() error {
	res, err := http.Post(fmt.Sprintf("http://%s:%d/logout", h.Name, h.AgentPort), "", nil)
	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("not 200 status code: %d", res.StatusCode)
	}

	return nil
}
