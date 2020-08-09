package entity

type AgentConfig struct {
	Host     string `yaml:"host" json:"host"`
	Port     int    `yaml:"port" json:"port"`
	CameraID int    `yaml:"camera_id" json:"camera_id"`
}
