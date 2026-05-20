package kind

import "strings"

// Palette is the `:` command-mode dispatcher. The UI half (input field, modal,
// keybinding) lives in the `view` package; this struct just maps a typed name
// to a registered Kind.
type Palette struct {
	app App
}

func NewPalette(app App) *Palette { return &Palette{app: app} }

// Submit handles a name typed into the palette. Empty input is a no-op
// (user cancelled).
func (p *Palette) Submit(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	k, ok := Get(name)
	if !ok {
		p.app.FlashError("unknown kind: " + name)
		return
	}
	v, err := k.Build(p.app)
	if err != nil {
		p.app.FlashError(err.Error())
		return
	}
	if err := p.app.SwitchView(k, v); err != nil {
		p.app.FlashError(err.Error())
	}
}
