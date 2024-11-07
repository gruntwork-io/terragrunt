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

type LevelFormatOption struct {
	*CommonOption[LevelFormatValue]
}

func (format *LevelFormatOption) Evaluate(data *Data, str string) (string, error) {
	switch format.Value() {
	case LevelFormatTiny:
		return data.Level.TinyName(), nil
	case LevelFormatShort:
		return data.Level.ShortName(), nil
	case LevelFormatFull:
	}

	return data.Level.FullName(), nil
}

func LevelFormat(val LevelFormatValue) Option {
	return &LevelFormatOption{
		CommonOption: NewCommonOption[LevelFormatValue](LevelFormatOptionName, val, levelFormatValues),
	}
}
