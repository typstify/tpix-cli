package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	appName        = "tpix-cli"
	configFilename = "settings.json"
	cachePathEnv   = "TYPST_PACKAGE_CACHE_PATH"
)

type Credentials struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken,omitempty"`
}

type Config struct {
	Credentials
	TypstCachePkgPath string `json:"typstCachePkgPath"`
}

var (
	configDir string
)

func init() {
	dir, err := getConfigDir()
	if err != nil {
		fmt.Println("Get config dir error: ", err)
		return
	}

	configDir = dir
}

func Load() (Config, error) {
	path := filepath.Join(configDir, configFilename)

	configFile, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return Config{}, err
	}

	defer configFile.Close()

	var appConfig Config

	err = json.NewDecoder(configFile).Decode(&appConfig)
	if err != nil && err.Error() != "EOF" {
		return Config{}, err
	}

	// If user provided a env variable, use it instead of the one in the config file
	envPath := os.Getenv(cachePathEnv)
	if envPath != "" {
		info, err := os.Stat(envPath)
		if err != nil {
			return appConfig, fmt.Errorf("Invalid path for %s: %s", cachePathEnv, envPath)
		}
		if !info.IsDir() {
			return appConfig, fmt.Errorf("Path is not a directory: %s", envPath)
		}

		absPath, err := filepath.Abs(envPath)
		if err != nil {
			return appConfig, err
		}

		appConfig.TypstCachePkgPath = absPath
		return appConfig, nil
	}

	// No env is set, try to use a detected cache path.
	if appConfig.TypstCachePkgPath == "" {
		appConfig.TypstCachePkgPath = defaultCacheDir()
	}

	return appConfig, nil

}

func Save(cfg Config) error {
	path := filepath.Join(configDir, configFilename)
	configFile, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	defer configFile.Close()

	if cfg.TypstCachePkgPath == "" {
		cfg.TypstCachePkgPath = defaultCacheDir()
	}

	err = json.NewEncoder(configFile).Encode(&cfg)
	if err != nil {
		return err
	}

	return nil
}

func getConfigDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	configPath := filepath.Join(configDir, appName)

	_, err = os.Stat(configPath)
	if os.IsNotExist(err) {
		err = os.MkdirAll(configPath, 0755)
		if err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}

	return configPath, nil
}

// defaultCacheDir returns the default typst package cache dir, according to
// https://github.com/typst/packages/blob/main/README.md.
func defaultCacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}

	cacheDir := filepath.Join(dir, "typst", "packages")

	_, err = os.Stat(cacheDir)
	if os.IsNotExist(err) {
		err = os.MkdirAll(cacheDir, 0755)
		if err != nil {
			fmt.Printf("Creating default cache dir failed: %v", err)
		}
	}

	return cacheDir
}

type CliCredentialProvider struct{}

func (c CliCredentialProvider) Load() (Credentials, error) {
	cfg, err := Load()
	if err != nil {
		return Credentials{}, err
	}

	return cfg.Credentials, err
}

func (c CliCredentialProvider) Save(cred Credentials) error {
	cfg, err := Load()
	if err != nil {
		return err
	}

	cfg.Credentials = cred

	return Save(cfg)
}
