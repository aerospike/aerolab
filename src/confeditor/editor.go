package confeditor

import "fmt"

type Editor struct {
	Path string
}

func (e *Editor) Run() error {
	fmt.Println("Hello Bob World")
	return nil
}
