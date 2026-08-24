package config

type Credentials struct {
	ApiKey string `json:"apiKey"`
}

type Config struct {
	Credentials
	TypstCachePkgPath string `json:"typstCachePkgPath"`
}

type ConfigManager interface {
	Load() (Config, error)
	Save(cfg Config) error
}
