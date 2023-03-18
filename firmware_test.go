package cyw43439

import (
	"fmt"
	"os"
	"testing"
)

func Test(t *testing.T) {
	FW := wifiFW[:]
	err := os.WriteFile("wififw.bin", FW, 0777)
	if err != nil {
		t.Fatal(err)
	}
	return
}
func TestFirmware(t *testing.T) {
	FW := wifiFW[:]
	fp, err := os.Create("wififw.S")
	if err != nil {
		t.Fatal(err)
	}
	defer fp.Close()
	fp.WriteString(".section .text\n")
	fp.WriteString(".global cyw43439wififirmware\n")
	fp.WriteString(".align\n")
	fp.WriteString(".arm\n")
	fp.WriteString("cyw43439wififirmware:\n")
	const width = 4 * 3

	for i := 0; i < len(FW); i++ {
		if i%width == 0 {
			fmt.Fprint(fp, "\n\t.byte ")
		}
		fmt.Fprintf(fp, "%#x", FW[i])
		if (i+1)%width != 0 {
			fp.Write([]byte{','})
		}
	}
}
