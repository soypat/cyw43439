/*
package tcpctl implements TCP as per RFC793 (September 1981).

# State diagram of TCP

	                             +---------+ ---------\      active OPEN
	                             |  CLOSED |            \    -----------
	                             +---------+<---------\   \   create TCB
	                               |     ^              \   \  snd SYN
	                  passive OPEN |     |   CLOSE        \   \
	                  ------------ |     | ----------       \   \
	                   create TCB  |     | delete TCB         \   \
	                               V     |                      \   \
	                             +---------+            CLOSE    |    \
	                             |  LISTEN |          ---------- |     |
	                             +---------+          delete TCB |     |
	                  rcv SYN      |     |     SEND              |     |
	                 -----------   |     |    -------            |     V
	+---------+      snd SYN,ACK  /       \   snd SYN          +---------+
	|         |<-----------------           ------------------>|         |
	|   SYN   |                    rcv SYN                     |   SYN   |
	|   RCVD  |<-----------------------------------------------|   SENT  |
	|         |                    snd ACK                     |         |
	|         |------------------           -------------------|         |
	+---------+   rcv ACK of SYN  \       /  rcv SYN,ACK       +---------+
	  |           --------------   |     |   -----------
	  |                  x         |     |     snd ACK
	  |                            V     V
	  |  CLOSE                   +---------+
	  | -------                  |  ESTAB  |
	  | snd FIN                  +---------+
	  |                   CLOSE    |     |    rcv FIN
	  V                  -------   |     |    -------
	+---------+          snd FIN  /       \   snd ACK          +---------+
	|  FIN    |<-----------------           ------------------>|  CLOSE  |
	| WAIT-1  |------------------                              |   WAIT  |
	+---------+          rcv FIN  \                            +---------+
	  | rcv ACK of FIN   -------   |                            CLOSE  |
	  | --------------   snd ACK   |                           ------- |
	  V        x                   V                           snd FIN V
	+---------+                  +---------+                   +---------+
	|FINWAIT-2|                  | CLOSING |                   | LAST-ACK|
	+---------+                  +---------+                   +---------+
	  |                rcv ACK of FIN |                 rcv ACK of FIN |
	  |  rcv FIN       -------------- |    Timeout=2MSL -------------- |
	  |  -------              x       V    ------------        x       V
	   \ snd ACK                 +---------+delete TCB         +---------+
	    ------------------------>|TIME WAIT|------------------>| CLOSED  |
	                             +---------+                   +---------+
*/
package tcpctl

import (
	"net"
	"time"
)

type netdever interface {

	// GetHostByName returns the IP address of either a hostname or IPv4
	// address in standard dot notation
	GetHostByName(name string) (net.IP, error)

	// Berkely Sockets-like interface, Go-ified.  See man page for socket(2), etc.
	Socket(domain int, stype int, protocol int) (int, error)
	Bind(sockfd int, ip net.IP, port int) error
	Connect(sockfd int, host string, ip net.IP, port int) error
	Listen(sockfd int, backlog int) error
	Accept(sockfd int, ip net.IP, port int) (int, error)
	Send(sockfd int, buf []byte, flags int, deadline time.Time) (int, error)
	Recv(sockfd int, buf []byte, flags int, deadline time.Time) (int, error)
	Close(sockfd int) error
	SetSockOpt(sockfd int, level int, opt int, value interface{}) error
}
