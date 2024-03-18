# HTTP Client example

This example shows end-to-end network stack initialization
and TCP connection to send a request to a server on the local network.

If the user wishes to reach an external network over the internet some adaptation is needed.
Refer to the ntp example which leverages DNS and Router management to connect to internet.


## Instructions
| Commands are shown run from the root directory of this project! |
|---|

1. Run a local HTTP server. One is provided under [`server`](./server/) directory:
    ```sh
    go run ./examples/server
    ```
2. If you haven't already, create [`secrets.go` from the `secrets.go.template` file](../common/secrets.go.template)
    with your wifi SSID and password (or `password=""` for open networks).

3. Change the server address constant in [`main.go`](./main.go) to reflect your 
    servers' address+port number combination.

4. Flash your Pico W with the program:
    ```sh
    tinygo flash -target=pico -stack-size=8kb -monitor ./examples/http-client/
    ```
    This command will flash the pico and immediately connect to it via USB and print it's output to your console.


The result should look somewhat like this:

```
pato@qtpi:~/src/tg/cyw43439$ tinygo flash -target=pico -stack-size=8kb -monitor ./examples/http-client/
Connected to /dev/ttyACM0. Press Ctrl-C to exit.
time=1970-01-01T00:00:01.593Z level=INFO msg=cyw43439:Init duration=1.583062s
time=1970-01-01T00:00:01.594Z level=INFO msg="joining WPA secure network" ssid="WHITTINGSLOW 2.4" passlen=10
time=1970-01-01T00:00:05.039Z level=INFO msg="wifi join success!" mac=28:cd:c1:05:4d:bb
time=1970-01-01T00:00:05.041Z level=INFO msg="DHCP ongoing..."
time=1970-01-01T00:00:05.042Z level=INFO msg=DHCP:tx msg=Discover
time=1970-01-01T00:00:05.093Z level=INFO msg=DHCP:rx msg=Offer
time=1970-01-01T00:00:05.094Z level=INFO msg=DHCP:tx msg=Request
time=1970-01-01T00:00:05.144Z level=INFO msg=DHCP:rx msg=Ack
time=1970-01-01T00:00:05.542Z level=INFO msg="DHCP complete" cidrbits=24 ourIP=192.168.0.68 dns=192.168.0.1 broadcast=192.168.0.255 gateway="invalid IP" router=192.168.0.1 dhcp=192.168.0.1 hostname=tinygo-http-client lease=1h0m0s renewal=30m0s rebinding=52m30s
time=1970-01-01T00:00:05.674Z level=INFO msg=tcp:ready clientaddr=192.168.0.68:53691 serveraddr=192.168.0.44:8080
1970/01/01 00:00:10 INFO dialing serveraddr=192.168.0.44:8080
time=1970-01-01T00:00:10.754Z level=INFO msg=TCP:rx-statechange port=53691 old=SynSent new=Established rxflags=[SYN,ACK]
got HTTP response!
HTTP/1.1 200 OK
Date: Mon, 18 Mar 2024 00:00:26 GMT
Content-Length: 65
Content-Type: text/plain; charset=utf-8

Dear 192.168.0.68:53691,
Greetings from qtpi @ 192.168.0.44:8080

1970/01/01 00:00:11 ERROR tcpconn:closing err=done
1970/01/01 00:00:11 INFO tcpconn:waiting state=FinWait1
time=1970-01-01T00:00:11.315Z level=INFO msg=TCP:tx-statechange port=53691 old=Established new=FinWait1 txflags=[FIN,ACK]
time=1970-01-01T00:00:11.366Z level=INFO msg=TCP:rx-statechange port=53691 old=FinWait1 new=TimeWait rxflags=[FIN,ACK]
```