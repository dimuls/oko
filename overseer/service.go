// +build windows

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"gopkg.in/tucnak/telebot.v2"
	"gopkg.in/yaml.v2"

	"github.com/dimuls/oko/entity"
	"github.com/dimuls/oko/face"
	"github.com/dimuls/oko/overseer/web"
)

const (
	hostConfigFileName = "host.conf"
)

var eLog debug.Log

type serviceConfigRaw struct {
	HostsConfigsDirectoryPath string `yaml:"hosts_configs_directory_path"`

	DefaultOnlineCheckPort int    `yaml:"default_online_check_port"`
	DefaultAgentHost       string `yaml:"default_agent_host"`
	DefaultAgentPort       int    `yaml:"default_agent_port"`
	DefaultCameraID        int    `yaml:"default_camera_id"`

	ProcessPeriod           string `yaml:"process_period"`
	ProcessConcurrency      int    `yaml:"process_concurrency"`
	CheckOnlineTimeout      string `yaml:"check_online_timeout"`
	CheckAgentOnlineTimeout string `yaml:"check_agent_online_timeout"`

	TelegramBotToken                string `yaml:"telegram_bot_token"`
	TelegramNotificationsRecipients []int  `yaml:"telegram_notifications_recipients"`

	FaceAPIConfig face.APIConfig   `yaml:"face_api"`
	WebServer     web.ServerConfig `yaml:"web_server"`
}

type serviceConfig struct {
	serviceConfigRaw
	ProcessPeriod           time.Duration
	CheckOnlineTimeout      time.Duration
	CheckAgentOnlineTimeout time.Duration
}

func (c *serviceConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var cRaw serviceConfigRaw

	err := unmarshal(&cRaw)
	if err != nil {
		return err
	}

	c.serviceConfigRaw = cRaw

	c.ProcessPeriod, err = time.ParseDuration(cRaw.ProcessPeriod)
	if err != nil {
		return fmt.Errorf("parse process_duration: %w", err)
	}

	c.CheckOnlineTimeout, err = time.ParseDuration(cRaw.CheckAgentOnlineTimeout)
	if err != nil {
		return fmt.Errorf("parse process_duration: %w", err)
	}

	c.CheckAgentOnlineTimeout, err = time.ParseDuration(cRaw.CheckAgentOnlineTimeout)
	if err != nil {
		return fmt.Errorf("parse process_duration: %w", err)
	}

	return nil
}

type service struct {
	config serviceConfig

	hosts   map[string]entity.Host
	hostsMx sync.RWMutex

	hostsStatuses   map[string]entity.HostStatus
	hostsStatusesMx sync.RWMutex

	tbBot         *telebot.Bot
	faceAPI       *face.API
	notifications chan string
}

func (s *service) AgentConfig(hostName string) (entity.AgentConfig, error) {
	s.hostsMx.Lock()
	defer s.hostsMx.Unlock()

	h, exists := s.hosts[hostName]
	if !exists {
		hostConfigsDirPath := path.Join(s.config.HostsConfigsDirectoryPath, hostName)
		hostConfigFilePath := path.Join(hostConfigsDirPath, hostConfigFileName)

		_, err := os.Stat(hostConfigsDirPath)
		if err != nil {
			if os.IsNotExist(err) {
				err := os.MkdirAll(hostConfigsDirPath, 0775)
				if err != nil {
					return entity.AgentConfig{}, fmt.Errorf("create host configs directory path: %w", err)
				}
			}
		}

		f, err := os.Create(hostConfigFilePath)
		if err != nil {
			return entity.AgentConfig{}, fmt.Errorf("open host config file: %w", err)
		}

		h = entity.Host{
			Name:            hostName,
			OnlineCheckPort: s.config.DefaultOnlineCheckPort,
			AgentHost:       s.config.DefaultAgentHost,
			AgentPort:       s.config.DefaultAgentPort,
			CameraID:        s.config.DefaultCameraID,
			Users:           nil,
		}

		err = yaml.NewEncoder(f).Encode(&h)
		if err != nil {
			return entity.AgentConfig{}, fmt.Errorf("YAML decode host config")
		}

		err = f.Close()
		if err != nil {
			return entity.AgentConfig{}, fmt.Errorf("close host config")
		}

		s.hosts[hostName] = h
	}

	return entity.AgentConfig{
		Host:     h.AgentHost,
		Port:     h.AgentPort,
		CameraID: h.CameraID,
	}, nil
}

func (s *service) Hosts() []entity.Host {
	s.hostsMx.RLock()
	defer s.hostsMx.RUnlock()

	var hs []entity.Host

	for _, h := range s.hosts {
		hs = append(hs, h)
	}

	return hs
}

func (s *service) HostsStatuses() []entity.HostStatus {
	s.hostsStatusesMx.RLock()
	defer s.hostsStatusesMx.RUnlock()

	var hss []entity.HostStatus

	for _, hs := range s.hostsStatuses {
		hss = append(hss, hs)
	}

	return hss
}

func (s *service) loadHosts() error {
	fis, err := ioutil.ReadDir(path.Join(s.config.HostsConfigsDirectoryPath))
	if err != nil {
		return fmt.Errorf("read hosts configs directory: %w", err)
	}

	hosts := map[string]entity.Host{}

	for _, fi := range fis {
		if !fi.IsDir() {
			continue
		}

		hostConfigFilePath := path.Join(s.config.HostsConfigsDirectoryPath, fi.Name(), hostConfigFileName)

		f, err := os.Open(hostConfigFilePath)
		if err != nil {
			return fmt.Errorf("open host config file: %w", err)
		}

		var h entity.Host

		err = yaml.NewDecoder(f).Decode(&h)
		if err != nil {
			return fmt.Errorf("YAML decode host config file: %w", err)
		}

		hosts[h.Name] = h
	}

	s.hostsMx.Lock()
	s.hosts = hosts
	s.hostsMx.Unlock()

	return nil
}

func (s *service) notify(notification string) {
	for _, r := range s.config.TelegramNotificationsRecipients {
		_, err := s.tbBot.Send(&telebot.User{ID: r}, notification)
		if err != nil {
			eLog.Error(1, fmt.Sprintf("не удалось отправить телеграм оповещеение: %v", err))
		}
	}
}

func (s *service) loadConfig(exePath string) error {
	f, err := os.Open(path.Join(exePath, "overseer.conf"))
	if err != nil {
		return err
	}

	return yaml.NewDecoder(f).Decode(&s.config)
}

func (s *service) Execute(args []string, changeRequests <-chan svc.ChangeRequest, statusChanges chan<- svc.Status) (ssec bool, errno uint32) {

	statusChanges <- svc.Status{State: svc.StartPending}

	exe, err := os.Executable()
	if err != nil {
		errno = 1
		return
	}

	exePath := filepath.Dir(exe)

	err = s.loadConfig(exePath)
	if err != nil {
		eLog.Error(1, fmt.Sprintf("не удалось загрузить конфиг: %v", err))
		errno = 2
		return
	}

	if !path.IsAbs(s.config.HostsConfigsDirectoryPath) {
		s.config.HostsConfigsDirectoryPath = path.Join(exePath, s.config.HostsConfigsDirectoryPath)
	}

	err = s.loadHosts()
	if err != nil {
		eLog.Error(1, fmt.Sprintf("не удалось загрузить хосты: %v", err))
		errno = 3
		return
	}

	s.hostsStatuses = map[string]entity.HostStatus{}

	s.tbBot, err = telebot.NewBot(telebot.Settings{
		Token: s.config.TelegramBotToken,
		Poller: &telebot.LongPoller{
			Timeout: 10 * time.Second,
		},
	})

	s.tbBot.Handle(telebot.OnText, func(m *telebot.Message) {
		s.tbBot.Reply(m, fmt.Sprintf("Ваш ID пользователя %d.", m.Sender.ID))
	})

	s.faceAPI = face.NewAPI(s.config.FaceAPIConfig)

	s.notifications = make(chan string)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for n := range s.notifications {
			s.notify(n)
		}
	}()

	ws, err := web.NewServer(s.config.WebServer, s, s)
	if err != nil {
		eLog.Error(1, fmt.Sprintf("не удалось запустить веб-сервер: %v", err))
		errno = 4
		return
	}

	ticker := time.NewTicker(s.config.ProcessPeriod)
	defer ticker.Stop()

	statusChanges <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

loop:
	for {
		select {
		case <-ticker.C:
			s.process()
		case cr := <-changeRequests:
			switch cr.Cmd {
			case svc.Interrogate:
				statusChanges <- cr.CurrentStatus
			case svc.Stop, svc.Shutdown:
				eLog.Info(1, "получен запрос остановки, останавливаю")
				break loop
			default:
				eLog.Error(1, fmt.Sprintf("неизвестный управляющий запрос #%d", cr))
			}
		}
	}

	statusChanges <- svc.Status{State: svc.StopPending}

	err = ws.Close()
	if err != nil {
		eLog.Error(1, fmt.Sprintf("не удалось остановить веб-сервер: %v", err))
	}

	close(s.notifications)

	wg.Wait()

	statusChanges <- svc.Status{State: svc.Stopped}

	return
}

func (s *service) processHost(h entity.Host) {
	var (
		online      bool
		agentOnline bool
		cameraFrame io.ReadCloser
		activeUser  string
		err         error
	)

	defer func() {
		s.hostsStatusesMx.Lock()
		defer s.hostsStatusesMx.Unlock()

		var errMsg string
		if err != nil {
			errMsg = err.Error()
		}

		s.hostsStatuses[h.Name] = entity.HostStatus{
			Online:      online,
			AgentOnline: agentOnline,
			ActiveUser:  activeUser,
			UpdatedAt:   time.Now(),
			Error:       errMsg,
		}
	}()

	online = h.CheckOnline(s.config.CheckOnlineTimeout)
	if !online {
		eLog.Info(1, fmt.Sprintf("[имя_хоста=%s] хост выключен", h.Name))
		return
	}

	agentOnline = h.CheckAgentOnline(s.config.CheckAgentOnlineTimeout)
	if !agentOnline {
		eLog.Info(1, fmt.Sprintf("[имя_хоста=%s] агент выключен", h.Name))
		s.notifications <- fmt.Sprintf("[имя_хоста=%s] агент выключен", h.Name)
		return
	}

	cameraFrame, activeUser, err = h.Status()

	if cameraFrame != nil {
		defer cameraFrame.Close()
	}

	if err != nil {
		eLog.Error(1, fmt.Sprintf("[имя_хоста=%s] не удалось получить статус агента: %v", h.Name, err))
		s.notifications <- fmt.Sprintf("[имя_хоста=%s] не удалось получить статус агента", h.Name)
		return
	}

	if activeUser == "" {
		eLog.Error(1, fmt.Sprintf("[имя_хоста=%s] нет залогиненного пользователя", h.Name))
		return
	}

	recognizedUserID, err := s.faceAPI.RecognizeUser(cameraFrame)
	if err != nil {
		if err == face.ErrFaceNotFound {
			eLog.Info(1, fmt.Sprintf("[имя_хоста=%s, имя_пользователя=%s] не удалось распознать лицо пользователя: нет лица в кадре", h.Name, activeUser))
			return
		}
		eLog.Error(1, fmt.Sprintf("[имя_хоста=%s, имя_пользователя=%s] не удалось распознать лицо пользователя: %v", h.Name, activeUser, err))
		s.notifications <- fmt.Sprintf("[имя_хоста=%s, имя_пользователя=%s] не удалось распознать лицо пользователя", h.Name, activeUser)
	}

	if recognizedUserID == "" || h.Users[activeUser] != recognizedUserID {
		eLog.Error(1, fmt.Sprintf("[имя_хоста=%s, имя_пользователя=%s, идентификатор_обнаруженного_пользователя=%s] обнаружен неразрешённый пользователь", h.Name, activeUser, recognizedUserID))
		s.notifications <- fmt.Sprintf("[имя_хоста=%s, имя_пользователя=%s] обнаружен неразрешённый пользователь", h.Name, activeUser)
		err = h.LogoutCurrentUser()
		if err != nil {
			eLog.Error(1, fmt.Sprintf("[имя_хоста=%s, имя_пользователя=%s] не удалось разлогинить неразрешённого пользователя: %v", h.Name, activeUser, err))
			s.notifications <- fmt.Sprintf("[имя_хоста=%s, имя_пользователя=%s] не удалось разлогинить неразрешённого пользователя", h.Name, activeUser)
		}
	}
}

func (s *service) process() {
	var wg sync.WaitGroup
	hosts := make(chan entity.Host)

	err := s.loadHosts()
	if err != nil {
		eLog.Error(1, fmt.Sprintf("failed to load hosts: %v", err))
	}

	for i := 0; i < s.config.ProcessConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for h := range hosts {
				s.processHost(h)
			}
		}()
	}

	s.hostsMx.RLock()
	defer s.hostsMx.RUnlock()

	for _, h := range s.hosts {
		hosts <- h
	}

	close(hosts)
	wg.Wait()
}

func runService(name string, isDebug bool) {
	var err error
	if isDebug {
		eLog = debug.New(name)
	} else {
		eLog, err = eventlog.Open(name)
		if err != nil {
			return
		}
	}
	defer eLog.Close()

	eLog.Info(1, fmt.Sprintf("starting %s service", name))
	run := svc.Run
	if isDebug {
		run = debug.Run
	}
	err = run(name, &service{})
	if err != nil {
		eLog.Error(1, fmt.Sprintf("%s service failed: %v", name, err))
		return
	}
	eLog.Info(1, fmt.Sprintf("%s service stopped", name))
}
