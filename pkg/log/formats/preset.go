package formats

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
	names := make([]string, len(presets))

	for i, preset := range presets {
		if preset.name != "" {
			names[i] = preset.name
		}
	}

	return names
}

type Preset struct {
	name string
	opts Options
}

func NewPreset(name string, opts ...*Option) *Preset {
	return &Preset{
		name: name,
		opts: opts,
	}
}
