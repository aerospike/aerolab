package backends

import (
	"fmt"
	"os"
)

func ReturnNotImplemented(backendType BackendType, functionName string) error {
	val := os.Getenv("AEROLAB_NOERROR_ON_NOT_IMPLEMENTED")
	if val == "true" || val == "1" {
		return nil
	}
	return fmt.Errorf("function %s not implemented for backend type: %s", functionName, backendType)
}
