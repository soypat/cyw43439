package cyw43439

import _ "embed"

// This file is here to speedup the IDE
// when developing.

var (
	//go:embed firmware/43439A0_clmbt.bin
	embassyFWclm string
	//go:embed firmware/43439A0bt.bin
	embassyFWbt string
	//go:embed firmware/43439A0_clm.bin
	clmFW string
	//go:embed firmware/43439A0.bin
	wifiFW2 string
	// Of raw size 225240.
	//go:embed firmware/wififw.bin
	wifiFW string
	// Of raw size 232408 bytes
	//go:embed firmware/wifibtfw.bin
	wifibtFW string
	// Of raw size 6164 bytes.
	//go:embed firmware/btfw.bin
	btFW string
)

const (
	wifiFWLen   = 224190
	wifibtFWLen = 231077
	clmLen      = 984
	_nvramlen   = len(nvram43439)
)

const nvram43439 = "NVRAMRev=$Rev$" + "\x00" +
	"manfid=0x2d0" + "\x00" +
	"prodid=0x0727" + "\x00" +
	"vendid=0x14e4" + "\x00" +
	"devid=0x43e2" + "\x00" +
	"boardtype=0x0887" + "\x00" +
	"boardrev=0x1100" + "\x00" +
	"boardnum=22" + "\x00" +
	"macaddr=00:A0:50:b5:59:5e" + "\x00" +
	"sromrev=11" + "\x00" +
	"boardflags=0x00404001" + "\x00" +
	"boardflags3=0x04000000" + "\x00" +
	"xtalfreq=37400" + "\x00" +
	"nocrc=1" + "\x00" +
	"ag0=255" + "\x00" +
	"aa2g=1" + "\x00" +
	"ccode=ALL" + "\x00" +
	"pa0itssit=0x20" + "\x00" +
	"extpagain2g=0" + "\x00" +
	"pa2ga0=-168,6649,-778" + "\x00" +
	"AvVmid_c0=0x0,0xc8" + "\x00" +
	"cckpwroffset0=5" + "\x00" +
	"maxp2ga0=84" + "\x00" +
	"txpwrbckof=6" + "\x00" +
	"cckbw202gpo=0" + "\x00" +
	"legofdmbw202gpo=0x66111111" + "\x00" +
	"mcsbw202gpo=0x77711111" + "\x00" +
	"propbw202gpo=0xdd" + "\x00" +
	"ofdmdigfilttype=18" + "\x00" +
	"ofdmdigfilttypebe=18" + "\x00" +
	"papdmode=1" + "\x00" +
	"papdvalidtest=1" + "\x00" +
	"pacalidx2g=45" + "\x00" +
	"papdepsoffset=-30" + "\x00" +
	"papdendidx=58" + "\x00" +
	"ltecxmux=0" + "\x00" +
	"ltecxpadnum=0x0102" + "\x00" +
	"ltecxfnsel=0x44" + "\x00" +
	"ltecxgcigpio=0x01" + "\x00" +
	"il0macaddr=00:90:4c:c5:12:38" + "\x00" +
	"wl0id=0x431b" + "\x00" +
	"deadman_to=0xffffffff" + "\x00" +
	"muxenab=0x100" + "\x00" +
	"spurconfig=0x3" + "\x00" +
	"glitch_based_crsmin=1" + "\x00" +
	"btc_mode=1" + "\x00" +
	"\x00\x00" // C includes null terminator in strings.

	/*
	   var (
	   	wifiFW   []byte
	   	wifibtFW [6164]byte
	   	btFW     [232408]byte
	   )
	*/

//go:aligned 4
const nvram1dx = "manfid=0x2d0\x00" +
	"prodid=0x0726\x00" +
	"vendid=0x14e4\x00" +
	"devid=0x43e2\x00" +
	"boardtype=0x0726\x00" +
	"boardrev=0x1202\x00" +
	"boardnum=22\x00" +
	"macaddr=00:90:4c:c5:12:38\x00" +
	"sromrev=11\x00" +
	"boardflags=0x00404201\x00" +
	"boardflags3=0x08000000\x00" +
	"xtalfreq=37400\x00" +
	"nocrc=1\x00" +
	"ag0=0\x00" +
	"aa2g=1\x00" +
	"ccode=ALL\x00" +
	// "pa0itssit=0x20\x00"+
	"extpagain2g=0\x00" +
	"pa2ga0=-145,6667,-751\x00" +
	"AvVmid_c0=0x0,0xc8\x00" +
	"cckpwroffset0=2\x00" +
	"maxp2ga0=74\x00" +
	// "txpwrbckof=6\x00"+
	"cckbw202gpo=0\x00" +
	"legofdmbw202gpo=0x88888888\x00" +
	"mcsbw202gpo=0xaaaaaaaa\x00" +
	"propbw202gpo=0xdd\x00" +
	"ofdmdigfilttype=18\x00" +
	"ofdmdigfilttypebe=18\x00" +
	"papdmode=1\x00" +
	"papdvalidtest=1\x00" +
	"pacalidx2g=48\x00" +
	"papdepsoffset=-22\x00" +
	"papdendidx=58\x00" +
	"il0macaddr=00:90:4c:c5:12:38\x00" +
	"wl0id=0x431b\x00" +
	"muxenab=0x10\x00" +
	// BT COEX deferral limit setting
	// "btc_params 8 45000\x00"+
	// "btc_params 10 20000\x00"+
	// "spurconfig=0x3\x00"+
	// Antenna diversity
	"swdiv_en=1\x00" +
	"swdiv_gpio=1\x00" +
	"\x00\x00\x00" // C includes null terminator in strings.

	// }
