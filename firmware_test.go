package cyw43439

import (
	"io"
	"strings"
	"testing"
	_ "unsafe"
)

type bla []byte
type reader interface {
	Read(b bla) (int, error)
}

//go:linkname readall io.ReadAll
func readall(r reader) ([]byte, error)

func TestPrintUint32(t *testing.T) {
	// var alignedFW [wifiFWLen / 4]uint32
	// for i := 0; i < wifiFWLen; i += 4 {
	r := &rr{r: strings.NewReader("blabla")}
	k, _ := readall(r)
	t.Error(k)
	// }
}

type rr struct {
	r io.Reader
}

func (r *rr) Read(b bla) (int, error) {
	n, err := r.r.Read(b)
	return n, err
}
