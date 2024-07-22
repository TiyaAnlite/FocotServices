package main

import (
	"flag"
	"github.com/TiyaAnlite/FocotServicesCommon/envx"
	"testing"
)

type config struct {
	ConfigFileName string `json:"config_file_name" yaml:"configFileName" env:"CONFIG_FILE_NAME" envDefault:"config.yaml"`
	DataDir        string `json:"data_dir" yaml:"data_dir" env:"DATA_DIR" envDefault:"vid"`
}

var (
	cfg = &config{}
)

func init() {
	testing.Init()
	flag.Parse()
	envx.MustLoadEnv(cfg)
	envx.MustReadYamlConfig(cfg, cfg.ConfigFileName)
}

func main() {

}
