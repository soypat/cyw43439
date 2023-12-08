//go:build pico

package cyw43439

import (
	"device/rp"
	"machine"
	"runtime/volatile"
	"unsafe"
)

// ref: gpio_get_irq_event_mask
func getIRQEventMask(gpio machine.Pin) uint32 {
	irqCtrlBase := &ioBank0.proc0IRQctrl
	statusReg := &irqCtrlBase.intS[gpio>>3]
	return uint32(getIntChange(gpio, statusReg.Get()))
}

// ref: _gpio_set_irq_enabled
func setIRQ(gpio machine.Pin, events uint32, enable bool) {
	irqCtrlBase := &ioBank0.proc0IRQctrl
	enableReg := &irqCtrlBase.intE[gpio/8]
	events <<= 4 * (gpio % 8)
	if enable {
		enableReg.SetBits(events)
	} else {
		enableReg.ClearBits(events)
	}
}

// ref: gpio_acknowledge_irq
func ackIRQ(gpio machine.Pin, events uint32) {
	ioBank0.intR[gpio/8].Set(events << (4 * (gpio % 8)))
}

type ioType struct {
	status volatile.Register32
	ctrl   volatile.Register32
}

type irqCtrl struct {
	intE [4]volatile.Register32
	intF [4]volatile.Register32
	intS [4]volatile.Register32
}

type ioBank0Type struct {
	io                 [30]ioType
	intR               [4]volatile.Register32
	proc0IRQctrl       irqCtrl
	proc1IRQctrl       irqCtrl
	dormantWakeIRQctrl irqCtrl
}

var ioBank0 = (*ioBank0Type)(unsafe.Pointer(rp.IO_BANK0))

type padsBank0Type struct {
	voltageSelect volatile.Register32
	io            [30]volatile.Register32
}

var padsBank0 = (*padsBank0Type)(unsafe.Pointer(rp.PADS_BANK0))

// Acquire interrupt data from a INT status register.
func getIntChange(p machine.Pin, status uint32) machine.PinChange {
	return machine.PinChange(status>>(4*(p%8))) & 0xf
}

func (d *Device) reenableIRQ() {
	const GPIO_IRQ_LEVEL_HIGH = 0x2
	ackIRQ(d.irq, GPIO_IRQ_LEVEL_HIGH)
	setIRQ(d.irq, GPIO_IRQ_LEVEL_HIGH, true)
	// println("IRQ re-enabled")
}

func (d *Device) irqHandler(pin machine.Pin) {
	// Pico-sdk definition.
	const GPIO_IRQ_LEVEL_HIGH = 0x2
	events := getIRQEventMask(d.irq)
	println("IRQ handler", events)
	if events&GPIO_IRQ_LEVEL_HIGH != 0 {
		// As we use a high level interrupt, it will go off forever until it's serviced
		// So disable the interrupt until this is done. It's re-enabled again by CYW43_POST_POLL_HOOK
		// which is called at the end of cyw43_poll_func
		setIRQ(d.irq, GPIO_IRQ_LEVEL_HIGH, false)
		// set work pending...
		println("disable IRQ")
	}
}

func (d *Device) initIRQ() {
	d.reenableIRQ()
	d.irq.Configure(machine.PinConfig{Mode: machine.PinInput})
	// d.irq.SetInterrupt(1<<2, d.irqHandler)
}

// ref: bool cyw43_ll_has_work(cyw43_ll_t *self_in)
func (d *Device) hasWork() bool {
	if sharedDATA {
		d.irq.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
	}
	return d.irq.Get()
}
