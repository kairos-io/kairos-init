package binaries

import _ "embed"

//go:embed assets/kairos-agent
var EmbeddedAgent []byte

//go:embed assets/immucore
var EmbeddedImmucore []byte

//go:embed assets/kcrypt-discovery-challenger
var EmbeddedKcryptChallenger []byte
