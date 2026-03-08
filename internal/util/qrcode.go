package util

import (
	"os"

	qrterminal "github.com/mdp/qrterminal/v3"
)

// PrintQR outputs an ASCII QR code for the given URL to stdout.
func PrintQR(url string) {
	qrterminal.GenerateHalfBlock(url, qrterminal.L, os.Stdout)
}
