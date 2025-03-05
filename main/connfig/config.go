package config

import (
	"flag"
	"os"
	"strconv"
	"time"

	"github.com/FurmanovVitaliy/logger"
	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	Env          string                    `yaml:"environment" env-default:"prod"`
	Logger       LoggerConfig              `yaml:"logger"`
	GRPC         GRPCConfig                `yaml:"grpc"`
	Streamer     StreamerCongig            `yaml:"streamer"`
	UserFS       UsersFileStorage          `yaml:"users_file_storage"`
	ScriptsVD    VirtualDisplayInitializer `yaml:"virtual_display_initializer"`
	UDPReader    UDPReaderCobfig           `yaml:"udp_reader"`
	Docker       DockerConfig              `yaml:"docker"`
	VideoCapture VideoCaptureConfig        `yaml:"video_capture"`
	AudioCapture AudioCaptureConfig        `yaml:"audio_capture"`
}

func (c *Config) LogValue() logger.Value {
	return logger.GroupValue(
		logger.StringAttr("env", c.Env),
		logger.Group(
			"logger",
			logger.StringAttr("level", c.Logger.Level),
			logger.BoolAttr("json", c.Logger.JSON),
			logger.BoolAttr("source", c.Logger.Source),
		),
		logger.Group(
			"grpc",
			logger.IntAttr("port", c.GRPC.Port),
			logger.DurationAttr("timeout", c.GRPC.Timeout),
		),
		logger.Group(
			"streamer",
			logger.StringAttr("video_codec", c.Streamer.VideoCodec),
			logger.StringAttr("audio_codec", c.Streamer.AudioCodec),
		),
		logger.Group(
			"users_file_storage",
			logger.StringAttr("fs_init_files_dir", c.UserFS.FsInitFilesPath),
			logger.StringAttr("path", c.UserFS.Path),
		),
		logger.Group(
			"virtual_display_initializer",
			logger.StringAttr("enable_virtual_displays_script_path", c.ScriptsVD.EnableVirtualDisplaysScriptPath),
			logger.StringAttr("display_info_json_path", c.ScriptsVD.DisplayInfoJsonPath),
		),
		logger.Group(
			"udp_reader",
			logger.IntAttr("port", c.UDPReader.MaxPort),
			logger.IntAttr("min_port", c.UDPReader.MinPort),
			logger.IntAttr("buffer_size", c.UDPReader.ReadBuffer),
			logger.IntAttr("udp_buffer", c.UDPReader.UdpBuffer),
		),
		logger.Group(
			"docker",
			logger.StringAttr("network", c.Docker.NetworkMode),
			logger.StringAttr("pulse_image", c.Docker.PulseImage),
			logger.StringAttr("video_image", c.Docker.VideoImage),
			logger.StringAttr("audio_image", c.Docker.AudioImage),
			logger.StringAttr("protone_image", c.Docker.ProtoneImage),
			logger.StringAttr("card_path", c.Docker.CardPath),
			logger.StringAttr("renderer_path", c.Docker.RendererPath),
		),
		logger.Group(
			"video_capture",
			StringsLogValue("env", c.VideoCapture.Env),
		),
		logger.Group(
			"audio_capture",
			StringsLogValue("env", c.AudioCapture.Env),
		),
		logger.StringAttr("adsa", "adsd"),
	)
}

func StringsLogValue(index string, extentions []string) logger.Value {
	attrs := make([]logger.Attr, 0, len(extentions))

	for i, v := range extentions {
		attrs = append(attrs, logger.StringAttr(index+strconv.Itoa(i), v))
	}

	return logger.GroupValue(attrs...)
}

type LoggerConfig struct {
	Level  string `yaml:"level"`
	JSON   bool   `yaml:"json" `
	Source bool   `yaml:"source" `
}

type GRPCConfig struct {
	Port       int           `yaml:"port"`
	TLSEnabled bool          `yaml:"tls_enabled"`
	Timeout    time.Duration `yaml:"timeout"`
}

type StreamerCongig struct {
	VideoCodec string `yaml:"video_codec"`
	AudioCodec string `yaml:"audio_codec"`
}

type UsersFileStorage struct {
	FsInitFilesPath string `yaml:"fs_init_files_dir"`
	Path            string `yaml:"path"`
}

type VirtualDisplayInitializer struct {
	EnableVirtualDisplaysScriptPath string `yaml:"enable_virtual_displays_script_path"`
	DisplayInfoJsonPath             string `yaml:"display_info_json_path"`
}
type UDPReaderCobfig struct {
	MinPort    int `yaml:"min_port"`
	MaxPort    int `yaml:"max_port"`
	ReadBuffer int `yaml:"read_buffer"`
	UdpBuffer  int `yaml:"udp_buffer"`
}

type DockerConfig struct {
	PulseImage   string `yaml:"pulse_image"`
	VideoImage   string `yaml:"video_image"`
	AudioImage   string `yaml:"audio_image"`
	ProtoneImage string `yaml:"protone_image"`

	CardPath       string `yaml:"card_path"`
	NetworkMode    string `yaml:"network_mode"`
	RendererPath   string `yaml:"renderer_path"`
	XauthorityPath string `yaml:"xauthority_path"`
}

type VideoCaptureConfig struct {
	Env []string `yaml:"env"`
}
type AudioCaptureConfig struct {
	Env []string `yaml:"env"`
}

func MustLoadByPath(configPath string) *Config {

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		panic("config file does not exist: " + configPath)
	}

	var cfg Config

	if err := cleanenv.ReadConfig(configPath, &cfg); err != nil {
		panic("failed to read config: " + err.Error())
	}

	return &cfg

}
func MustLoad() *Config {
	path := fetchConfigPath()
	if path == "" {
		panic("config path is required")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		panic("config file does not exist: " + path)
	}

	var cfg Config

	if err := cleanenv.ReadConfig(path, &cfg); err != nil {
		panic("failed to read config: " + err.Error())
	}

	return &cfg

}

// fetchConfigPath returns the path of the config file from the environment variable or comand line flag.
// Priority: command line flag > environment variable > default value
// Default value: empty string.
func fetchConfigPath() string {
	var res string

	flag.StringVar(&res, "config", "", "path to the config file")
	flag.Parse()

	if res == "" {
		res = os.Getenv("CONFIG_PATH")
	}
	return res
}
