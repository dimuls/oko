package web

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/dimuls/oko/entity"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
)

type AgentConfigProvider interface {
	AgentConfig(hostName string) (entity.AgentConfig, error)
}

type HostProvider interface {
	Hosts() []entity.Host
	HostsStatuses() []entity.HostStatus
}

type ServerConfig struct {
	Address  string `yaml:"address"`
	Login    string `yaml:"login"`
	Password string `yaml:"password"`
}

type Server struct {
	echo                *echo.Echo
	agentConfigProvider AgentConfigProvider
	hostProvider        HostProvider
}

func NewServer(c ServerConfig, acp AgentConfigProvider, hp HostProvider) (*Server, error) {

	e := echo.New()

	e.Use(middleware.Recover())
	e.Use(middleware.Logger())

	e.GET("/hosts/:host_name/agent_config", func(c echo.Context) error {
		hostName := c.Param("host_name")

		ip, _, err := net.SplitHostPort(c.Request().RemoteAddr)
		if err != nil {
			return fmt.Errorf("split host and port from remote address: %w", err)
		}

		remoteHostNames, err := net.LookupAddr(ip)
		if err != nil {
			return fmt.Errorf("lookup remote host name: %w", err)
		}

		for _, rhn := range remoteHostNames {
			if hostName == rhn {
				ac, err := acp.AgentConfig(hostName)
				if err != nil {
					return fmt.Errorf("get agent config: %w", err)
				}

				return c.JSON(http.StatusOK, ac)
			}
		}

		return echo.NewHTTPError(http.StatusNotFound)
	})

	basicAuthentificator := func(login string, password string, e echo.Context) (bool, error) {
		return login == c.Login && password == c.Password, nil
	}

	p := e.Group("", middleware.CORS(), middleware.BasicAuth(basicAuthentificator))

	p.GET("/hosts", func(c echo.Context) error {
		return c.JSON(http.StatusOK, hp.Hosts())
	})

	p.GET("/hosts_statuses", func(c echo.Context) error {
		return c.JSON(http.StatusOK, hp.HostsStatuses())
	})

	failed := make(chan error)

	go func() {
		tries := 0
		for {
			tries++
			err := e.Start(c.Address)
			if err != nil {
				if err == http.ErrServerClosed {
					return
				}
				if tries >= 3 {
					failed <- err
					return
				}
				time.Sleep(time.Second)
			}
		}
	}()

	select {

	case err := <-failed:
		return nil, err

	case <-time.After(5 * time.Second):
		return &Server{
			echo:                e,
			agentConfigProvider: acp,
		}, nil
	}
}

func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.echo.Shutdown(ctx)
}
