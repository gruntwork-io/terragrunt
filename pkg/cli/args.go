package cli

const (
	OnePrefixFlag    NormalizeActsType = iota
	DoublePrefixFlag NormalizeActsType = iota
)

type NormalizeActsType byte

type Args interface {
	Get(n int) string
	// First returns the first argument, or else a blank string
	First() string
	// Tail returns the rest of the arguments (not the first one)
	// or else an empty string slice
	Tail() []string
	// Len returns the length of the wrapped slice
	Len() int
	// Present checks if there are any arguments present
	Present() bool
	// Slice returns a copy of the internal slice
	Slice() []string
	// Normalize formats the arguments according to the given actions.
	Normalize(...NormalizeActsType) Args
}

type args []string

func (list *args) Normalize(acts ...NormalizeActsType) Args {
	var args args

	for _, arg := range *list {
		arg := arg

		for _, act := range acts {
			switch act {
			case OnePrefixFlag:
				if len(arg) >= 3 && arg[0:2] == "--" && arg[2] != '-' {
					arg = arg[1:]
				}
			case DoublePrefixFlag:
				if len(arg) >= 2 && arg[0] == '-' && arg[1] != '-' {
					arg = "-" + arg
				}
			}
		}

		args = append(args, arg)
	}

	return &args
}

func (list *args) Get(n int) string {
	if len(*list) > n {
		return (*list)[n]
	}
	return ""
}

func (list *args) First() string {
	return list.Get(0)
}

func (list *args) Tail() []string {
	if list.Len() >= 2 {
		tail := []string((*list)[1:])
		ret := make([]string, len(tail))
		copy(ret, tail)
		return ret
	}
	return []string{}
}

func (list *args) Len() int {
	return len(*list)
}

func (list *args) Present() bool {
	return list.Len() != 0
}

func (list *args) Slice() []string {
	ret := make([]string, len(*list))
	copy(ret, *list)
	return ret
}
