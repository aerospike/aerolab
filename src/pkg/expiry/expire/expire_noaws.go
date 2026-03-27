//go:build noaws

package expire

import "log"

func (h *ExpiryHandler) expireEksctl(_ string) error {
	log.Print("EKS: expiry skipped (AWS not available in this build)")
	return nil
}
