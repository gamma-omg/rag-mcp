package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	LogFile       string `yaml:"log"`
	DocRoot       string `yaml:"doc_root"`
	MergeEventsMs int    `yaml:"write_debounce_ms"`
	ChunkSize     int    `yaml:"chunk_size"`
	ChunkOverlap  int    `yaml:"chunk_overlap"`
	RequestSize   int    `yaml:"request_size"`
	Results       int    `yaml:"results"`
	ServerAddr    string `yaml:"server_addr"`
	OpenAI        *struct {
		Model  string `yaml:"model"`
		ApiKey string `yaml:"api_key"`
	} `yaml:"open_ai"`
	Gemini *struct {
		Model  string `yaml:"model"`
		ApiKey string `yaml:"api_key"`
	}
}

func readConfig(cfgPath string) (*Config, error) {
	cfgFile, err := os.Open(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("unable to open config file: %w", err)
	}
	defer cfgFile.Close()

	cfg := &Config{}
	dec := yaml.NewDecoder(cfgFile)
	err = dec.Decode(cfg)
	if err != nil {
		return nil, fmt.Errorf("unable to parse config file: %w", err)
	}

	return cfg, nil
}
