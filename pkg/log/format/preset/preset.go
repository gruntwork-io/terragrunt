package preset

type Presets []*Preset

func (presets Presets) Find(name string) *Preset {
	for _, preset := range presets {
		if preset.name == name {
			return preset
		}
	}

	return nil
}

func (presets Presets) Names() []string {
	var names []string

	for _, preset := range presets {
		if preset.name != "" {
			names = append(names, preset.name)
		}
	}

	return names
}

type Preset struct {
	name string
	opts Options
}

func (preset *Preset) Options() Options {
	return preset.opts
}

func New(name string, opts ...*Option) *Preset {
	return &Preset{
		name: name,
		opts: opts,
	}
}
