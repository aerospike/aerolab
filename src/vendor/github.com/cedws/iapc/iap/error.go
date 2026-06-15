package iap

import "fmt"

type CloseError struct {
	Code   int
	Reason string
}

func (e *CloseError) Error() string {
	return fmt.Sprintf("connection closed: code %v (%v)", e.Code, e.Reason)
}

type ProtocolError struct {
	Err string
}

func (e *ProtocolError) Error() string {
	return fmt.Sprintf("protocol error: %v", e.Err)
}
