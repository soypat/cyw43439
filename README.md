# cyw43439
Driver for the Wifi+bluetooth integrated circuit on the pico.

## FYI
* [CYW43439 datasheet](https://www.infineon.com/dgdl/Infineon-CYW43439-DataSheet-v03_00-EN.pdf?fileId=8ac78c8c8386267f0183c320336c029f)

* [Pico SDK github repo](https://github.com/raspberrypi/pico-sdk)
    * [`pico_w.h`](https://github.com/raspberrypi/pico-sdk/blob/master/src/boards/include/boards/pico_w.h)
    
    * [`cyw43_driver/cyw43_bus_pio_spi.c`](https://github.com/raspberrypi/pico-sdk/blob/master/src/rp2_common/cyw43_driver/cyw43_bus_pio_spi.c): Core driver for interfacing directly with the CYW43439. This is what this repo will target in the port. 
    
    * [`pico_cyw43_arch`](https://github.com/raspberrypi/pico-sdk/blob/master/src/rp2_common/pico_cyw43_arch): Architecture for integrating the CYW43 driver (for the wireless on Pico W) and lwIP (for TCP/IP stack) into the SDK. It is also necessary for accessing the on-board LED on Pico W.
        * [`pico_cyw43_arch/include/pico/cyw43_arch.h`](https://github.com/raspberrypi/pico-sdk/blob/master/src/rp2_common/pico_cyw43_arch/include/pico/cyw43_arch.h): Headers for the architecture driver. Has a **very complete comment** introducing the architecture library.

### Go and TinyGo Ethernet/IP/TCP stack comparisons
![stack comparison](stack_comparison.png)