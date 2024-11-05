package options

const LevelFormatOptionName = "format"

const (
	LevelFormatTiny LevelFormatValue = iota
	LevelFormatShort
	LevelFormatFull
)

var levelFormatValues = CommonMapValues[LevelFormatValue]{
	LevelFormatTiny:  "tiny",
	LevelFormatShort: "short",
	LevelFormatFull:  "full",
}

type LevelFormatValue byte

type levelFormat struct {
	*CommonOption[LevelFormatValue]
}

func (format *levelFormat) Evaluate(data *Data, str string) string {
	switch format.Value() {
	case LevelFormatTiny:
		return data.Level.TinyName()
	case LevelFormatShort:
		return data.Level.ShortName()
	case LevelFormatFull:
	}

	return data.Level.FullName()
}

func LevelFormat(val LevelFormatValue) Option {
	return &levelFormat{
		CommonOption: NewCommonOption[LevelFormatValue](LevelFormatOptionName, val, levelFormatValues),
	}
}
