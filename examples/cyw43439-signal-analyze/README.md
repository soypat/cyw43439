## CYW43439 Signal Analyze

This program declares the SPI bus as a software bit-bang implementation
which also broadcasts the CYW43439 responses to the the mock output pins
so they can be analyzed by a logic analyzer, such as a Saleae or Digital Analog Discovery devices.


To flash a Pico W with the program:
```sh
tinygo flash -target=pico -tags=cy43nopio ./examples/cyw43439-signal-analyze
```

The mocked signals will be observable on pins defined by:
```go
	// Mock pins can be any not shared by original implementation.
	// Attach your logic analyzer to these.
	MOCK_DAT = machine.GPIO4
	MOCK_CLK = machine.GPIO5
	MOCK_CS  = machine.GPIO6
```
In the case above
* GPIO4 will have the data exchange, both read and write.
* GPIO5 will have the SPI clock signal.
* GPIO6 will have the SPI Chip Select.
