package options

// LevelFormatOptionName is the option name.
const LevelFormatOptionName = "format"

const (
	// LevelFormatFull, `format=full`, outputs the whole level name, such as `debug`.
	LevelFormatFull LevelFormatValue = iota
	// LevelFormatShort, `format=short`, outputs a three-letter level name, such as `deb`.
	LevelFormatShort
	// LevelFormatTiny, `format=tiny`, outputs a one-letter level name, such as `d`.
	LevelFormatTiny
)

var levelFormatList = NewMapValue(map[LevelFormatValue]string{
	LevelFormatTiny:  "tiny",
	LevelFormatShort: "short",
	LevelFormatFull:  "full",
})

// LevelFormatValue selects which name of the entry's level [LevelFormatOption]
// outputs.
type LevelFormatValue byte

// LevelFormatOption is the `format` option of the `%level` placeholder. It
// replaces the text with the entry's level name.
type LevelFormatOption struct {
	*CommonOption[LevelFormatValue]
}

// Format implements `Option` interface.
func (format *LevelFormatOption) Format(data *Data, _ any) (any, error) {
	switch format.value.Get() {
	case LevelFormatTiny:
		return data.Level.TinyName(), nil
	case LevelFormatShort:
		return data.Level.ShortName(), nil
	case LevelFormatFull:
	}

	return data.Level.FullName(), nil
}

// LevelFormat creates the option to format level name.
func LevelFormat(val LevelFormatValue) Option {
	return &LevelFormatOption{
		CommonOption: NewCommonOption(LevelFormatOptionName, levelFormatList.Set(val)),
	}
}
