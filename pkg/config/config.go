package config

import (
	"fmt"
	semver "github.com/hashicorp/go-version"
	"gopkg.in/yaml.v3"
	"os"
)

// Config is the struct to track the config of the init image
// So we can access it from anywhere
type Config struct {
	Level               string
	Stage               string
	Model               string
	Variant             Variant
	Registry            string
	TrustedBoot         bool
	Fips                bool
	KubernetesProvider  KubernetesProvider
	KubernetesVersion   string
	KairosVersion       semver.Version
	Extensions          bool
	VersionOverrides    VersionOverrides
	SkipInstallPackages bool
	SkipInstallK8s      bool
}

// VersionOverrides holds version overrides for binaries
type VersionOverrides struct {
	Agent            string `yaml:"agent,omitempty"`
	Immucore         string `yaml:"immucore,omitempty"`
	KcryptChallenger string `yaml:"kcrypt_challenger,omitempty"`
	Provider         string `yaml:"provider,omitempty"`
	EdgeVpn          string `yaml:"edgevpn,omitempty"`
}

var DefaultConfig = Config{}

type Variant string

func (v Variant) Equal(s string) bool {
	return string(v) == s
}

func (v Variant) String() string {
	return string(v)
}

func (v *Variant) FromString(variant string) error {
	*v = Variant(variant)
	switch *v {
	case CoreVariant, StandardVariant:
		return nil
	default:
		return fmt.Errorf("invalid variant: %s, possible values are %s", variant, ValidVariants)
	}
}

const CoreVariant Variant = "core"
const StandardVariant Variant = "standard"

var ValidVariants = []Variant{CoreVariant, StandardVariant}

type KubernetesProvider string

func (v *KubernetesProvider) FromString(provider string) error {
	*v = KubernetesProvider(provider)
	switch *v {
	case K3sProvider, K0sProvider:
		return nil
	default:
		return fmt.Errorf("invalid Kubernetes provider: %s, possible values are %s", provider, ValidProviders)
	}
}

const K3sProvider KubernetesProvider = "k3s"
const K0sProvider KubernetesProvider = "k0s"

var ValidProviders = []KubernetesProvider{K3sProvider, K0sProvider}

// LoadVersionOverrides initializes the VersionOverrides from a file
func (c *Config) LoadVersionOverrides() {
	file, err := os.Open("/etc/kairos/.init_versions.yaml")
	if err != nil {
		fmt.Println("error opening version overrides file:", err)
		return
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	err = decoder.Decode(&c.VersionOverrides)
	if err != nil {
		fmt.Println("error decoding version overrides file:", err)
		return
	}
}

func init() {
	// Attempt to load version overrides during initialization
	DefaultConfig.LoadVersionOverrides()
}
