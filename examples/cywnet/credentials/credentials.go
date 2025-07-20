package credentials

import (
	_ "embed"
)

var (
	//go:embed ssid.text
	ssid string
	//go:embed password.text
	pass string
)

// SSID returns the contents of ssid.text file predefined by user in this package.
// This package is NOT meant to be imported outside of the examples in the CYW43439 repo.
// If you program is failing to compile it is because you need to create a ssid.text and password.text file
// in this package's directory containing the SSID and password of the network you wish to connect to.
//
// Deprecated: Marked as deprecated so IDE warns users agains its use. Your wifi password should be defined outside of this repo for security reasons!
func SSID() string {
	return ssid
}

// Password returns the contents of password.text file predefined by user in this package.
// This package is NOT meant to be imported outside of the examples in the CYW43439 repo.
// If you program is failing to compile it is because you need to create a ssid.text and password.text file
// in this package's directory containing the SSID and password of the network you wish to connect to.
//
// Deprecated: Marked as deprecated so IDE warns users agains its use. Your wifi password should be defined outside of this repo for security reasons!
func Password() string {
	return pass
}
