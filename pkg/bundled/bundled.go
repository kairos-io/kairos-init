package bundled

import (
	"embed"
	_ "embed"
)

//go:embed binaries/kairos-agent
var EmbeddedAgent []byte

//go:embed binaries/immucore
var EmbeddedImmucore []byte

//go:embed binaries/kcrypt-discovery-challenger
var EmbeddedKcryptChallenger []byte

//go:embed cloudconfigs/*
var EmbeddedConfigs embed.FS
