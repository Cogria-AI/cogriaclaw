package wa

import (
	"fmt"
	"io"

	"github.com/mdp/qrterminal/v3"
)

// PrintQR renders a WhatsApp pairing QR as half-block characters.
// Level L is the lowest correction — enough for QR codes scanned at close range
// off a clear terminal.
func PrintQR(w io.Writer, code string) {
	fmt.Fprintln(w, "Scan with WhatsApp → Settings → Linked Devices → Link a Device:")
	qrterminal.GenerateHalfBlock(code, qrterminal.L, w)
}
