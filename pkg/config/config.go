package config

// Config is the struct to track the config of the init image
// So we can access it from anywhere
type Config struct {
	Level            string
	Stage            string
	Model            string
	FrameworkVersion string
	Variant          string
	Registry         string
	TrustedBoot      bool
}

var DefaultConfig = Config{}
