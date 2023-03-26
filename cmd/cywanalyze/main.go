package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	cyw43439 "github.com/soypat/cy43439"
	"github.com/soypat/saleae"
	"github.com/soypat/saleae/analyzers"
)

func main() {
	sdio := flag.String("sdio", "digital_4.bin", "Binary Saleae digital data filename with SPI SDO/SDI data.")
	enable := flag.String("enable", "digital_2.bin", "Binary Saleae digital data filename with SPI enable data.")
	clk := flag.String("clk", "digital_1.bin", "Binary Saleae digital data filename with SPI clock data.")
	output := flag.String("o", "commands.txt", "Output history of commands to file.")
	start := time.Now()
	if err := run(*sdio, *enable, *clk, *output); err != nil {
		log.Fatal(err.Error())
	}
	log.Println("finished in", time.Since(start))
}

func run(sdio, enable, clk, output string) error {
	const fmtMsg = "cmd×%2d %s data=%#x\n"
	commands, err := processSpiFiles(sdio, clk, enable)
	if err != nil {
		return err
	}
	fp, err := os.Create(output)
	if err != nil {
		return err
	}
	defer fp.Close()
	for _, action := range commands {
		_, err = fmt.Fprintf(fp, fmtMsg, action.Num, action.Cmd.String(), action.Data)
		if err != nil {
			return err
		}
	}
	return nil
}

func processSpiFiles(fsdio, fclk, fenable string) ([]cywtx, error) {
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
	return process(txs), nil
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
	Fn      cyw43439.Function
	Addr    uint32
	Size    uint32
}

func (cmd *CYW43439Cmd) String() string {
	return fmt.Sprintf("addr=%#7x  fn=%9s  sz=%4v write=%5v autoinc=%5v",
		cmd.Addr, cmd.Fn.String(), cmd.Size, cmd.Write, cmd.AutoInc)
}

func CommandFromBytes(b []byte) (cmd CYW43439Cmd, data []byte) {
	_ = b[3]
	command := binary.LittleEndian.Uint32(b)
	cmd.Write = command&(1<<31) != 0
	cmd.AutoInc = command&(1<<30) != 0
	cmd.Fn = cyw43439.Function(command>>28) & 0b11
	cmd.Addr = (command >> 11) & 0x1ffff
	cmd.Size = command & ((1 << 11) - 1)
	data = b[4:]
	if cmd.Fn == cyw43439.FuncBackplane && !cmd.Write && len(data) > 4 {
		data = b[8:] // padding.
	}
	return cmd, data
}

type cywtx struct {
	Num  int
	Cmd  CYW43439Cmd
	Data []byte
}

func process(txs []analyzers.TxSPI) (cytxs []cywtx) {
	var accumulativeResults int
	for i := 0; i < len(txs); i++ {
		tx := txs[i]
		if len(tx.SDO) < 4 {
			panic("too short exchange for cyw43439!")
		}
		cmd, data := CommandFromBytes(tx.SDO)
		for j := i + 1; j < len(txs); j++ {
			accumulativeResults++
			nextcmd, nextdata := CommandFromBytes(txs[j].SDO)
			if nextcmd != cmd || !bytes.Equal(data, nextdata) {
				break
			}
			i = j
		}
		cytxs = append(cytxs, cywtx{
			Num:  accumulativeResults,
			Cmd:  cmd,
			Data: data,
		})
		accumulativeResults = 0
	}
	return cytxs
}
