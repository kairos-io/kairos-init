package config

import (
	"fmt"
	semver "github.com/hashicorp/go-version"
	"gopkg.in/yaml.v3"
	"os"
	"strings"
)

// Config is the struct to track the config of the init image
// So we can access it from anywhere
type Config struct {
	Model              string
	Variant            Variant
	TrustedBoot        bool
	Fips               bool
	KubernetesProvider KubernetesProvider
	KubernetesVersion  string
	KairosVersion      semver.Version
	Extensions         bool
	VersionOverrides   VersionOverrides
	SkipSteps          []string
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
		return
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	err = decoder.Decode(&c.VersionOverrides)
	if err != nil {
		return
	}
}

func init() {
	// Attempt to load version overrides during initialization
	DefaultConfig.LoadVersionOverrides()
}

// ContainsSkipStep checks if a step is in the skip steps list
func ContainsSkipStep(step string) bool {
	for _, s := range DefaultConfig.SkipSteps {
		if strings.ToLower(s) == strings.ToLower(step) {
			return true
		}
	}
	return false
}
