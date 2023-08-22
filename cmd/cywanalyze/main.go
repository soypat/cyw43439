package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/soypat/cyw43439/internal/slog"
	"github.com/soypat/saleae"
	"github.com/soypat/saleae/analyzers"
	"golang.org/x/exp/constraints"
)

// Optional flags.
var (
	timingsOutput string
)

type BusCtl struct {
	// Bus ordering.
	Order binary.ByteOrder
	// Interpret bytes as words.
	WordInterpreter binary.ByteOrder
	TrimForce       uint
	TrimStatus      bool
	OmitReadData    bool
	OmitRead        bool
	OmitWrite       bool
	OmitIneffectual bool
	PadDataToWord   bool
}

func main() {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(handler))
	slog.Debug("hello")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "cywanalyze - Process Binary Saleae digital data files corresponding to CYW43439 transactions.\n\tUsage:\n")
		flag.PrintDefaults()
	}
	sdio := flag.String("f-sd", "digital_1.bin", "Input filename: SPI SDO/SDI data.")
	enable := flag.String("f-cs", "digital_0.bin", "Input filename: SPI CS/SS data.")
	clk := flag.String("f-clk", "digital_2.bin", "Input filename: SPI CS data.")
	output := flag.String("o-cmd", "commands.txt", "Output filename of CYW43439 command transactions.")

	flag.StringVar(&timingsOutput, "o-time", "", "Output timing data to a file corresponding to output command history line-by-line.")
	const defaultOrdering = "le"
	flagInterpretWords := flag.String("interpret-words", "", "Interpret byte data as uint32 words based on bctl-le. Accepts 'be' or 'le'.")
	flagBCTLLE := flag.String("bctl-order", defaultOrdering, "Bus Control register in little endian mode.")
	flagTrimStatus := flag.Bool("trim-stat", false, "Trim status word. Will look at command length and trim 4 trailing bytes not part of actual command data.")
	flagTrimForce := flag.Uint("trim-force", 0, "Trims n bytes off the end of every command.")
	omitReadData := flag.Bool("omit-read-data", false, "Choose to omit read data in output.")
	omitReadAll := flag.Bool("omit-read", false, "Choose to omit read commands in output.")
	omitWriteAll := flag.Bool("omit-write", false, "Choose to omit write commands in output.")
	omitIneffectual := flag.Bool("omit-inef", false, "Omit data after the command size.")
	padDataToWord := flag.Bool("pad-data", false, "Pad data to word size (4 bytes).")
	flag.Parse()
	if *flagInterpretWords == "" {
		*flagInterpretWords = *flagBCTLLE
	}
	getOrder := func(s string) binary.ByteOrder {
		switch s {
		case "be":
			return binary.BigEndian
		case "le":
			return binary.LittleEndian
		}
		log.Fatal("invalid ordering", s)
		return nil
	}
	BUS := BusCtl{
		Order:           getOrder(*flagBCTLLE),
		WordInterpreter: getOrder(*flagInterpretWords),
		TrimForce:       *flagTrimForce,
		TrimStatus:      *flagTrimStatus,
		OmitReadData:    *omitReadData,
		OmitRead:        *omitReadAll,
		OmitWrite:       *omitWriteAll,
		PadDataToWord:   *padDataToWord,
		OmitIneffectual: *omitIneffectual,
	}
	if BUS.OmitRead && BUS.OmitWrite {
		log.Fatal("cannot omit both read and write commands")
	}
	start := time.Now()
	if err := BUS.run(*sdio, *enable, *clk, *output); err != nil {
		log.Fatal(err.Error())
	}
	log.Println("finished in", time.Since(start))
}

func (bus *BusCtl) run(sdio, enable, clk, output string) error {
	const fmtMsg = "cmd×%2d %s data=%#x"
	commands, err := bus.processSpiFiles(sdio, clk, enable)
	if err != nil {
		return err
	}
	fp, err := os.Create(output)
	if err != nil {
		return err
	}
	defer fp.Close()

	var timings *os.File
	if timingsOutput != "" {
		log.Println("creating timings file", timingsOutput)
		timings, err = os.Create(timingsOutput)
		if err != nil {
			return err
		}
		defer timings.Close()
	}

	for _, action := range commands {
		if (bus.OmitRead && !action.Cmd.Write) || (bus.OmitWrite && action.Cmd.Write) {
			continue
		} else if bus.OmitReadData && !action.Cmd.Write {
			action.Data = []byte{}
		} else if bus.PadDataToWord && len(action.Data)%4 != 0 {
			unpadded := len(action.Data) - len(action.Data)%4
			data := append([]byte{}, action.Data[:unpadded]...)
			if bus.WordInterpreter == binary.BigEndian {
				data = append(data, make([]byte, 4-len(action.Data)%4)...)
				action.Data = append(data, action.Data[unpadded:]...)
			} else {
				data = append(action.Data[unpadded:], data...)
				action.Data = append(data, make([]byte, 4-len(action.Data)%4)...)
			}
		}
		if bus.OmitIneffectual && action.Cmd.Size < uint32(len(action.Data)) {
			action.Data = action.Data[:action.Cmd.Size]
		}
		if action.Cmd.Size < uint32(len(action.Data)) {
			// Print a space demarcating end of the command data.
			// Anything after space is "garbage" data and not actually part of the command.
			fmt.Fprintf(fp, fmtMsg, action.Num, action.Cmd.String(), action.Data[:action.Cmd.Size])
			_, err = fmt.Fprintf(fp, " %x", action.Data[action.Cmd.Size:])
		} else {
			_, err = fmt.Fprintf(fp, fmtMsg, action.Num, action.Cmd.String(), action.Data)
		}
		if err != nil {
			return err
		}
		fmt.Fprintln(fp)
		if timings != nil {
			fmt.Fprintf(timings, "t=%f\tdata=%#x\n", action.Start, action.Data)
		}
	}
	return nil
}

func (bus *BusCtl) processSpiFiles(fsdio, fclk, fenable string) ([]cywtx, error) {
	sdio, err := opendigital(fsdio)
	if err != nil {
		return nil, err
	}
	clk, err := opendigital(fclk)
	if err != nil {
		return nil, err
	}
	enable, err := opendigital(fenable)
	if err != nil {
		return nil, err
	}
	spi := analyzers.SPI{}
	txs, _ := spi.Scan(clk, enable, sdio, sdio)
	return bus.process(txs), nil
}

func opendigital(filename string) (*saleae.DigitalFile, error) {
	fp, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer fp.Close()
	df, err := saleae.ReadDigitalFile(fp)
	if err != nil {
		return nil, err
	}
	return df, nil
}

type CYW43439Cmd struct {
	Write   bool
	AutoInc bool
	Fn      Function
	Addr    uint32
	Size    uint32
}

func (cmd *CYW43439Cmd) String() string {
	return fmt.Sprintf("addr=%#7x  fn=%9s  sz=%4v write=%5v autoinc=%5v",
		cmd.Addr, cmd.Fn.String(), cmd.Size, cmd.Write, cmd.AutoInc)
}

func (bus *BusCtl) CommandFromBytes(b []byte) (cmd CYW43439Cmd, data []byte) {
	if len(b) < 4 {
		// Invalid command. Look for "invalid"
		cmd, _ := bus.CommandFromBytes([]byte{0xff, 0xff, 0xff, 0xff})
		cmd.Fn = funcInvalid
		return cmd, b
	}
	_ = b[3]
	command := bus.Order.Uint32(b)

	cmd.Write = command&(1<<31) != 0
	cmd.AutoInc = command&(1<<30) != 0
	cmd.Fn = Function(command>>28) & 0b11
	cmd.Addr = (command >> 11) & 0x1ffff
	cmd.Size = command & ((1 << 11) - 1)
	data = b[4:]
	if cmd.Fn == FuncBackplane && !cmd.Write && len(data) > 4 {
		data = b[8:] // padding.
	}
	if bus.TrimForce > 0 {
		data = data[:max(0, len(data)-int(bus.TrimForce))]
	}
	if bus.TrimStatus && len(data)-int(cmd.Size) == 4 {
		data = data[:cmd.Size]
	}
	return cmd, data
}

type cywtx struct {
	Num   int
	Cmd   CYW43439Cmd
	Data  []byte
	Start float64
}

func (bus *BusCtl) process(txs []analyzers.TxSPI) (cytxs []cywtx) {
	var accumulativeResults int = 1
	for i := 0; i < len(txs); i++ {
		tx := txs[i]
		cmd, data := bus.CommandFromBytes(tx.SDO)
		for j := i + 1; j < len(txs); j++ {
			nextcmd, nextdata := bus.CommandFromBytes(txs[j].SDO)
			if nextcmd != cmd || !bytes.Equal(data, nextdata) {
				break
			}
			accumulativeResults++
			i = j
		}
		bus.interpretBytes(data)
		cytxs = append(cytxs, cywtx{
			Num:   accumulativeResults,
			Cmd:   cmd,
			Data:  data,
			Start: tx.StartTime(),
		})
		accumulativeResults = 1
	}
	return cytxs
}

type Function uint32

const (
	// All SPI-specific registers.
	FuncBus Function = 0b00
	// Registers and memories belonging to other blocks in the chip (64 bytes max).
	FuncBackplane Function = 0b01
	// DMA channel 1. WLAN packets up to 2048 bytes.
	FuncDMA1 Function = 0b10
	FuncWLAN          = FuncDMA1
	// DMA channel 2 (optional). Packets up to 2048 bytes.
	FuncDMA2    Function = 0b11
	funcInvalid Function = 0b111011110111
)

func (f Function) String() (s string) {
	switch f {
	case FuncBus:
		s = "bus"
	case FuncBackplane:
		s = "backplane"
	case FuncWLAN: // same as FuncDMA1
		s = "wlan"
	case FuncDMA2:
		s = "dma2"
	case funcInvalid:
		s = "invalid"
	default:
		s = "unknown"
	}
	return s
}

var interpretOnce sync.Once

func (bus *BusCtl) interpretBytes(data []byte) {
	if bus.WordInterpreter == bus.Order {
		return // Idempotent transformation.
	}
	interpretOnce.Do(func() {
		log.Println("interpreting bytes as words in", bus.WordInterpreter.String(), "order")
	})
	for len(data) >= 4 {
		word := bus.Order.Uint32(data[:4])
		bus.WordInterpreter.PutUint32(data[:4], word)
		data = data[4:]
	}
}

func min[T constraints.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

func max[T constraints.Ordered](a, b T) T {
	if a > b {
		return a
	}
	return b
}
