package main

import (
	"encoding/binary"
	"encoding/csv"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

type Transaction struct {
	Data []byte
}

func main() {
	fileName := flag.String("file", "digital.csv", "Path to the input file")
	omitAddrs := flag.String("omit-addrs", "", "Omit commands with these addresses. Comma separated list of hex addresses.")
	hexDump := flag.Bool("hex-dump", false, "Do full hex.Dump() of out data")
	flag.Parse()

	var addrs = make(map[uint32]bool)
	if *omitAddrs != "" {
		for i, addr := range strings.Split(*omitAddrs, ",") {
			addr = strings.TrimPrefix(addr, "0x")
			v, err := strconv.ParseUint(addr, 16, 32)
			if err != nil {
				log.Fatalf("parsing address %d: %s", i+1, err)
			}
			addrs[uint32(v)] = true
		}
	}

	file, err := os.Open(*fileName)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	transactions := parseRecords(records[1:])
	for _, t := range transactions {
		parsed := parseTransaction(t.Data, addrs, *hexDump)
		if parsed != "" {
			fmt.Printf("%s\n", parsed)
		}
	}
}

func swapBytes(n uint32) uint32 {
	return (n&0xFF00FF00)>>8 | (n&0x00FF00FF)<<8
}

func swapData(data []byte) []byte {
	if len(data)%4 != 0 {
		// handle this case as needed
		panic("slice length is not a multiple of 4")
	}

	for i := 0; i < len(data); i += 4 {
		data[i], data[i+1], data[i+2], data[i+3] = data[i+3], data[i+2], data[i+1], data[i]
	}

	return data
}

var fns = map[uint32]string{
	0: "bus",
	1: "backplane",
	2: "wlan",
	3: "dma2",
}

func parseTransaction(data []byte, omitAddrs map[uint32]bool, hexDump bool) string {
	endian := "BE"
	cmd := binary.BigEndian.Uint32(data[:4])
	size := cmd & 0x7ff
	if size == 0 {
		// size==0 so cmd must be LE?
		endian = "LE"
		cmd = swapBytes(binary.LittleEndian.Uint32(data[:4]))
	}

	write := (cmd & (1 << 31)) >> 31
	autoinc := (cmd & (1 << 30)) >> 30
	fn := (cmd & (0x3 << 28)) >> 28
	addr := (cmd & (0x1ffff << 11)) >> 11
	size = cmd & 0x7ff

	if omitAddrs[addr] {
		return ""
	}

	out := ""
	if write == 1 {
		data = swapData(data[4:len(data)-4])
		for i := 0; i < len(data); i++ {
			out += hex.EncodeToString(data[i:i+1])
			if ((i+1) % 4) == 0 {
				out += " "
			}
		}
		if hexDump {
			out += "\n" + hex.Dump(data)
		}
	}

	return fmt.Sprintf("%s cmd=0x%08x addr=0x%05x fn=%-9s sz=0x%03x w=%d inc=%d   %s", endian, cmd, addr, fns[fn], size, write, autoinc, out)
}

func parseRecords(records [][]string) []Transaction {
	var transactions []Transaction
	var currentTransaction []byte
	var currentByte uint8
	var bitCounter uint8

	var prevCS int
	var prevCLK int

	for _, record := range records {
		CS, _ := strconv.Atoi(record[1])
		MockSDI, _ := strconv.Atoi(record[2])
		CLK, _ := strconv.Atoi(record[3])

		if prevCS == 1 && CS == 0 {
			// Start of a new transaction
			currentTransaction = nil
			currentByte = 0
			bitCounter = 0
		}

		if prevCLK == 0 && CLK == 1 {
			// Capture MockSDI bit on CLK rising edge
			currentByte = (currentByte << 1) | uint8(MockSDI)
			bitCounter++

			if bitCounter == 8 {
				currentTransaction = append(currentTransaction, currentByte)
				bitCounter = 0
				currentByte = 0
			}
		}

		if prevCS == 0 && CS == 1 {
			// End of transaction, store the transaction if it has data
			if len(currentTransaction) > 0 {
				transactions = append(transactions, Transaction{Data: currentTransaction})
			}
		}

		prevCS = CS
		prevCLK = CLK
	}

	return transactions
}
