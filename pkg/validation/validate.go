package validation

import (
	"github.com/kairos-io/kairos-init/pkg/system"
	"github.com/kairos-io/kairos-init/pkg/values"
	"github.com/kairos-io/kairos-sdk/types"
)

type Validator struct {
	Log    types.KairosLogger
	System values.System
}

func NewValidator(logger types.KairosLogger) *Validator {
	sis := system.DetectSystem(logger)
	return &Validator{Log: logger, System: sis}
}

func (v *Validator) Validate() error {
	v.Log.Debug("Validating system")
	v.Log.Debug(v.System)
	return nil
}
