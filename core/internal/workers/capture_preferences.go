package workers

import (
	"context"

	"github.com/zajuna-app/core/internal/checklist"
)

// CapturePreferences are the user's capture settings (Configuración ›
// Capturas y Cuenta Zajuna) applied to every checklist run.
type CapturePreferences struct {
	// FullPage keeps the full-page rule of profile and cronograma targets.
	// When false those targets capture only the matched block.
	FullPage bool `json:"fullPage"`
	// ReuseSession shares authenticated Chromium sessions across the targets
	// of one run. When false every target opens and closes its own session.
	ReuseSession bool `json:"reuseSession"`
	// AutoRenew retries a target once with a fresh login when Zajuna expired
	// the session in the middle of the run.
	AutoRenew bool `json:"autoRenew"`
}

func DefaultCapturePreferences() CapturePreferences {
	return CapturePreferences{FullPage: true, ReuseSession: true, AutoRenew: true}
}

// SetPreferences installs the source of capture preferences. It is read at
// the start of each run so a change in Configuración applies to the next job
// without restarting the core.
func (w *CaptureChecklistWorker) SetPreferences(load func(context.Context) CapturePreferences) {
	w.loadPreferences = load
}

func (w *CaptureChecklistWorker) preferences(ctx context.Context) CapturePreferences {
	if w.loadPreferences == nil {
		return DefaultCapturePreferences()
	}
	return w.loadPreferences(ctx)
}

func applyCapturePreferences(targets []checklist.CaptureTarget, prefs CapturePreferences) []checklist.CaptureTarget {
	if prefs.FullPage {
		return targets
	}
	adjusted := make([]checklist.CaptureTarget, len(targets))
	for index, target := range targets {
		target.FullPage = false
		adjusted[index] = target
	}
	return adjusted
}
