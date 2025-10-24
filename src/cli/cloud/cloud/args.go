package cloud

import flags "github.com/jessevdk/go-flags"

func ParseArgs(args []string) error {
	opts := &Options{}
	parser := NewParser(opts)

	_, err := parser.ParseArgs(args)
	if err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			return nil
		}
		return err
	}
	return nil
}
